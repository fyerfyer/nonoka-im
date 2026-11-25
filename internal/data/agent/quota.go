package agentdata

import (
	"context"
	"errors"
	"strconv"
	"time"

	"nonoka-im/internal/agent/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type QuotaRow struct {
	ID                uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Scope             string    `gorm:"column:scope;size:16;not null"`
	ScopeKey          string    `gorm:"column:scope_key;size:160;not null"`
	MaxConcurrentRuns *int      `gorm:"column:max_concurrent_runs"`
	DailyRuns         *int      `gorm:"column:daily_runs"`
	DailyTokens       *int64    `gorm:"column:daily_tokens"`
	DailyCostUSD      *float64  `gorm:"column:daily_cost_usd"`
	MaxToolTurns      *int      `gorm:"column:max_tool_turns"`
	UpdatedBy         int64     `gorm:"column:updated_by"`
	CreatedAt         time.Time `gorm:"column:created_at"`
	UpdatedAt         time.Time `gorm:"column:updated_at"`
}

func (QuotaRow) TableName() string { return "agent_quotas" }

func (r QuotaRow) toDomain() domain.Quota {
	return domain.Quota{
		ID: r.ID, Scope: r.Scope, ScopeKey: r.ScopeKey,
		MaxConcurrentRuns: r.MaxConcurrentRuns, DailyRuns: r.DailyRuns,
		DailyTokens: r.DailyTokens, DailyCostUSD: r.DailyCostUSD,
		MaxToolTurns: r.MaxToolTurns, UpdatedBy: r.UpdatedBy,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

type UsageRow struct {
	Day          time.Time `gorm:"column:day;type:date"`
	Scope        string    `gorm:"column:scope;size:16"`
	ScopeKey     string    `gorm:"column:scope_key;size:160"`
	Runs         int64     `gorm:"column:runs"`
	InputTokens  int64     `gorm:"column:input_tokens"`
	OutputTokens int64     `gorm:"column:output_tokens"`
	CostUSD      float64   `gorm:"column:cost_usd"`
}

func (UsageRow) TableName() string { return "agent_usage_daily" }

func (r UsageRow) toDomain() domain.Usage {
	return domain.Usage{
		Day: r.Day, Scope: r.Scope, ScopeKey: r.ScopeKey, Runs: r.Runs,
		InputTokens: r.InputTokens, OutputTokens: r.OutputTokens, CostUSD: r.CostUSD,
	}
}

func (s *Store) GetQuota(ctx context.Context, scope, key string) (*domain.Quota, error) {
	var row QuotaRow
	err := s.db.WithContext(ctx).Where("scope = ? AND scope_key = ?", scope, key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, transientOrDomain(err)
	}
	q := row.toDomain()
	return &q, nil
}

func (s *Store) ListQuotas(ctx context.Context, scope string, limit, offset int) ([]domain.Quota, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows []QuotaRow
	q := s.db.WithContext(ctx).Order("updated_at desc").Limit(limit).Offset(offset)
	if scope != "" {
		q = q.Where("scope = ?", scope)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, transientOrDomain(err)
	}
	out := make([]domain.Quota, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

func (s *Store) UpsertQuota(ctx context.Context, q domain.Quota) (*domain.Quota, error) {
	row := QuotaRow{
		Scope: q.Scope, ScopeKey: q.ScopeKey,
		MaxConcurrentRuns: q.MaxConcurrentRuns, DailyRuns: q.DailyRuns,
		DailyTokens: q.DailyTokens, DailyCostUSD: q.DailyCostUSD,
		MaxToolTurns: q.MaxToolTurns, UpdatedBy: q.UpdatedBy,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "scope"}, {Name: "scope_key"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"max_concurrent_runs", "daily_runs", "daily_tokens", "daily_cost_usd",
			"max_tool_turns", "updated_by", "updated_at",
		}),
	}).Create(&row).Error
	if err != nil {
		return nil, transientOrDomain(err)
	}
	return s.GetQuota(ctx, q.Scope, q.ScopeKey)
}

func (s *Store) DeleteQuota(ctx context.Context, scope, key string) error {
	return transientOrDomain(s.db.WithContext(ctx).
		Where("scope = ? AND scope_key = ?", scope, key).Delete(&QuotaRow{}).Error)
}

func (s *Store) GetUsage(ctx context.Context, day time.Time, scope, key string) (domain.Usage, error) {
	var row UsageRow
	err := s.db.WithContext(ctx).Where("day = ? AND scope = ? AND scope_key = ?", dayKey(day), scope, key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Usage{Day: day, Scope: scope, ScopeKey: key}, nil
	}
	if err != nil {
		return domain.Usage{}, transientOrDomain(err)
	}
	return row.toDomain(), nil
}

func (s *Store) UsageSeries(ctx context.Context, scope, key string, from, to time.Time) ([]domain.Usage, error) {
	var rows []UsageRow
	err := s.db.WithContext(ctx).
		Where("scope = ? AND scope_key = ? AND day >= ? AND day <= ?", scope, key, dayKey(from), dayKey(to)).
		Order("day asc").Find(&rows).Error
	if err != nil {
		return nil, transientOrDomain(err)
	}
	out := make([]domain.Usage, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

func (s *Store) TopUsage(ctx context.Context, day time.Time, scope, metric string, limit int) ([]domain.Usage, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	order := "cost_usd desc"
	switch metric {
	case "runs":
		order = "runs desc"
	case "tokens":
		order = "(input_tokens + output_tokens) desc"
	}
	var rows []UsageRow
	err := s.db.WithContext(ctx).
		Where("day = ? AND scope = ?", dayKey(day), scope).
		Order(order).Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, transientOrDomain(err)
	}
	out := make([]domain.Usage, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

func (s *Store) incrementDailyRuns(ctx context.Context, tx *gorm.DB, day time.Time, userID, agentID int64) error {
	rows := []struct {
		scope string
		key   string
	}{
		{domain.QuotaScopeUser, intKey(userID)},
		{domain.QuotaScopeAgent, intKey(agentID)},
	}
	for _, row := range rows {
		err := tx.WithContext(ctx).Exec(`
			INSERT INTO agent_usage_daily (day, scope, scope_key, runs, input_tokens, output_tokens, cost_usd, updated_at)
			VALUES (?, ?, ?, 1, 0, 0, 0, now())
			ON CONFLICT (day, scope, scope_key) DO UPDATE SET
				runs = agent_usage_daily.runs + EXCLUDED.runs,
				updated_at = now()
		`, dayKey(day), row.scope, row.key).Error
		if err != nil {
			return err
		}
	}
	return nil
}

// RecordUsageEvent persists one model call's usage exactly once and updates the
// user/agent daily aggregates plus the run totals.
func (s *Store) RecordUsageEvent(ctx context.Context, day time.Time, ev domain.UsageEvent) (bool, error) {
	inserted := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.WithContext(ctx).Exec(`
			INSERT INTO agent_usage_events (
				run_id, attempt_id, step, agent_id, actor_user_id, topic,
				model_id, model_key, provider, input_tokens, output_tokens, cost_usd, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, now())
			ON CONFLICT (run_id, attempt_id, step) DO NOTHING
		`, ev.RunID, ev.AttemptID, ev.Step, ev.AgentID, ev.ActorUserID, ev.Topic,
			ev.ModelID, ev.ModelKey, ev.Provider, ev.InputTokens, ev.OutputTokens, ev.CostUSD)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		inserted = true
		scopes := []struct{ scope, key string }{
			{domain.QuotaScopeUser, intKey(ev.ActorUserID)},
			{domain.QuotaScopeAgent, intKey(ev.AgentID)},
		}
		for _, sc := range scopes {
			if err := tx.WithContext(ctx).Exec(`
				INSERT INTO agent_usage_daily (day, scope, scope_key, runs, input_tokens, output_tokens, cost_usd, updated_at)
				VALUES (?, ?, ?, 0, ?, ?, ?, now())
				ON CONFLICT (day, scope, scope_key) DO UPDATE SET
					input_tokens = agent_usage_daily.input_tokens + EXCLUDED.input_tokens,
					output_tokens = agent_usage_daily.output_tokens + EXCLUDED.output_tokens,
					cost_usd = agent_usage_daily.cost_usd + EXCLUDED.cost_usd,
					updated_at = now()
			`, dayKey(day), sc.scope, sc.key, ev.InputTokens, ev.OutputTokens, ev.CostUSD).Error; err != nil {
				return err
			}
		}
		err := tx.WithContext(ctx).Exec(`
			UPDATE agent_runs
			SET input_tokens = input_tokens + ?,
			    output_tokens = output_tokens + ?,
			    total_tokens = total_tokens + ?,
			    estimated_cost_usd = estimated_cost_usd + ?,
			    model_key = CASE WHEN model_key = '' THEN ? ELSE model_key END
			WHERE id = ?
		`, ev.InputTokens, ev.OutputTokens, ev.InputTokens+ev.OutputTokens, ev.CostUSD, ev.ModelKey, ev.RunID).Error
		return err
	})
	return inserted, transientOrDomain(err)
}

func dayKey(t time.Time) string {
	return t.Format("2006-01-02")
}

func intKey(v int64) string {
	return strconv.FormatInt(v, 10)
}

// ClaimRunWithQuota is the only path a worker may use to start an execution. It
// serializes per-user and per-agent admission with transaction-scoped advisory
// locks, checks live concurrency (not a drift-prone counter), and rejects a run
// whose daily budget is already exhausted. A concurrency-limit hit leaves the
// run queued for a later replay.
func (s *Store) ClaimRunWithQuota(ctx context.Context, runID uint64, lease time.Duration, day time.Time, retryAt time.Time, userLimits, agentLimits domain.QuotaLimits) (*domain.AgentRun, bool, error) {
	if runID == 0 {
		return nil, false, nil
	}
	if lease <= 0 {
		lease = 3 * time.Minute
	}
	var claimed *domain.AgentRun
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row AgentRunRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, runID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if row.Status != domain.RunQueued {
			return nil
		}
		if row.NextAttemptAt != nil && row.NextAttemptAt.After(time.Now()) {
			return nil
		}
		locks := []string{
			"nonoka:quota:user:" + intKey(row.ActorUserID),
			"nonoka:quota:agent:" + intKey(row.AgentID),
		}
		for _, key := range locks {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", key).Error; err != nil {
				return err
			}
		}
		if userLimits.MaxConcurrentRuns > 0 || agentLimits.MaxConcurrentRuns > 0 {
			userRunning, err := countRunningTx(ctx, tx, "actor_user_id", row.ActorUserID, runID)
			if err != nil {
				return err
			}
			agentRunning, err := countRunningTx(ctx, tx, "agent_id", row.AgentID, runID)
			if err != nil {
				return err
			}
			if (userLimits.MaxConcurrentRuns > 0 && userRunning >= userLimits.MaxConcurrentRuns) ||
				(agentLimits.MaxConcurrentRuns > 0 && agentRunning >= agentLimits.MaxConcurrentRuns) {
				return nil
			}
		}
		userUsage, err := usageTx(ctx, tx, day, domain.QuotaScopeUser, intKey(row.ActorUserID))
		if err != nil {
			return err
		}
		agentUsage, err := usageTx(ctx, tx, day, domain.QuotaScopeAgent, intKey(row.AgentID))
		if err != nil {
			return err
		}
		if over, metric, limit, used := budgetExceeded(userLimits, userUsage); over {
			claimed = &domain.AgentRun{ID: row.ID, EventID: row.EventID, AgentID: row.AgentID, ActorUserID: row.ActorUserID, Topic: row.Topic}
			return &domain.BudgetError{
				Scope: domain.QuotaScopeUser, Metric: metric, Limit: limit, Used: used,
				RetryAt: retryAt, RetryAfter: time.Until(retryAt),
				Message: "You reached your daily " + metric + " budget. Try again after " + retryAt.Format("2006-01-02 15:04 MST") + ".",
			}
		}
		if over, metric, limit, used := budgetExceeded(agentLimits, agentUsage); over {
			claimed = &domain.AgentRun{ID: row.ID, EventID: row.EventID, AgentID: row.AgentID, ActorUserID: row.ActorUserID, Topic: row.Topic}
			return &domain.BudgetError{
				Scope: domain.QuotaScopeAgent, Metric: metric, Limit: limit, Used: used,
				RetryAt: retryAt, RetryAfter: time.Until(retryAt),
				Message: "This agent reached its daily " + metric + " budget. Try again after " + retryAt.Format("2006-01-02 15:04 MST") + ".",
			}
		}
		attemptID := uuid.NewString()
		var rows []AgentRunRow
		err = tx.Raw(`
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
			return err
		}
		if len(rows) == 1 {
			run := rows[0].toDomain()
			claimed = &run
		}
		return nil
	})
	if err != nil {
		var budgetErr *domain.BudgetError
		if errors.As(err, &budgetErr) {
			return claimed, false, budgetErr
		}
		return nil, false, transientOrDomain(err)
	}
	if claimed == nil {
		return nil, false, nil
	}
	return claimed, true, nil
}

// EnqueueRunWithQuota admits a run at dispatch time. Daily run quotas and the
// per-session queue cap are checked under the same user/agent locks used by
// execution, so concurrent dispatchers cannot oversell them.
func (s *Store) EnqueueRunWithQuota(ctx context.Context, candidate *domain.AgentRun, day time.Time, retryAt time.Time, userLimits, agentLimits domain.QuotaLimits, maxSessionQueued, maxGlobalQueued int) (bool, *domain.QuotaError, error) {
	if candidate.Status == "" {
		candidate.Status = domain.RunQueued
	}
	if candidate.CreatedAt.IsZero() {
		candidate.CreatedAt = time.Now()
	}
	created := false
	var quotaErr *domain.QuotaError
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked := &Store{db: tx}
		session, err := locked.GetSession(ctx, candidate.AgentID, candidate.ActorUserID, candidate.Topic)
		if err != nil {
			return err
		}
		candidate.SessionID = session.ID
		locks := []string{
			"nonoka:quota:user:" + intKey(candidate.ActorUserID),
			"nonoka:quota:agent:" + intKey(candidate.AgentID),
			"nonoka:queue:session:" + strconv.FormatUint(session.ID, 10),
		}
		for _, key := range locks {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", key).Error; err != nil {
				return err
			}
		}
		userUsage, err := usageTx(ctx, tx, day, domain.QuotaScopeUser, intKey(candidate.ActorUserID))
		if err != nil {
			return err
		}
		agentUsage, err := usageTx(ctx, tx, day, domain.QuotaScopeAgent, intKey(candidate.AgentID))
		if err != nil {
			return err
		}
		if over, metric, limit, used := runsExceeded(userLimits, userUsage); over {
			quotaErr = &domain.QuotaError{
				Scope: domain.QuotaScopeUser, Metric: metric, Limit: limit, Used: used,
				RetryAfter: time.Until(retryAt), RetryAt: retryAt,
				Message: "You reached your daily agent run limit. Try again tomorrow.",
			}
			return nil
		}
		if over, metric, limit, used := runsExceeded(agentLimits, agentUsage); over {
			quotaErr = &domain.QuotaError{
				Scope: domain.QuotaScopeAgent, Metric: metric, Limit: limit, Used: used,
				RetryAfter: time.Until(retryAt), RetryAt: retryAt,
				Message: "This agent reached its daily run limit. Try again tomorrow.",
			}
			return nil
		}
		if maxSessionQueued > 0 {
			var queued int64
			if err := tx.Model(&AgentRunRow{}).
				Where("session_id = ? AND status IN ?", session.ID, []string{domain.RunQueued, domain.RunRunning, domain.RunWaitingApproval}).
				Count(&queued).Error; err != nil {
				return err
			}
			if queued >= int64(maxSessionQueued) {
				quotaErr = &domain.QuotaError{
					Scope: "session", Metric: "queued_runs", Limit: float64(maxSessionQueued), Used: float64(queued),
					RetryAfter: 5 * time.Second, RetryAt: time.Now().Add(5 * time.Second),
					Message: "This conversation already has too many queued agent runs. Please wait for the current run to finish.",
				}
				return nil
			}
		}
		if maxGlobalQueued > 0 {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "nonoka:queue:global").Error; err != nil {
				return err
			}
			var queued int64
			if err := tx.Model(&AgentRunRow{}).
				Where("status IN ?", []string{domain.RunQueued, domain.RunRunning, domain.RunWaitingApproval}).
				Count(&queued).Error; err != nil {
				return err
			}
			if queued >= int64(maxGlobalQueued) {
				quotaErr = &domain.QuotaError{
					Scope: "global", Metric: "queued_runs", Limit: float64(maxGlobalQueued), Used: float64(queued),
					RetryAfter: 10 * time.Second, RetryAt: time.Now().Add(10 * time.Second),
					Message: "The agent queue is full. Please try again shortly.",
				}
				return nil
			}
		}
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
			if err := locked.incrementDailyRuns(ctx, tx, day, candidate.ActorUserID, candidate.AgentID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return false, nil, transientOrDomain(err)
	}
	return created, quotaErr, nil
}

// EnqueueDeferredRun records a transiently deferred run and spends the daily
// run quota once, like any other accepted run. The replay loop picks it up
// after retryAt, so a queue-full admission is not permanently dropped.
func (s *Store) EnqueueDeferredRun(ctx context.Context, candidate *domain.AgentRun, day time.Time, retryAt time.Time, maxQueued int) error {
	if candidate.Status == "" {
		candidate.Status = domain.RunQueued
	}
	if candidate.CreatedAt.IsZero() {
		candidate.CreatedAt = time.Now()
	}
	candidate.NextAttemptAt = &retryAt
	return transientOrDomain(s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A deferred row still occupies a queued slot. Allow at most 2x the
		// configured capacity so queue-full events are not dropped, while a
		// sustained flood applies backpressure through Kafka redelivery.
		if maxQueued > 0 {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "nonoka:queue:global").Error; err != nil {
				return err
			}
			var queued int64
			if err := tx.Model(&AgentRunRow{}).
				Where("status IN ?", []string{domain.RunQueued, domain.RunRunning, domain.RunWaitingApproval}).
				Count(&queued).Error; err != nil {
				return err
			}
			if queued > int64(maxQueued*2) {
				return &domain.QuotaError{
					Scope: "global", Metric: "queued_runs", Limit: float64(maxQueued * 2), Used: float64(queued),
					RetryAfter: 10 * time.Second, RetryAt: time.Now().Add(10 * time.Second),
					Message: "The agent queue is full. Please try again shortly.",
				}
			}
		}
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
		if res.RowsAffected == 0 {
			return nil
		}
		candidate.ID = row.ID
		return locked.incrementDailyRuns(ctx, tx, day, candidate.ActorUserID, candidate.AgentID)
	}))
}

// EnqueueRejectedRun records a terminal, non-executed run for visibility. It
// deliberately does not increment the daily run counters.
func (s *Store) EnqueueRejectedRun(ctx context.Context, candidate *domain.AgentRun, code, class, message string, retryAt *time.Time) error {
	candidate.Status = domain.RunFailed
	candidate.ErrorCode = code
	candidate.ErrorClass = class
	candidate.LastError = message
	candidate.RetryAfterAt = retryAt
	candidate.FinishedAt = ptrTimeValue(time.Now())
	if candidate.CreatedAt.IsZero() {
		candidate.CreatedAt = time.Now()
	}
	return transientOrDomain(s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
		return nil
	}))
}

func countRunningTx(ctx context.Context, tx *gorm.DB, column string, id int64, excludeRunID uint64) (int, error) {
	var n int64
	err := tx.WithContext(ctx).Model(&AgentRunRow{}).
		Where(column+" = ? AND status = ? AND id <> ? AND (lease_until IS NULL OR lease_until > now())", id, domain.RunRunning, excludeRunID).
		Count(&n).Error
	return int(n), err
}

func usageTx(ctx context.Context, tx *gorm.DB, day time.Time, scope, key string) (domain.Usage, error) {
	var row UsageRow
	err := tx.WithContext(ctx).Where("day = ? AND scope = ? AND scope_key = ?", dayKey(day), scope, key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Usage{Day: day, Scope: scope, ScopeKey: key}, nil
	}
	if err != nil {
		return domain.Usage{}, err
	}
	return row.toDomain(), nil
}

func budgetExceeded(limits domain.QuotaLimits, usage domain.Usage) (bool, string, float64, float64) {
	tokens := usage.InputTokens + usage.OutputTokens
	if limits.DailyTokens > 0 && tokens >= limits.DailyTokens {
		return true, "tokens", float64(limits.DailyTokens), float64(tokens)
	}
	if limits.DailyCostUSD > 0 && usage.CostUSD >= limits.DailyCostUSD {
		return true, "cost", limits.DailyCostUSD, usage.CostUSD
	}
	return false, "", 0, 0
}

func runsExceeded(limits domain.QuotaLimits, usage domain.Usage) (bool, string, float64, float64) {
	if limits.DailyRuns > 0 && usage.Runs >= int64(limits.DailyRuns) {
		return true, "runs", float64(limits.DailyRuns), float64(usage.Runs)
	}
	return false, "", 0, 0
}

func nextDayRetry(day time.Time, loc *time.Location) time.Duration {
	next := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	return time.Until(next)
}

func nextDayRetryAt(day time.Time, loc *time.Location) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
}

func ptrTimeValue(t time.Time) *time.Time { return &t }

// FailQueuedRun terminal-fails a run that has not been claimed (for example a
// daily budget denial). It never touches a running attempt.
func (s *Store) FailQueuedRun(ctx context.Context, runID uint64, code, class, message string, retryAt *time.Time) error {
	values := map[string]interface{}{
		"status": domain.RunFailed, "error_code": code, "error_class": class,
		"last_error": truncateError(message), "finished_at": time.Now(),
		"next_attempt_at": nil, "attempt_id": "", "lease_until": nil,
		"retry_after_at": retryAt,
	}
	res := s.db.WithContext(ctx).Model(&AgentRunRow{}).
		Where("id = ? AND status = ?", runID, domain.RunQueued).Updates(values)
	if res.Error != nil {
		return transientOrDomain(res.Error)
	}
	return nil
}

// ClaimQuotaNotice returns true when a (agent, topic, code) notice should be
// sent. The upsert is atomic across workers and survives Redis restarts.
func (s *Store) ClaimQuotaNotice(ctx context.Context, agentID int64, topic, code string, window time.Duration) (bool, error) {
	if window <= 0 {
		window = 10 * time.Minute
	}
	res := s.db.WithContext(ctx).Exec(`
		INSERT INTO agent_quota_notices (agent_id, topic, error_code, last_sent_at)
		VALUES (?, ?, ?, now())
		ON CONFLICT (agent_id, topic, error_code) DO UPDATE
		SET last_sent_at = now()
		WHERE agent_quota_notices.last_sent_at < now() - (? * interval '1 millisecond')
	`, agentID, topic, code, window.Milliseconds())
	if res.Error != nil {
		return false, transientOrDomain(res.Error)
	}
	return res.RowsAffected > 0, nil
}
