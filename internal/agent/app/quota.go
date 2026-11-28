package app

import (
	"context"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/audit"
)

var ErrInvalidInput = fmt.Errorf("invalid quota input")
var ErrForbidden = fmt.Errorf("platform admin required")

// QuotaStore is the persistence surface used by quota admission and usage
// accounting. It is implemented by internal/data/agent.
type QuotaStore interface {
	GetQuota(ctx context.Context, scope, key string) (*domain.Quota, error)
	ListQuotas(ctx context.Context, scope string, limit, offset int) ([]domain.Quota, error)
	UpsertQuota(ctx context.Context, quota domain.Quota) (*domain.Quota, error)
	DeleteQuota(ctx context.Context, scope, key string) error

	GetUsage(ctx context.Context, day time.Time, scope, key string) (domain.Usage, error)
	UsageSeries(ctx context.Context, scope, key string, from, to time.Time) ([]domain.Usage, error)
	TopUsage(ctx context.Context, day time.Time, scope, metric string, limit int) ([]domain.Usage, error)
	RecordUsageEvent(ctx context.Context, day time.Time, event domain.UsageEvent) (bool, error)

	EnqueueRunWithQuota(ctx context.Context, run *domain.AgentRun, day time.Time, retryAt time.Time, userLimits, agentLimits domain.QuotaLimits, maxSessionQueued, maxGlobalQueued int) (bool, *domain.QuotaError, error)
	EnqueueRejectedRun(ctx context.Context, run *domain.AgentRun, code, class, message string, retryAt *time.Time) error
	EnqueueDeferredRun(ctx context.Context, run *domain.AgentRun, day time.Time, retryAt time.Time, maxQueued int) error
	ClaimRunWithQuota(ctx context.Context, runID uint64, lease time.Duration, day time.Time, retryAt time.Time, userLimits, agentLimits domain.QuotaLimits) (*domain.AgentRun, bool, error)
	FailQueuedRun(ctx context.Context, runID uint64, code, class, message string, retryAt *time.Time) error
	GetRun(ctx context.Context, id uint64) (*domain.AgentRun, error)

	ClaimQuotaNotice(ctx context.Context, agentID int64, topic, code string, window time.Duration) (bool, error)
}

type QuotaConfig struct {
	Timezone         string
	User             domain.QuotaLimits
	Agent            domain.QuotaLimits
	MaxSessionQueued int
	MaxGlobalQueued  int
}

type QuotaAuthorizer interface {
	CanManagePlatform(ctx context.Context, userID int64) error
}

type QuotaService struct {
	Store QuotaStore
	Authz QuotaAuthorizer
	Audit *audit.Service
	Cfg   QuotaConfig
	Loc   *time.Location
}

func NewQuotaService(store QuotaStore, cfg QuotaConfig) *QuotaService {
	loc := time.UTC
	if name := strings.TrimSpace(cfg.Timezone); name != "" {
		if parsed, err := time.LoadLocation(name); err == nil {
			loc = parsed
		}
	}
	if cfg.MaxSessionQueued <= 0 {
		cfg.MaxSessionQueued = 3
	}
	if cfg.User.MaxToolTurns <= 0 {
		cfg.User.MaxToolTurns = 8
	}
	if cfg.Agent.MaxToolTurns <= 0 {
		cfg.Agent.MaxToolTurns = 8
	}
	return &QuotaService{Store: store, Cfg: cfg, Loc: loc}
}

// Day returns the natural-day key in the configured quota timezone.
func (s *QuotaService) Day(t time.Time) time.Time {
	local := t.In(s.Loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, s.Loc)
}

// NextReset returns the next local midnight.
func (s *QuotaService) NextReset(t time.Time) time.Time {
	return s.Day(t).AddDate(0, 0, 1)
}

// Effective merges configured defaults with explicit user/agent rows.
func (s *QuotaService) Effective(ctx context.Context, userID, agentID int64) (domain.QuotaLimits, domain.QuotaLimits, error) {
	userLimits := s.Cfg.User
	agentLimits := s.Cfg.Agent
	if userID > 0 {
		row, err := s.Store.GetQuota(ctx, domain.QuotaScopeUser, key(userID))
		if err != nil {
			return userLimits, agentLimits, err
		}
		applyQuota(&userLimits, row)
	}
	if agentID > 0 {
		row, err := s.Store.GetQuota(ctx, domain.QuotaScopeAgent, key(agentID))
		if err != nil {
			return userLimits, agentLimits, err
		}
		applyQuota(&agentLimits, row)
	}
	return userLimits, agentLimits, nil
}

// AdmitEnqueue applies daily run and per-session queue limits at dispatch time.
func (s *QuotaService) AdmitEnqueue(ctx context.Context, run *domain.AgentRun) (bool, *domain.QuotaError, error) {
	userLimits, agentLimits, err := s.Effective(ctx, run.ActorUserID, run.AgentID)
	if err != nil {
		return false, nil, err
	}
	return s.Store.EnqueueRunWithQuota(ctx, run, s.Day(time.Now()), s.NextReset(time.Now()), userLimits, agentLimits, s.Cfg.MaxSessionQueued, s.Cfg.MaxGlobalQueued)
}

// Reject records a terminal run that was refused before any model call.
func (s *QuotaService) Reject(ctx context.Context, run *domain.AgentRun, code, class string, cause error, retryAt *time.Time) error {
	message := "Agent run blocked by quota policy."
	if cause != nil {
		message = cause.Error()
	}
	return s.Store.EnqueueRejectedRun(ctx, run, code, class, message, retryAt)
}

// Defer records a transiently queue-full run so the replay loop can execute it
// after the queue drains.
func (s *QuotaService) Defer(ctx context.Context, run *domain.AgentRun, retryAt time.Time) error {
	return s.Store.EnqueueDeferredRun(ctx, run, s.Day(time.Now()), retryAt, s.Cfg.MaxGlobalQueued)
}

// ClaimRun is the execution admission path. A daily budget failure is recorded
// as a terminal failure before returning; a concurrency hit leaves the run
// queued.
func (s *QuotaService) ClaimRun(ctx context.Context, runID uint64, lease time.Duration) (*domain.AgentRun, bool, error) {
	pending, err := s.Store.GetRun(ctx, runID)
	if err != nil {
		return nil, false, err
	}
	// Cheap rejection before loading quotas: a replay loop can enqueue the same
	// ID many times while another worker already owns it. This keeps duplicate
	// claims to one indexed read instead of three.
	if pending.Status != domain.RunQueued {
		return nil, false, nil
	}
	if pending.NextAttemptAt != nil && pending.NextAttemptAt.After(time.Now()) {
		return nil, false, nil
	}
	userLimits, agentLimits, err := s.Effective(ctx, pending.ActorUserID, pending.AgentID)
	if err != nil {
		return nil, false, err
	}
	retryAt := s.NextReset(time.Now())
	run, ok, err := s.Store.ClaimRunWithQuota(ctx, runID, lease, s.Day(time.Now()), retryAt, userLimits, agentLimits)
	if err != nil {
		var budgetErr *domain.BudgetError
		if !asBudget(err, &budgetErr) {
			return nil, false, err
		}
		code := "budget_exceeded"
		retry := budgetErr.RetryAt
		_ = s.Store.FailQueuedRun(ctx, runID, code, "budget", budgetErr.Error(), &retry)
		return run, false, budgetErr
	}
	return run, ok, nil
}

func (s *QuotaService) RecordUsage(ctx context.Context, ev domain.UsageEvent) (bool, error) {
	return s.Store.RecordUsageEvent(ctx, s.Day(time.Now()), ev)
}

func (s *QuotaService) Usage(ctx context.Context, scope, key string, days int) ([]domain.Usage, domain.Usage, error) {
	if days <= 0 {
		days = 7
	}
	if days > 90 {
		days = 90
	}
	to := s.Day(time.Now())
	from := to.AddDate(0, 0, -(days - 1))
	rows, err := s.Store.UsageSeries(ctx, scope, key, from, to)
	if err != nil {
		return nil, domain.Usage{}, err
	}
	var total domain.Usage
	total.Day = to
	total.Scope, total.ScopeKey = scope, key
	for _, row := range rows {
		total.Runs += row.Runs
		total.InputTokens += row.InputTokens
		total.OutputTokens += row.OutputTokens
		total.CostUSD += row.CostUSD
	}
	return rows, total, nil
}

func (s *QuotaService) Top(ctx context.Context, scope, metric string, limit int) ([]domain.Usage, error) {
	return s.Store.TopUsage(ctx, s.Day(time.Now()), scope, metric, limit)
}

func (s *QuotaService) List(ctx context.Context, scope string, limit, offset int) ([]domain.Quota, error) {
	return s.Store.ListQuotas(ctx, scope, limit, offset)
}

func (s *QuotaService) Update(ctx context.Context, scope, scopeKey string, limits domain.QuotaLimits, userID int64) (*domain.Quota, error) {
	if err := validateScope(scope, scopeKey); err != nil {
		return nil, err
	}
	if limits.MaxConcurrentRuns < 0 || limits.DailyRuns < 0 || limits.DailyTokens < 0 || limits.DailyCostUSD < 0 || limits.MaxToolTurns < 0 {
		return nil, fmt.Errorf("%w: quota limits must be non-negative", ErrInvalidInput)
	}
	q := domain.Quota{
		Scope: scope, ScopeKey: scopeKey, UpdatedBy: userID,
		MaxConcurrentRuns: intPtr(limits.MaxConcurrentRuns),
		DailyRuns:         intPtr(limits.DailyRuns),
		DailyTokens:       int64Ptr(limits.DailyTokens),
		DailyCostUSD:      floatPtr(limits.DailyCostUSD),
		MaxToolTurns:      intPtr(limits.MaxToolTurns),
	}
	return s.Store.UpsertQuota(ctx, q)
}

func (s *QuotaService) Reset(ctx context.Context, scope, scopeKey string) error {
	if err := validateScope(scope, scopeKey); err != nil {
		return err
	}
	return s.Store.DeleteQuota(ctx, scope, scopeKey)
}

func (s *QuotaService) ShouldNotify(ctx context.Context, agentID int64, topic, code string) (bool, error) {
	return s.Store.ClaimQuotaNotice(ctx, agentID, topic, code, 10*time.Minute)
}

func (s *QuotaService) AdminList(ctx context.Context, userID int64, scope string, limit, offset int) ([]domain.Quota, error) {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return nil, err
	}
	return s.Store.ListQuotas(ctx, scope, limit, offset)
}

func (s *QuotaService) AdminUpdate(ctx context.Context, userID int64, scope, key string, limits domain.QuotaLimits) (*domain.Quota, error) {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return nil, err
	}
	before, _ := s.Store.GetQuota(ctx, scope, key)
	row, err := s.Update(ctx, scope, key, limits, userID)
	if err != nil {
		return nil, err
	}
	if s.Audit != nil {
		if err := s.Audit.Record(ctx, audit.Event{
			ActorUserID: userID, Action: "quota.update", ResourceType: "quota", ResourceID: scope + ":" + key,
			Before: quotaAuditFields(before), After: quotaAuditFields(row),
		}); err != nil {
			return nil, err
		}
	}
	return row, nil
}

func (s *QuotaService) AdminReset(ctx context.Context, userID int64, scope, key string) error {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return err
	}
	before, _ := s.Store.GetQuota(ctx, scope, key)
	if err := s.Reset(ctx, scope, key); err != nil {
		return err
	}
	if s.Audit != nil {
		if err := s.Audit.Record(ctx, audit.Event{
			ActorUserID: userID, Action: "quota.reset", ResourceType: "quota", ResourceID: scope + ":" + key,
			Before: quotaAuditFields(before),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *QuotaService) AdminTop(ctx context.Context, userID int64, scope, metric string, limit int) ([]domain.Usage, error) {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return nil, err
	}
	return s.Top(ctx, scope, metric, limit)
}

func (s *QuotaService) AdminUsage(ctx context.Context, userID int64, scope, key string, days int) ([]domain.Usage, domain.Usage, error) {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return nil, domain.Usage{}, err
	}
	return s.Usage(ctx, scope, key, days)
}

func (s *QuotaService) requireAdmin(ctx context.Context, userID int64) error {
	if s.Authz == nil || s.Authz.CanManagePlatform(ctx, userID) != nil {
		return ErrForbidden
	}
	return nil
}

func (s *QuotaService) RequireAdmin(ctx context.Context, userID int64) error {
	return s.requireAdmin(ctx, userID)
}

func validateScope(scope, key string) error {
	if scope != domain.QuotaScopeUser && scope != domain.QuotaScopeAgent {
		return fmt.Errorf("%w: invalid quota scope", ErrInvalidInput)
	}
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("%w: quota scope key is required", ErrInvalidInput)
	}
	return nil
}

func applyQuota(limits *domain.QuotaLimits, q *domain.Quota) {
	if q == nil {
		return
	}
	if q.MaxConcurrentRuns != nil {
		limits.MaxConcurrentRuns = *q.MaxConcurrentRuns
	}
	if q.DailyRuns != nil {
		limits.DailyRuns = *q.DailyRuns
	}
	if q.DailyTokens != nil {
		limits.DailyTokens = *q.DailyTokens
	}
	if q.DailyCostUSD != nil {
		limits.DailyCostUSD = *q.DailyCostUSD
	}
	if q.MaxToolTurns != nil {
		limits.MaxToolTurns = *q.MaxToolTurns
	}
}

func key(id int64) string { return fmt.Sprintf("%d", id) }
func quotaAuditFields(q *domain.Quota) map[string]any {
	if q == nil {
		return nil
	}
	return map[string]any{
		"scope": q.Scope, "scopeKey": q.ScopeKey, "maxConcurrentRuns": q.MaxConcurrentRuns,
		"dailyRuns": q.DailyRuns, "dailyTokens": q.DailyTokens, "dailyCostUsd": q.DailyCostUSD,
		"maxToolTurns": q.MaxToolTurns,
	}
}
func intPtr(v int) *int           { return &v }
func int64Ptr(v int64) *int64     { return &v }
func floatPtr(v float64) *float64 { return &v }
func asBudget(err error, dst **domain.BudgetError) bool {
	for err != nil {
		if e, ok := err.(*domain.BudgetError); ok {
			*dst = e
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
