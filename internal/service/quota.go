package service

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"nonoka-im/internal/agent/app"
	"nonoka-im/internal/agent/domain"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

type QuotaView struct {
	Scope             string    `json:"scope"`
	ScopeKey          string    `json:"scopeKey"`
	MaxConcurrentRuns *int      `json:"maxConcurrentRuns,omitempty"`
	DailyRuns         *int      `json:"dailyRuns,omitempty"`
	DailyTokens       *int64    `json:"dailyTokens,omitempty"`
	DailyCostUSD      *float64  `json:"dailyCostUsd,omitempty"`
	MaxToolTurns      *int      `json:"maxToolTurns,omitempty"`
	UpdatedBy         int64     `json:"updatedBy,omitempty"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type quotaLimitsRequest struct {
	MaxConcurrentRuns int     `json:"maxConcurrentRuns"`
	DailyRuns         int     `json:"dailyRuns"`
	DailyTokens       int64   `json:"dailyTokens"`
	DailyCostUSD      float64 `json:"dailyCostUsd"`
	MaxToolTurns      int     `json:"maxToolTurns"`
}

type UsageView struct {
	Day          string  `json:"day"`
	Scope        string  `json:"scope"`
	ScopeKey     string  `json:"scopeKey"`
	Runs         int64   `json:"runs"`
	InputTokens  int64   `json:"inputTokens"`
	OutputTokens int64   `json:"outputTokens"`
	CostUSD      float64 `json:"costUsd,omitempty"`
}

func (s *AgentHTTP) ListQuotas(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		scope := c.Query().Get("scope")
		limit, _ := strconv.Atoi(c.Query().Get("limit"))
		offset, _ := strconv.Atoi(c.Query().Get("offset"))
		rows, err := s.quota.AdminList(ctx, extractUserIDFromContext(ctx), scope, limit, offset)
		if err != nil {
			return err
		}
		out := make([]QuotaView, 0, len(rows))
		for i := range rows {
			out = append(out, quotaView(rows[i]))
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{
			"items":    out,
			"defaults": quotaDefaults(s.quota.Cfg),
		})
		return nil
	})
}

func (s *AgentHTTP) UpdateQuota(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		scope := c.Vars().Get("scope")
		key := c.Vars().Get("key")
		var input quotaLimitsRequest
		if err := decodeJSONBody(r, &input); err != nil {
			return err
		}
		row, err := s.quota.AdminUpdate(ctx, extractUserIDFromContext(ctx), scope, key, domain.QuotaLimits{
			MaxConcurrentRuns: input.MaxConcurrentRuns,
			DailyRuns:         input.DailyRuns,
			DailyTokens:       input.DailyTokens,
			DailyCostUSD:      input.DailyCostUSD,
			MaxToolTurns:      input.MaxToolTurns,
		})
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, quotaView(*row))
		return nil
	})
}

func (s *AgentHTTP) ResetQuota(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		if err := s.quota.AdminReset(ctx, extractUserIDFromContext(ctx), c.Vars().Get("scope"), c.Vars().Get("key")); err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]string{"status": "reset"})
		return nil
	})
}

func (s *AgentHTTP) TopUsage(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		limit, _ := strconv.Atoi(c.Query().Get("limit"))
		scope := c.Query().Get("scope")
		if scope == "" {
			scope = domain.QuotaScopeAgent
		}
		rows, err := s.quota.AdminTop(ctx, extractUserIDFromContext(ctx), scope, c.Query().Get("metric"), limit)
		if err != nil {
			return err
		}
		out := make([]UsageView, 0, len(rows))
		for i := range rows {
			out = append(out, usageView(rows[i], true))
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"items": out})
		return nil
	})
}

func (s *AgentHTTP) AgentUsage(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := parseAgentID(c)
		if err != nil {
			return err
		}
		uid := extractUserIDFromContext(ctx)
		_, _, manager, _, err := s.agents.Detail(ctx, uid, id)
		if err != nil {
			return err
		}
		days, _ := strconv.Atoi(c.Query().Get("days"))
		if days <= 0 {
			days = 7
		}
		rows, total, err := s.quota.Usage(ctx, domain.QuotaScopeAgent, strconv.FormatInt(id, 10), days)
		if err != nil {
			return err
		}
		out := make([]UsageView, 0, len(rows))
		for i := range rows {
			out = append(out, usageView(rows[i], manager))
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{
			"days": out, "total": usageView(total, manager), "canSeeCost": manager,
		})
		return nil
	})
}

func quotaView(q domain.Quota) QuotaView {
	return QuotaView{
		Scope: q.Scope, ScopeKey: q.ScopeKey, MaxConcurrentRuns: q.MaxConcurrentRuns,
		DailyRuns: q.DailyRuns, DailyTokens: q.DailyTokens, DailyCostUSD: q.DailyCostUSD,
		MaxToolTurns: q.MaxToolTurns, UpdatedBy: q.UpdatedBy, UpdatedAt: q.UpdatedAt,
	}
}

func quotaDefaults(cfg app.QuotaConfig) map[string]domain.QuotaLimits {
	return map[string]domain.QuotaLimits{
		domain.QuotaScopeUser:  cfg.User,
		domain.QuotaScopeAgent: cfg.Agent,
	}
}

func usageView(u domain.Usage, includeCost bool) UsageView {
	v := UsageView{
		Day: u.Day.Format("2006-01-02"), Scope: u.Scope, ScopeKey: u.ScopeKey,
		Runs: u.Runs, InputTokens: u.InputTokens, OutputTokens: u.OutputTokens,
	}
	if includeCost {
		v.CostUSD = u.CostUSD
	}
	return v
}
