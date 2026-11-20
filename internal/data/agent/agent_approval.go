package agentdata

import (
	"context"
	"errors"
	"fmt"
	"time"

	"nonoka-im/internal/agent/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CreateApproval inserts a pending approval unless one already exists for the
// (run, tool call) pair, making interrupt handling idempotent under Kafka
// redelivery. The unique index is the real concurrency guard; the transaction
// makes the insert-or-select sequence atomic and returns the winning row so
// callers can derive a stable card message ID.
func (s *Store) CreateApproval(ctx context.Context, row *domain.AgentApproval) error {
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now()
	}
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = row.CreatedAt
	}
	return transientOrDomain(s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return createApprovalTx(tx, row)
	}))
}

func createApprovalTx(tx *gorm.DB, approval *domain.AgentApproval) error {
	row := agentApprovalRowFromDomain(*approval)
	res := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "run_id"}, {Name: "tool_call_id"}},
		DoNothing: true,
	}).Create(&row)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 1 {
		*approval = row.toDomain()
		return nil
	}
	var existing AgentApprovalRow
	if err := tx.Where("run_id = ? AND tool_call_id = ?", approval.RunID, approval.ToolCallID).First(&existing).Error; err != nil {
		return err
	}
	*approval = existing.toDomain()
	return nil
}

// ParkRunForApproval atomically records the pending approval and moves the run
// to waiting_approval. Doing both in one transaction prevents an orphan
// approval when the lease is lost and prevents a run stuck in running when the
// approval insert succeeds first.
func (s *Store) ParkRunForApproval(ctx context.Context, run *domain.AgentRun, approval *domain.AgentApproval, checkpointID string) (*domain.AgentApproval, error) {
	if run == nil || run.AttemptID == "" {
		return nil, domain.ErrRunState
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := (&Store{db: tx}).UpdateRunFenced(ctx, run.ID, run.AttemptID, map[string]interface{}{
			"status": domain.RunWaitingApproval, "checkpoint_id": checkpointID, "error_code": "",
			"attempt_id": "", "lease_until": nil, "started_at": nil,
		}); err != nil {
			return err
		}
		return createApprovalTx(tx, approval)
	})
	if err != nil {
		return nil, transientOrDomain(err)
	}
	return approval, nil
}

func (s *Store) GetApproval(ctx context.Context, id uint64) (*domain.AgentApproval, error) {
	var row AgentApprovalRow
	err := s.db.WithContext(ctx).First(&row, id).Error
	if err != nil {
		return nil, notFoundOrTransient(err, domain.ErrApprovalNotFound)
	}
	approval := row.toDomain()
	return &approval, nil
}

// ListApprovals returns one page of approvals for the given approver and
// optional status, plus the total count.
func (s *Store) ListApprovals(ctx context.Context, approverID int64, status string, limit, offset int) ([]domain.AgentApproval, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	q := s.db.WithContext(ctx).Model(&AgentApprovalRow{}).Where("approver_id = ?", approverID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, transientOrDomain(err)
	}
	var rows []AgentApprovalRow
	if err := q.Order("id desc").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, transientOrDomain(err)
	}
	out := make([]domain.AgentApproval, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, total, nil
}

// ApprovalForRun returns the most recent approval of the run, if any.
func (s *Store) ApprovalForRun(ctx context.Context, runID uint64) (*domain.AgentApproval, error) {
	var row AgentApprovalRow
	err := s.db.WithContext(ctx).Where("run_id = ?", runID).Order("id desc").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrTransient, err)
	}
	approval := row.toDomain()
	return &approval, nil
}

// DecideApproval records an approve/reject decision only while the approval is
// pending and not yet expired. An expired approval is moved to expired and the
// caller receives ErrApprovalExpired, so the 30-second sweeper window can no
// longer authorize an action on stale consent.
func (s *Store) DecideApproval(ctx context.Context, id uint64, decision string, actorID int64) error {
	if decision != domain.ApprovalApproved && decision != domain.ApprovalRejected {
		return errors.New("invalid approval decision")
	}
	now := time.Now()
	result := s.db.WithContext(ctx).Model(&AgentApprovalRow{}).
		Where("id = ? AND status = ? AND expires_at > ?", id, domain.ApprovalPending, now).
		Updates(map[string]interface{}{
			"status": decision, "decided_at": now, "decided_by": actorID, "updated_at": now,
		})
	if result.Error != nil {
		return transientOrDomain(result.Error)
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var row AgentApprovalRow
	if err := s.db.WithContext(ctx).First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ErrApprovalNotFound
		}
		return fmt.Errorf("%w: %w", domain.ErrTransient, err)
	}
	if row.Status == domain.ApprovalPending {
		// Still pending means the expiry predicate failed. Park it as expired;
		// a concurrent sweep may have won the race, which is harmless.
		_ = s.db.WithContext(ctx).Model(&AgentApprovalRow{}).
			Where("id = ? AND status = ?", id, domain.ApprovalPending).
			Updates(map[string]interface{}{"status": domain.ApprovalExpired, "updated_at": now}).Error
		return domain.ErrApprovalExpired
	}
	return domain.ErrApprovalGone
}

// DecideAndRequeue records the approval decision and moves the waiting run back
// to queued in one transaction. A crash between the two writes previously left
// an approved approval with a permanently parked run.
func (s *Store) DecideAndRequeue(ctx context.Context, id uint64, decision string, actorID int64) error {
	if decision != domain.ApprovalApproved && decision != domain.ApprovalRejected {
		return errors.New("invalid approval decision")
	}
	now := time.Now()
	var resultErr error
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row AgentApprovalRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrApprovalNotFound
			}
			return fmt.Errorf("%w: %w", domain.ErrTransient, err)
		}
		switch row.Status {
		case domain.ApprovalPending:
			if !row.ExpiresAt.After(now) {
				// Persist the expired state and commit it; returning an error
				// here would roll the status update back.
				if err := tx.Model(&AgentApprovalRow{}).Where("id = ? AND status = ?", id, domain.ApprovalPending).
					Updates(map[string]interface{}{"status": domain.ApprovalExpired, "updated_at": now}).Error; err != nil {
					return fmt.Errorf("%w: %w", domain.ErrTransient, err)
				}
				resultErr = domain.ErrApprovalExpired
				return nil
			}
		case domain.ApprovalExpired:
			resultErr = domain.ErrApprovalExpired
			return nil
		default:
			resultErr = domain.ErrApprovalGone
			return nil
		}
		if err := tx.Model(&AgentApprovalRow{}).Where("id = ? AND status = ?", id, domain.ApprovalPending).
			Updates(map[string]interface{}{"status": decision, "decided_at": now, "decided_by": actorID, "updated_at": now}).Error; err != nil {
			return fmt.Errorf("%w: %w", domain.ErrTransient, err)
		}
		res := tx.Model(&AgentRunRow{}).Where("id = ? AND status = ?", row.RunID, domain.RunWaitingApproval).
			Updates(map[string]interface{}{
				"status": domain.RunQueued, "started_at": nil, "attempt_id": "", "lease_until": nil,
				"next_attempt_at": now,
			})
		if res.Error != nil {
			return fmt.Errorf("%w: %w", domain.ErrTransient, res.Error)
		}
		if res.RowsAffected != 1 {
			return domain.ErrRunState
		}
		return nil
	})
	if err != nil {
		return err
	}
	return resultErr
}

// ConsumeApproval atomically claims an approved/rejected/expired approval for
// the resume attempt. It returns the decision to feed the runtime. A second
// resume for the same approval gets ok=false and must not execute the tool.
func (s *Store) ConsumeApproval(ctx context.Context, id uint64, attemptID string) (decision string, ok bool, err error) {
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row AgentApprovalRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		switch row.Status {
		case domain.ApprovalApproved, domain.ApprovalRejected, domain.ApprovalExpired:
		default:
			return nil
		}
		res := tx.Model(&AgentApprovalRow{}).
			Where("id = ? AND status = ?", id, row.Status).
			Updates(map[string]interface{}{
				"status": domain.ApprovalConsumed, "consumed_at": time.Now(), "updated_at": time.Now(), "attempt_id": attemptID,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 1 {
			decision, ok = row.Status, true
		}
		return nil
	})
	return decision, ok, transientOrDomain(err)
}

// ExpirePendingApprovals marks pending approvals past their expiry. Returns
// the affected run IDs so the caller can requeue the waiting runs.
func (s *Store) ExpirePendingApprovals(ctx context.Context, now time.Time) ([]uint64, error) {
	var rows []AgentApprovalRow
	if err := s.db.WithContext(ctx).Where("status = ? AND expires_at < ?", domain.ApprovalPending, now).Find(&rows).Error; err != nil {
		return nil, transientOrDomain(err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	runIDs := make([]uint64, 0, len(rows))
	for i := range rows {
		res := s.db.WithContext(ctx).Model(&AgentApprovalRow{}).
			Where("id = ? AND status = ?", rows[i].ID, domain.ApprovalPending).
			Updates(map[string]interface{}{"status": domain.ApprovalExpired, "updated_at": now})
		if res.Error != nil {
			return nil, transientOrDomain(res.Error)
		}
		if res.RowsAffected == 1 {
			runIDs = append(runIDs, rows[i].RunID)
		}
	}
	return runIDs, nil
}

// ClaimWaitingRun atomically moves a waiting_approval run back to queued for
// resume execution. Returns ErrRunState when the run is not waiting.
func (s *Store) ClaimWaitingRun(ctx context.Context, id uint64) error {
	result := s.db.WithContext(ctx).Model(&AgentRunRow{}).
		Where("id = ? AND status = ?", id, domain.RunWaitingApproval).
		Updates(map[string]interface{}{
			"status": domain.RunQueued, "started_at": nil, "attempt_id": "", "lease_until": nil, "next_attempt_at": time.Now(),
		})
	if result.Error != nil {
		return transientOrDomain(result.Error)
	}
	if result.RowsAffected != 1 {
		return domain.ErrRunState
	}
	return nil
}

// SaveCheckpoint upserts the checkpoint bytes for a checkpoint ID.
func (s *Store) SaveCheckpoint(ctx context.Context, runID uint64, checkpointID string, state []byte, ttl time.Duration) error {
	row := AgentCheckpointRow{RunID: runID, CheckpointID: checkpointID, State: state, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(ttl)}
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "checkpoint_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"run_id", "state", "expires_at"}),
	}).Create(&row).Error
	return transientOrDomain(err)
}

func (s *Store) GetCheckpoint(ctx context.Context, checkpointID string) ([]byte, bool, error) {
	var row AgentCheckpointRow
	err := s.db.WithContext(ctx).Where("checkpoint_id = ?", checkpointID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, transientOrDomain(err)
	}
	if time.Now().After(row.ExpiresAt) {
		return nil, false, nil
	}
	return row.State, true, nil
}

// DeleteCheckpoint removes the checkpoint of a run (for example after a
// failed resume, so a manual retry starts fresh instead of replaying the
// interrupted side effect).
func (s *Store) DeleteCheckpoint(ctx context.Context, checkpointID string) error {
	return transientOrDomain(s.db.WithContext(ctx).Where("checkpoint_id = ?", checkpointID).Delete(&AgentCheckpointRow{}).Error)
}

// PruneExpiredCheckpoints deletes checkpoints past their TTL. Called from the
// worker sweeper.
func (s *Store) PruneExpiredCheckpoints(ctx context.Context, now time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = 500
	}
	result := s.db.WithContext(ctx).Where("expires_at < ?", now).Limit(limit).Delete(&AgentCheckpointRow{})
	return result.RowsAffected, transientOrDomain(result.Error)
}
