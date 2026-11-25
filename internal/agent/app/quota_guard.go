package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/port"
)

// RunGuard settles model usage and aborts a run before the next model/tool
// call when a daily budget or tool-turn cap is exhausted.
type RunGuard struct {
	quota   *QuotaService
	run     *domain.AgentRun
	model   port.ModelEndpoint
	kind    string
	day     time.Time
	retryAt time.Time

	userLimits    domain.QuotaLimits
	agentLimits   domain.QuotaLimits
	dbUserTokens  int64
	dbUserCost    float64
	dbAgentTokens int64
	dbAgentCost   float64
	maxTurns      int

	mu         sync.Mutex
	step       int
	last       port.Usage
	usedTokens int64
	usedCost   float64
}

func (s *QuotaService) BeginRun(ctx context.Context, run *domain.AgentRun, model port.ModelEndpoint, kind string) (*RunGuard, int, error) {
	if run == nil {
		return nil, 0, fmt.Errorf("%w: run is required", ErrInvalidInput)
	}
	userLimits, agentLimits, err := s.Effective(ctx, run.ActorUserID, run.AgentID)
	if err != nil {
		return nil, 0, err
	}
	day := s.Day(time.Now())
	userUsage, err := s.Store.GetUsage(ctx, day, domain.QuotaScopeUser, key(run.ActorUserID))
	if err != nil {
		return nil, 0, err
	}
	agentUsage, err := s.Store.GetUsage(ctx, day, domain.QuotaScopeAgent, key(run.AgentID))
	if err != nil {
		return nil, 0, err
	}
	startUserTokens := userUsage.InputTokens + userUsage.OutputTokens
	startAgentTokens := agentUsage.InputTokens + agentUsage.OutputTokens
	startUserCost := userUsage.CostUSD
	startAgentCost := agentUsage.CostUSD
	maxTurns := agentLimits.MaxToolTurns
	if maxTurns <= 0 || (userLimits.MaxToolTurns > 0 && userLimits.MaxToolTurns < maxTurns) {
		maxTurns = userLimits.MaxToolTurns
	}
	guard := &RunGuard{
		quota: s, run: run, model: model, kind: kind, day: day,
		retryAt: s.NextReset(time.Now()), userLimits: userLimits, agentLimits: agentLimits,
		dbUserTokens: startUserTokens, dbUserCost: startUserCost,
		dbAgentTokens: startAgentTokens, dbAgentCost: startAgentCost,
		maxTurns: maxTurns,
	}
	return guard, maxTurns, nil
}

func (g *RunGuard) BeforeModel(ctx context.Context, turn int) error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.maxTurns > 0 && turn > g.maxTurns {
		return &domain.QuotaError{
			Scope: domain.QuotaScopeAgent, Metric: "tool_turns", Limit: float64(g.maxTurns), Used: float64(turn),
			RetryAfter: 0, RetryAt: time.Time{},
			Message: fmt.Sprintf("This run reached its maximum of %d model turns.", g.maxTurns),
		}
	}
	g.day = g.quota.Day(time.Now())
	g.retryAt = g.quota.NextReset(time.Now())
	return g.refreshAndCheckLocked(ctx)
}

func (g *RunGuard) AfterModel(ctx context.Context, cumulative port.Usage) error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	deltaIn := cumulative.InputTokens - g.last.InputTokens
	deltaOut := cumulative.OutputTokens - g.last.OutputTokens
	if deltaIn < 0 {
		deltaIn = 0
	}
	if deltaOut < 0 {
		deltaOut = 0
	}
	if deltaIn == 0 && deltaOut == 0 {
		return nil
	}
	g.last = cumulative
	g.step++
	cost := (float64(deltaIn)*g.model.InputPricePerMTok + float64(deltaOut)*g.model.OutputPricePerMTok) / 1_000_000
	ev := domain.UsageEvent{
		RunID: g.run.ID, AttemptID: g.run.AttemptID, Step: g.step, Kind: g.kind,
		AgentID: g.run.AgentID, ActorUserID: g.run.ActorUserID, Topic: g.run.Topic,
		ModelID: g.model.ModelID, ModelKey: g.model.ModelKey, Provider: g.model.Provider,
		InputTokens: deltaIn, OutputTokens: deltaOut, CostUSD: cost,
	}
	settleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if _, err := g.quota.RecordUsage(settleCtx, ev); err != nil {
		return fmt.Errorf("record usage: %w", err)
	}
	g.usedTokens += int64(deltaIn + deltaOut)
	g.usedCost += cost
	g.day = g.quota.Day(time.Now())
	g.retryAt = g.quota.NextReset(time.Now())
	return g.refreshAndCheckLocked(settleCtx)
}

func (g *RunGuard) refreshAndCheckLocked(ctx context.Context) error {
	userUsage, err := g.quota.Store.GetUsage(ctx, g.day, domain.QuotaScopeUser, key(g.run.ActorUserID))
	if err != nil {
		return fmt.Errorf("refresh user usage: %w", err)
	}
	agentUsage, err := g.quota.Store.GetUsage(ctx, g.day, domain.QuotaScopeAgent, key(g.run.AgentID))
	if err != nil {
		return fmt.Errorf("refresh agent usage: %w", err)
	}
	g.dbUserTokens = userUsage.InputTokens + userUsage.OutputTokens
	g.dbUserCost = userUsage.CostUSD
	g.dbAgentTokens = agentUsage.InputTokens + agentUsage.OutputTokens
	g.dbAgentCost = agentUsage.CostUSD
	return g.checkLocked()
}

func (g *RunGuard) BeforeTool(ctx context.Context, _ string) error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.day = g.quota.Day(time.Now())
	g.retryAt = g.quota.NextReset(time.Now())
	return g.refreshAndCheckLocked(ctx)
}

func (g *RunGuard) checkLocked() error {
	if g.userLimits.DailyTokens > 0 && g.dbUserTokens >= g.userLimits.DailyTokens {
		return g.budgetError(domain.QuotaScopeUser, "tokens", float64(g.userLimits.DailyTokens), float64(g.dbUserTokens))
	}
	if g.userLimits.DailyCostUSD > 0 && g.dbUserCost >= g.userLimits.DailyCostUSD {
		return g.budgetError(domain.QuotaScopeUser, "cost", g.userLimits.DailyCostUSD, g.dbUserCost)
	}
	if g.agentLimits.DailyTokens > 0 && g.dbAgentTokens >= g.agentLimits.DailyTokens {
		return g.budgetError(domain.QuotaScopeAgent, "tokens", float64(g.agentLimits.DailyTokens), float64(g.dbAgentTokens))
	}
	if g.agentLimits.DailyCostUSD > 0 && g.dbAgentCost >= g.agentLimits.DailyCostUSD {
		return g.budgetError(domain.QuotaScopeAgent, "cost", g.agentLimits.DailyCostUSD, g.dbAgentCost)
	}
	return nil
}

func (g *RunGuard) budgetError(scope, metric string, limit, used float64) error {
	return &domain.BudgetError{
		Scope: scope, Metric: metric, Limit: limit, Used: used,
		RetryAt: g.retryAt, RetryAfter: time.Until(g.retryAt),
		Message: fmt.Sprintf("%s daily %s budget exhausted (%s/%s). Try again after %s.",
			scope, metric, formatAmount(used), formatAmount(limit), g.retryAt.Format("2006-01-02 15:04 MST")),
	}
}

func formatAmount(v float64) string {
	if v < 0.01 {
		return fmt.Sprintf("%.6f", v)
	}
	return fmt.Sprintf("%.2f", v)
}
