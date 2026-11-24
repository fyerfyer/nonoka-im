package sdk

import (
	"context"
	"fmt"
	"net/http"
)

type MCPServerInput struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url"`
	Transport   string `json:"transport,omitempty"`
	AuthType    string `json:"authType,omitempty"`
	HeaderName  string `json:"headerName,omitempty"`
	Credential  string `json:"credential,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
	Enabled     *bool  `json:"enabled,omitempty"`
}

type MCPConnectionInput struct {
	AuthType   string `json:"authType,omitempty"`
	HeaderName string `json:"headerName,omitempty"`
	Credential string `json:"credential"`
}

type MCPResource struct {
	http *apiTransport
}

func (r *MCPResource) Servers(ctx context.Context) ([]MCPServer, error) {
	var reply struct {
		Servers []MCPServer `json:"servers"`
	}
	if err := r.http.doJSON(ctx, http.MethodGet, "/v1/mcp/servers", nil, nil, &reply, true); err != nil {
		return nil, err
	}
	return reply.Servers, nil
}

func (r *MCPResource) Server(ctx context.Context, id uint64) (*MCPServer, error) {
	var out MCPServer
	if err := r.http.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/mcp/servers/%d", id), nil, nil, &out, true); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *MCPResource) Register(ctx context.Context, input MCPServerInput) (*MCPServer, error) {
	var out MCPServer
	if err := r.http.doJSON(ctx, http.MethodPost, "/v1/mcp/servers", nil, input, &out, false); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *MCPResource) Update(ctx context.Context, id uint64, input MCPServerInput) (*MCPServer, error) {
	var out MCPServer
	if err := r.http.doJSON(ctx, http.MethodPatch, fmt.Sprintf("/v1/mcp/servers/%d", id), nil, input, &out, false); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *MCPResource) Delete(ctx context.Context, id uint64) error {
	return r.http.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/v1/mcp/servers/%d", id), nil, nil, nil, false)
}

func (r *MCPResource) Discover(ctx context.Context, id uint64) ([]MCPTool, error) {
	var reply struct {
		Tools []MCPTool `json:"tools"`
	}
	if err := r.http.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/mcp/servers/%d/discover", id), nil, nil, &reply, false); err != nil {
		return nil, err
	}
	return reply.Tools, nil
}

func (r *MCPResource) Test(ctx context.Context, id uint64) error {
	return r.http.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/mcp/servers/%d/test", id), nil, nil, nil, false)
}

func (r *MCPResource) SetEnabled(ctx context.Context, id uint64, enabled bool) error {
	body := map[string]any{"enabled": enabled}
	return r.http.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/mcp/servers/%d/status", id), nil, body, nil, false)
}

func (r *MCPResource) Review(ctx context.Context, id uint64, status, reason string) error {
	body := map[string]any{"status": status, "reason": reason}
	return r.http.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/mcp/servers/%d/review", id), nil, body, nil, false)
}

func (r *MCPResource) Tools(ctx context.Context, id uint64) ([]MCPTool, error) {
	var reply struct {
		Tools []MCPTool `json:"tools"`
	}
	if err := r.http.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/mcp/servers/%d/tools", id), nil, nil, &reply, true); err != nil {
		return nil, err
	}
	return reply.Tools, nil
}

func (r *MCPResource) Connections(ctx context.Context) ([]MCPConnection, error) {
	var reply struct {
		Connections []MCPConnection `json:"connections"`
	}
	if err := r.http.doJSON(ctx, http.MethodGet, "/v1/mcp/connections", nil, nil, &reply, true); err != nil {
		return nil, err
	}
	return reply.Connections, nil
}

func (r *MCPResource) Connect(ctx context.Context, serverID uint64, input MCPConnectionInput) (*MCPConnection, error) {
	var out MCPConnection
	if err := r.http.doJSON(ctx, http.MethodPut, fmt.Sprintf("/v1/mcp/connections/%d", serverID), nil, input, &out, false); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *MCPResource) Disconnect(ctx context.Context, serverID uint64) error {
	return r.http.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/v1/mcp/connections/%d", serverID), nil, nil, nil, false)
}
