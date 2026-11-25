package agentdata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"nonoka-im/internal/agent/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Store is the PostgreSQL repository for agent configuration, run state,
// approvals and checkpoints. Domain types cross its public surface; row types
// and gorm details stay inside this package.
type Store struct{ db *gorm.DB }

func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

// EnqueueRun records a triggered run without claiming it. The insert is
// idempotent on (event_id, agent_id): a Kafka redelivery returns created=false
// and leaves the existing row unchanged. Only a worker may claim a run.
func (s *Store) EnqueueRun(ctx context.Context, candidate *domain.AgentRun) (bool, error) {
	if candidate.Status == "" {
		candidate.Status = domain.RunQueued
	}
	if candidate.CreatedAt.IsZero() {
		candidate.CreatedAt = time.Now()
	}
	created := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked := &Store{db: tx}
		session, err := locked.GetSession(ctx, candidate.AgentID, candidate.ActorUserID, candidate.Topic)
		if err != nil {
			return err
		}
		candidate.SessionID = session.ID
		row := agentRunRowFromDomain(*candidate)
		res := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "event_id"}, {Name: "agent_id"}},
			DoNothing: true,
		}).Create(&row)
		if res.Error != nil {
			return res.Error
		}
		created = res.RowsAffected == 1
		if created {
			candidate.ID = row.ID
		}
		return nil
	})
	return created, transientOrDomain(err)
}

// ClaimRun atomically moves one queued run to running and grants a lease.
// It enforces per-session ordering in the same statement, so a stale
// QueuedRuns snapshot cannot cause an out-of-order execution. The returned
// AttemptID is the fencing token every later write must present.
func (s *Store) ClaimRun(ctx context.Context, runID uint64, lease time.Duration) (*domain.AgentRun, bool, error) {
	if runID == 0 {
		return nil, false, fmt.Errorf("run id is required")
	}
	if lease <= 0 {
		lease = 3 * time.Minute
	}
	attemptID := uuid.NewString()
	var rows []AgentRunRow
	err := s.db.WithContext(ctx).Raw(`
		UPDATE agent_runs
		SET status = ?,
		    attempts = attempts + 1,
		    attempt_id = ?,
		    started_at = now(),
		    lease_until = now() + (? * interval '1 millisecond'),
		    error_code = '',
		    error_class = '',
		    last_error = '',
		    retry_after_at = NULL,
		    next_attempt_at = NULL
		WHERE id = ?
		  AND status = ?
		  AND (next_attempt_at IS NULL OR next_attempt_at <= now())
		  AND NOT EXISTS (
		    SELECT 1 FROM agent_runs p
		    WHERE p.session_id = agent_runs.session_id
		      AND p.id <> agent_runs.id
		      AND (p.topic_seq < agent_runs.topic_seq
		           OR (p.topic_seq = agent_runs.topic_seq AND p.id < agent_runs.id))
		      AND p.status IN (?, ?, ?)
		  )
		RETURNING *`,
		domain.RunRunning, attemptID, lease.Milliseconds(), runID, domain.RunQueued,
		domain.RunQueued, domain.RunRunning, domain.RunWaitingApproval,
	).Scan(&rows).Error
	if err != nil {
		return nil, false, transientOrDomain(err)
	}
	if len(rows) == 0 {
		return nil, false, nil
	}
	run := rows[0].toDomain()
	return &run, true, nil
}

// LoadRunState loads the session, the compacted snapshot and the turns after
// it for a claimed run. It performs only reads; no lock is held across the
// model call.
func (s *Store) LoadRunState(ctx context.Context, run *domain.AgentRun) (*domain.AgentSession, *domain.AgentContextSnapshot, []domain.AgentTurn, error) {
	var sessionRow AgentSessionRow
	if err := s.db.WithContext(ctx).First(&sessionRow, run.SessionID).Error; err != nil {
		return nil, nil, nil, notFoundOrTransient(err, domain.ErrSessionNotFound)
	}
	session := sessionRow.toDomain()
	snapshot, err := s.Snapshot(ctx, session.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	after := uint64(0)
	if snapshot != nil {
		after = snapshot.LastTurnID
	}
	turns, err := s.TurnsAfter(ctx, session.ID, after, 1000)
	if err != nil {
		return nil, nil, nil, err
	}
	return &session, snapshot, turns, nil
}

// CompleteRun commits the turn, the context snapshot and the terminal run
// state together. The fenced status update runs first: if another worker has
// taken over the lease, the transaction updates zero rows and rolls back.
func (s *Store) CompleteRun(ctx context.Context, run *domain.AgentRun, turn *domain.AgentTurn, snapshot *domain.AgentContextSnapshot, values map[string]interface{}) error {
	if run == nil || run.AttemptID == "" {
		return domain.ErrRunState
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked := &Store{db: tx}
		if err := locked.UpdateRunFenced(ctx, run.ID, run.AttemptID, values); err != nil {
			return err
		}
		if turn != nil {
			if err := locked.AddTurn(ctx, turn); err != nil {
				return err
			}
		}
		if snapshot != nil {
			if err := locked.SaveSnapshot(ctx, snapshot); err != nil {
				return err
			}
		}
		return nil
	})
	return transientOrDomain(err)
}

// FailRun records a failed execution attempt. Retryable failures return the
// run to queued with a next_attempt_at; the attempt count comes from the row,
// so a process restart cannot reset it. Non-retryable failures are terminal.
func (s *Store) FailRun(ctx context.Context, run *domain.AgentRun, failure domain.Failure, maxAttempts int, update map[string]interface{}) error {
	if run == nil || run.AttemptID == "" {
		return domain.ErrRunState
	}
	// A write tool that already committed an external effect makes the run
	// unsafe to retry automatically, even if the failure itself is transient.
	var sideEffectAt sql.NullTime
	if err := s.db.WithContext(ctx).Model(&AgentRunRow{}).Where("id = ?", run.ID).Pluck("side_effect_at", &sideEffectAt).Error; err != nil {
		return fmt.Errorf("%w: %w", domain.ErrTransient, err)
	}
	if sideEffectAt.Valid && failure.Retryable {
		failure.Retryable = false
		failure.Class = domain.FailureClassSideEffect
		failure.Code = domain.FailureCodeManualRetry
	}
	if update == nil {
		update = map[string]interface{}{}
	}
	values := map[string]interface{}{
		"error_code":  failure.Code,
		"error_class": failure.Class,
		"last_error":  truncateError(failure.Message),
		"attempt_id":  "",
		"lease_until": nil,
	}
	if failure.RetryAfter > 0 {
		values["retry_after_at"] = time.Now().Add(failure.RetryAfter)
	}
	for k, v := range update {
		values[k] = v
	}
	status := domain.RunFailed
	if failure.Retryable && (maxAttempts <= 0 || run.Attempts < maxAttempts) {
		status = domain.RunQueued
		delay := failure.RetryAfter
		if delay <= 0 {
			delay = retryBackoff(run.Attempts)
		}
		values["next_attempt_at"] = time.Now().Add(delay)
		values["started_at"] = nil
	} else {
		values["next_attempt_at"] = nil
		values["finished_at"] = time.Now()
	}
	values["status"] = status
	return transientOrDomain(s.UpdateRunFenced(ctx, run.ID, run.AttemptID, values))
}

// retryBackoff is exponential with a 5 minute cap: 5s, 10s, 20s, ...
func retryBackoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	delay := 5 * time.Second * time.Duration(1<<min(attempts-1, 6))
	if delay > 5*time.Minute {
		return 5 * time.Minute
	}
	return delay
}

func truncateError(msg string) string {
	const max = 2000
	if len(msg) <= max {
		return msg
	}
	return msg[:max]
}

// transientOrDomain preserves domain sentinels and marks raw driver errors as
// retryable infrastructure failures.
func transientOrDomain(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrTransient) || errors.Is(err, domain.ErrRunState) || errors.Is(err, domain.ErrAgentNotFound) ||
		errors.Is(err, domain.ErrVersionNotFound) || errors.Is(err, domain.ErrSessionNotFound) ||
		errors.Is(err, domain.ErrApprovalNotFound) || errors.Is(err, domain.ErrApprovalGone) ||
		errors.Is(err, domain.ErrApprovalExpired) || errors.Is(err, domain.ErrSideEffectCommitted) ||
		errors.Is(err, domain.ErrDraftConflict) || errors.Is(err, domain.ErrMCPConflict) ||
		errors.Is(err, domain.ErrSkillNotFound) || errors.Is(err, domain.ErrMCPNotFound) ||
		errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return fmt.Errorf("%w: %w", domain.ErrTransient, err)
}

// UpdateRunFenced writes run state only while the caller still owns the
// lease. It returns ErrRunState when the attempt was superseded or the run is
// no longer running.
func (s *Store) UpdateRunFenced(ctx context.Context, id uint64, attemptID string, values map[string]interface{}) error {
	if attemptID == "" {
		return domain.ErrRunState
	}
	result := s.db.WithContext(ctx).Model(&AgentRunRow{}).
		Where("id = ? AND status = ? AND attempt_id = ?", id, domain.RunRunning, attemptID).
		Updates(values)
	if result.Error != nil {
		return transientOrDomain(result.Error)
	}
	if result.RowsAffected != 1 {
		return domain.ErrRunState
	}
	return nil
}

// UpdateRun is an unfenced administrative update (retry, skip, cancel by the
// service layer). Run execution writes must use UpdateRunFenced.
func (s *Store) UpdateRun(ctx context.Context, id uint64, values map[string]interface{}) error {
	return transientOrDomain(s.db.WithContext(ctx).Model(&AgentRunRow{}).Where("id = ?", id).Updates(values).Error)
}

// Heartbeat extends the lease while the worker still owns the attempt.
func (s *Store) Heartbeat(ctx context.Context, id uint64, attemptID string, lease time.Duration) (bool, error) {
	if attemptID == "" {
		return false, nil
	}
	if lease <= 0 {
		lease = 3 * time.Minute
	}
	result := s.db.WithContext(ctx).Model(&AgentRunRow{}).
		Where("id = ? AND status = ? AND attempt_id = ?", id, domain.RunRunning, attemptID).
		Update("lease_until", gorm.Expr("now() + (? * interval '1 millisecond')", lease.Milliseconds()))
	if result.Error != nil {
		return false, transientOrDomain(result.Error)
	}
	return result.RowsAffected == 1, nil
}

// MarkRunSideEffect records that a write tool produced an external effect during
// this attempt. It is fenced by the attempt token so a stale worker cannot mark
// a run that has already been reclaimed.
func (s *Store) MarkRunSideEffect(ctx context.Context, id uint64, attemptID string) (bool, error) {
	if attemptID == "" {
		return false, nil
	}
	result := s.db.WithContext(ctx).Model(&AgentRunRow{}).
		Where("id = ? AND status = ? AND attempt_id = ?", id, domain.RunRunning, attemptID).
		Update("side_effect_at", time.Now())
	if result.Error != nil {
		return false, fmt.Errorf("%w: %w", domain.ErrTransient, result.Error)
	}
	return result.RowsAffected == 1, nil
}

// SweepStaleRuns recovers runs whose worker stopped heartbeating. Runs under
// the attempt limit are requeued; exhausted runs are failed. The decision is
// based on the lease, not a hard-coded elapsed time.
func (s *Store) SweepStaleRuns(ctx context.Context, maxAttempts int) (requeued, failed int64, err error) {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	res := s.db.WithContext(ctx).Model(&AgentRunRow{}).
		Where("status = ? AND attempts >= ? AND (lease_until IS NULL OR lease_until < now())", domain.RunRunning, maxAttempts).
		Updates(map[string]interface{}{
			"status": domain.RunFailed, "error_code": "lease_expired", "error_class": "transient_infra",
			"finished_at": time.Now(), "attempt_id": "", "lease_until": nil,
		})
	if res.Error != nil {
		return 0, 0, transientOrDomain(res.Error)
	}
	failed = res.RowsAffected
	res = s.db.WithContext(ctx).Model(&AgentRunRow{}).
		Where("status = ? AND attempts < ? AND (lease_until IS NULL OR lease_until < now())", domain.RunRunning, maxAttempts).
		Updates(map[string]interface{}{
			"status": domain.RunQueued, "started_at": nil, "attempt_id": "", "lease_until": nil,
			"next_attempt_at": time.Now(),
		})
	if res.Error != nil {
		return 0, 0, transientOrDomain(res.Error)
	}
	requeued = res.RowsAffected
	return requeued, failed, nil
}

// BlockingRun returns the earlier run in the same session that still blocks
// runID, including waiting_approval (approval pauses the conversation order).
func (s *Store) BlockingRun(ctx context.Context, sessionID uint64, topicSeq uint64, runID uint64) (*domain.AgentRun, error) {
	var row AgentRunRow
	err := s.db.WithContext(ctx).
		Where("session_id = ? AND id <> ? AND (topic_seq < ? OR (topic_seq = ? AND id < ?)) AND status IN ?",
			sessionID, runID, topicSeq, topicSeq, runID,
			[]string{domain.RunQueued, domain.RunRunning, domain.RunWaitingApproval}).
		Order("topic_seq asc, id asc").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, transientOrDomain(err)
	}
	run := row.toDomain()
	return &run, nil
}

func (s *Store) GetSession(ctx context.Context, agentID, actorID int64, topic string) (*domain.AgentSession, error) {
	row := AgentSessionRow{AgentID: agentID, ActorUserID: actorID, Topic: topic}
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "agent_id"}, {Name: "actor_user_id"}, {Name: "topic"}}, DoUpdates: clause.AssignmentColumns([]string{"updated_at"})}).Create(&row).Error
	if err != nil {
		return nil, transientOrDomain(err)
	}
	err = s.db.WithContext(ctx).Where("agent_id=? AND actor_user_id=? AND topic=?", agentID, actorID, topic).First(&row).Error
	if err != nil {
		return nil, notFoundOrTransient(err, domain.ErrSessionNotFound)
	}
	session := row.toDomain()
	return &session, nil
}

func (s *Store) Snapshot(ctx context.Context, sessionID uint64) (*domain.AgentContextSnapshot, error) {
	var row AgentContextSnapshotRow
	err := s.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, transientOrDomain(err)
	}
	snapshot := row.toDomain()
	return &snapshot, nil
}

func (s *Store) SaveSnapshot(ctx context.Context, snapshot *domain.AgentContextSnapshot) error {
	row := agentContextSnapshotRowFromDomain(*snapshot)
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "session_id"}}, DoUpdates: clause.AssignmentColumns([]string{"last_turn_id", "agent_version", "summary", "summary_tokens", "updated_at"})}).Create(&row).Error
	return transientOrDomain(err)
}

func (s *Store) TurnsAfter(ctx context.Context, sessionID, afterID uint64, limit int) ([]domain.AgentTurn, error) {
	var rows []AgentTurnRow
	err := s.db.WithContext(ctx).Where("session_id = ? AND id > ?", sessionID, afterID).Order("id asc").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, transientOrDomain(err)
	}
	out := make([]domain.AgentTurn, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

func (s *Store) TouchSession(ctx context.Context, agentID, actorID int64, topic string) error {
	var row AgentSessionRow
	err := s.db.WithContext(ctx).Where("agent_id=? AND actor_user_id=? AND topic=?", agentID, actorID, topic).First(&row).Error
	if err == nil {
		return transientOrDomain(s.db.WithContext(ctx).Model(&AgentSessionRow{}).Where("id = ?", row.ID).Update("updated_at", time.Now()).Error)
	}
	row = AgentSessionRow{AgentID: agentID, ActorUserID: actorID, Topic: topic, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return transientOrDomain(s.db.WithContext(ctx).Model(&AgentSessionRow{}).Where("agent_id=? AND actor_user_id=? AND topic=?", agentID, actorID, topic).Update("updated_at", time.Now()).Error)
	}
	return nil
}

func (s *Store) AddTurn(ctx context.Context, turn *domain.AgentTurn) error {
	row := agentTurnRowFromDomain(*turn)
	return transientOrDomain(s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "run_id"}}, DoNothing: true}).Create(&row).Error)
}

func (s *Store) RecentTurns(ctx context.Context, agentID, actorID int64, topic string, limit int) ([]domain.AgentTurn, error) {
	if limit <= 0 {
		limit = 10
	}
	var rows []AgentTurnRow
	err := s.db.WithContext(ctx).Where("agent_id=? AND actor_user_id=? AND topic=?", agentID, actorID, topic).Order("created_at desc").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, transientOrDomain(err)
	}
	out := make([]domain.AgentTurn, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

// QueuedRuns returns runs that are due and not blocked by an earlier run in
// the same session. Claiming is still required: multiple workers may see the
// same candidates.
func (s *Store) QueuedRuns(ctx context.Context, limit int) ([]domain.AgentRun, error) {
	if limit <= 0 {
		limit = 32
	}
	var rows []AgentRunRow
	err := s.db.WithContext(ctx).Table("agent_runs AS r").Select("r.*").
		Where(`r.status = ?
		  AND (r.next_attempt_at IS NULL OR r.next_attempt_at <= now())
		  AND NOT EXISTS (
		    SELECT 1 FROM agent_runs p
		    WHERE p.session_id = r.session_id
		      AND p.id <> r.id
		      AND (p.topic_seq < r.topic_seq OR (p.topic_seq = r.topic_seq AND p.id < r.id))
		      AND p.status IN ('queued','running','waiting_approval')
		  )`, domain.RunQueued).
		Order("r.topic_seq asc, r.id asc").Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, transientOrDomain(err)
	}
	out := make([]domain.AgentRun, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}
