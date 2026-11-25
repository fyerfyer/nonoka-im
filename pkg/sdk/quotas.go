package sdk

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Quota struct {
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

type QuotaLimits struct {
	MaxConcurrentRuns int     `json:"maxConcurrentRuns"`
	DailyRuns         int     `json:"dailyRuns"`
	DailyTokens       int64   `json:"dailyTokens"`
	DailyCostUSD      float64 `json:"dailyCostUsd"`
	MaxToolTurns      int     `json:"maxToolTurns"`
}

type Usage struct {
	Day          string  `json:"day"`
	Scope        string  `json:"scope"`
	ScopeKey     string  `json:"scopeKey"`
	Runs         int64   `json:"runs"`
	InputTokens  int64   `json:"inputTokens"`
	OutputTokens int64   `json:"outputTokens"`
	CostUSD      float64 `json:"costUsd,omitempty"`
}

type QuotasResource struct {
	http *apiTransport
}

func (r *QuotasResource) List(ctx context.Context, scope string, limit, offset int) ([]Quota, map[string]QuotaLimits, error) {
	query := url.Values{}
	if scope != "" {
		query.Set("scope", scope)
	}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		query.Set("offset", strconv.Itoa(offset))
	}
	var reply struct {
		Items    []Quota                `json:"items"`
		Defaults map[string]QuotaLimits `json:"defaults"`
	}
	if err := r.http.doJSON(ctx, http.MethodGet, "/v1/admin/quotas", query, nil, &reply, true); err != nil {
		return nil, nil, err
	}
	return reply.Items, reply.Defaults, nil
}

func (r *QuotasResource) Update(ctx context.Context, scope, key string, limits QuotaLimits) (*Quota, error) {
	var out Quota
	path := fmt.Sprintf("/v1/admin/quotas/%s/%s", url.PathEscape(scope), url.PathEscape(key))
	if err := r.http.doJSON(ctx, http.MethodPut, path, nil, limits, &out, false); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *QuotasResource) Reset(ctx context.Context, scope, key string) error {
	path := fmt.Sprintf("/v1/admin/quotas/%s/%s", url.PathEscape(scope), url.PathEscape(key))
	return r.http.doJSON(ctx, http.MethodDelete, path, nil, nil, nil, false)
}

func (r *QuotasResource) Top(ctx context.Context, scope, metric string, limit int) ([]Usage, error) {
	query := url.Values{"scope": {scope}, "metric": {metric}, "limit": {strconv.Itoa(limit)}}
	var reply struct {
		Items []Usage `json:"items"`
	}
	if err := r.http.doJSON(ctx, http.MethodGet, "/v1/admin/usage/top", query, nil, &reply, true); err != nil {
		return nil, err
	}
	return reply.Items, nil
}
