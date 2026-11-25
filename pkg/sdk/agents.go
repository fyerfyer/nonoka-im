package sdk

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// AgentInput is the editable agent configuration used by create and draft
// updates. Revision is required for updates and ignored on create.
type AgentInput struct {
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	Instruction       string            `json:"instruction"`
	ModelID           int64             `json:"modelId"`
	Tools             []string          `json:"tools"`
	CapabilityPresets []string          `json:"capabilityPresets"`
	ToolPolicies      map[string]string `json:"toolPolicies"`
	MaxTokens         int               `json:"maxTokens"`
	MCPBindings       []MCPBinding      `json:"mcpBindings"`
	Skills            []SkillBinding    `json:"skills"`
	Revision          int64             `json:"revision,omitempty"`
}

type AgentsResource struct {
	http *apiTransport
}

func (r *AgentsResource) List(ctx context.Context, opts ListOptions) (*Page[Agent], error) {
	query := url.Values{}
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	query.Set("limit", strconv.Itoa(limit))
	if opts.Cursor != "" {
		query.Set("cursor", opts.Cursor)
	}
	var page Page[Agent]
	if err := r.http.doJSON(ctx, http.MethodGet, "/v1/agents", query, nil, &page, true); err != nil {
		return nil, err
	}
	return &page, nil
}

func (r *AgentsResource) Get(ctx context.Context, id int64) (*Agent, error) {
	var agent Agent
	if err := r.http.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/agents/%d", id), nil, nil, &agent, true); err != nil {
		return nil, err
	}
	return &agent, nil
}

func (r *AgentsResource) Create(ctx context.Context, input AgentInput) (*Agent, error) {
	var agent Agent
	if err := r.http.doJSON(ctx, http.MethodPost, "/v1/agents", nil, input, &agent, false); err != nil {
		return nil, err
	}
	return &agent, nil
}

func (r *AgentsResource) UpdateDraft(ctx context.Context, id int64, input AgentInput) (*Agent, error) {
	var agent Agent
	if err := r.http.doJSON(ctx, http.MethodPut, fmt.Sprintf("/v1/agents/%d/draft", id), nil, input, &agent, false); err != nil {
		return nil, err
	}
	return &agent, nil
}

func (r *AgentsResource) Publish(ctx context.Context, id, revision int64, note string) (*AgentVersion, error) {
	var version AgentVersion
	body := map[string]any{"revision": revision, "note": note}
	if err := r.http.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/agents/%d/publish", id), nil, body, &version, false); err != nil {
		return nil, err
	}
	return &version, nil
}

func (r *AgentsResource) Restore(ctx context.Context, id, version, revision int64) (*Agent, error) {
	var agent Agent
	body := map[string]any{"version": version, "revision": revision}
	if err := r.http.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/agents/%d/restore", id), nil, body, &agent, false); err != nil {
		return nil, err
	}
	return &agent, nil
}

func (r *AgentsResource) SetStatus(ctx context.Context, id int64, status string) error {
	body := map[string]any{"status": status}
	return r.http.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/agents/%d/status", id), nil, body, nil, false)
}

func (r *AgentsResource) Delete(ctx context.Context, id int64) error {
	return r.http.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/v1/agents/%d", id), nil, nil, nil, false)
}

func (r *AgentsResource) Shares(ctx context.Context, id int64) ([]AgentShare, error) {
	var reply struct {
		Shares []AgentShare `json:"shares"`
	}
	if err := r.http.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/agents/%d/shares", id), nil, nil, &reply, true); err != nil {
		return nil, err
	}
	return reply.Shares, nil
}

func (r *AgentsResource) Share(ctx context.Context, id int64, kind, target, role string, enabled bool) error {
	body := map[string]any{"kind": kind, "target": target, "role": role, "enabled": enabled}
	return r.http.doJSON(ctx, http.MethodPut, fmt.Sprintf("/v1/agents/%d/shares", id), nil, body, nil, false)
}

func (r *AgentsResource) Versions(ctx context.Context, id int64) ([]AgentVersionSummary, error) {
	var reply struct {
		Versions []AgentVersionSummary `json:"versions"`
	}
	if err := r.http.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/agents/%d/versions", id), nil, nil, &reply, true); err != nil {
		return nil, err
	}
	return reply.Versions, nil
}

func (r *AgentsResource) Version(ctx context.Context, id, version int64) (*AgentVersion, error) {
	var out AgentVersion
	if err := r.http.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/agents/%d/versions/%d", id, version), nil, nil, &out, true); err != nil {
		return nil, err
	}
	return &out, nil
}

// Test starts a playground run against the current draft. It never publishes
// the agent or writes chat messages.
func (r *AgentsResource) Tools(ctx context.Context) ([]CapabilityPreset, []ToolDefinition, error) {
	var reply struct {
		Presets []CapabilityPreset `json:"presets"`
		Tools   []ToolDefinition   `json:"tools"`
	}
	if err := r.http.doJSON(ctx, http.MethodGet, "/v1/agent-tools", nil, nil, &reply, true); err != nil {
		return nil, nil, err
	}
	return reply.Presets, reply.Tools, nil
}

func (r *AgentsResource) Test(ctx context.Context, id int64, message string) (*AgentRun, error) {
	var run AgentRun
	body := map[string]any{"message": message}
	if err := r.http.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/agents/%d/test", id), nil, body, &run, false); err != nil {
		return nil, err
	}
	return &run, nil
}

// Runs returns one page of an agent's run history.
func (r *AgentsResource) Runs(ctx context.Context, id int64, opts ListOptions) (*Page[AgentRun], error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	query := url.Values{}
	query.Set("limit", strconv.Itoa(limit))
	if opts.Cursor != "" {
		query.Set("cursor", opts.Cursor)
	}
	var page Page[AgentRun]
	if err := r.http.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/agents/%d/runs", id), query, nil, &page, true); err != nil {
		return nil, err
	}
	return &page, nil
}
