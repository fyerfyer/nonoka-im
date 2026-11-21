package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"nonoka-im/internal/agent/app/admin"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

type mcpServerRequest struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Tools   []string `json:"tools"`
	Risk    string   `json:"risk"`
	Enabled bool     `json:"enabled"`
}

func (s *AgentHTTP) RegisterMCPServer(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		var input mcpServerRequest
		if err := decodeJSONBody(r, &input); err != nil {
			return err
		}
		row, err := s.mcp.Register(ctx, extractUserIDFromContext(ctx), admin.MCPInput{
			Name: input.Name, URL: input.URL, Tools: input.Tools, Risk: input.Risk, Enabled: input.Enabled,
		})
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusCreated, map[string]any{
			"id": row.ID, "name": row.Name, "url": row.URL,
			"tools": json.RawMessage(row.AllowedTools), "risk": row.RiskClass, "enabled": row.Enabled,
		})
		return nil
	})
}

func (s *AgentHTTP) ListMCPServers(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		rows, err := s.mcp.List(ctx, extractUserIDFromContext(ctx))
		if err != nil {
			return err
		}
		servers := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			servers = append(servers, map[string]any{
				"id": row.ID, "name": row.Name, "url": row.URL,
				"tools": rawJSON(row.AllowedTools), "risk": row.RiskClass, "enabled": row.Enabled,
			})
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"servers": servers})
		return nil
	})
}

func (s *AgentHTTP) SetMCPServerStatus(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := strconv.ParseUint(c.Vars().Get("id"), 10, 64)
		if err != nil || id == 0 {
			return invalidAgentInput("invalid server id")
		}
		var input mcpServerRequest
		if err := decodeJSONBody(r, &input); err != nil {
			return err
		}
		if err := s.mcp.SetStatus(ctx, extractUserIDFromContext(ctx), id, input.Enabled); err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"id": id, "enabled": input.Enabled})
		return nil
	})
}
