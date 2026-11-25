package domain

import "time"

// Quota scopes. topic is intentionally not implemented yet; the persistence
// model keeps scope as a string so it can be added without a schema rewrite.
const (
	QuotaScopeUser  = "user"
	QuotaScopeAgent = "agent"
)

// Quota is the editable per-scope limit row. Nil means "use the configured
// default for this scope"; zero in an effective Limits value means unlimited.
type Quota struct {
	ID                uint64
	Scope             string
	ScopeKey          string
	MaxConcurrentRuns *int
	DailyRuns         *int
	DailyTokens       *int64
	DailyCostUSD      *float64
	MaxToolTurns      *int
	UpdatedBy         int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// QuotaLimits are the effective limits applied to one run. A zero value means
// that metric is unlimited for this scope.
type QuotaLimits struct {
	MaxConcurrentRuns int     `json:"maxConcurrentRuns"`
	DailyRuns         int     `json:"dailyRuns"`
	DailyTokens       int64   `json:"dailyTokens"`
	DailyCostUSD      float64 `json:"dailyCostUsd"`
	MaxToolTurns      int     `json:"maxToolTurns"`
}

// Usage is one aggregated daily row.
type Usage struct {
	Day          time.Time
	Scope        string
	ScopeKey     string
	Runs         int64
	InputTokens  int64
	OutputTokens int64
	CostUSD      float64
}

// UsageEvent is one model call's accounted usage. Step is unique within a run
// attempt, making settlement idempotent across retries.
type UsageEvent struct {
	RunID        uint64
	AttemptID    string
	Step         int
	AgentID      int64
	ActorUserID  int64
	Topic        string
	ModelID      uint64
	ModelKey     string
	Provider     string
	InputTokens  int
	OutputTokens int
	CostUSD      float64
}

// QuotaError is a terminal quota denial. It is safe to expose its message to
// the end user; RetryAt tells the client when the quota resets.
type QuotaError struct {
	Scope      string
	Metric     string
	Limit      float64
	Used       float64
	RetryAfter time.Duration
	RetryAt    time.Time
	Message    string
}

func (e *QuotaError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "quota exceeded"
}

// BudgetError is a terminal cost/token budget denial. It covers both Nonoka
// daily budgets and Bifrost hard budgets (HTTP 402).
type BudgetError struct {
	Scope      string
	Metric     string
	Limit      float64
	Used       float64
	RetryAfter time.Duration
	RetryAt    time.Time
	Message    string
}

func (e *BudgetError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "budget exceeded"
}

// RateLimitError is a provider/gateway rate limit (HTTP 429).
type RateLimitError struct {
	RetryAfter time.Duration
	Message    string
}

func (e *RateLimitError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "rate limited"
}
