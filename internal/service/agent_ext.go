package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// Admin endpoints for the MCP registry and skill publishing. Platform admins
// are configured through AGENT_ADMIN_USER_IDS; without any configured admin,
// all write operations are refused.

func (s *AgentHTTP) requireAdmin(ctx context.Context, w http.ResponseWriter) (int64, bool) {
	uid := extractUserIDFromContext(ctx)
	if uid <= 0 || !isAgentAdmin(uid) {
		writeAgentJSON(w, http.StatusForbidden, map[string]string{"code": "FORBIDDEN", "message": "platform admin required"})
		return 0, false
	}
	return uid, true
}

// decodeJSONBody decodes a small strict JSON request body (64 KiB cap,
// unknown fields rejected) into dst.
func decodeJSONBody(r *http.Request, dst any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return agentInputError{fmt.Errorf("invalid request: %w", err)}
	}
	return nil
}

type mcpServerRequest struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Tools   []string `json:"tools"`
	Risk    string   `json:"risk"`
	Enabled bool     `json:"enabled"`
}

func (s *AgentHTTP) RegisterMCPServer(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		if _, ok := s.requireAdmin(ctx, w); !ok {
			return nil
		}
		var input mcpServerRequest
		if err := decodeJSONBody(r, &input); err != nil {
			return err
		}
		input.Name = strings.TrimSpace(input.Name)
		if input.Name == "" || len(input.Name) > 100 || strings.ContainsAny(input.Name, " \t\n/") {
			return invalidAgentInput("invalid server name")
		}
		u, err := url.Parse(strings.TrimSpace(input.URL))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return invalidAgentInput("url must be an http(s) URL")
		}
		if len(input.Tools) == 0 || len(input.Tools) > 100 {
			return invalidAgentInput("tools must list 1-100 allowed tool names")
		}
		seen := map[string]bool{}
		for _, name := range input.Tools {
			if name == "" || strings.ContainsAny(name, " \t\n") || seen[name] {
				return invalidAgentInput("invalid or duplicate tool name %q", name)
			}
			seen[name] = true
		}
		if input.Risk == "" {
			input.Risk = "read_only"
		}
		if input.Risk != "read_only" && input.Risk != "write" {
			return invalidAgentInput("risk must be read_only or write")
		}
		toolsJSON, _ := json.Marshal(input.Tools)
		row, err := s.ext.RegisterServer(ctx, input.Name, "http", u.String(), string(toolsJSON), input.Risk)
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusCreated, map[string]any{"id": row.ID, "name": row.Name, "url": row.URL, "tools": json.RawMessage(row.AllowedTools), "risk": row.RiskClass, "enabled": row.Enabled})
		return nil
	})
}

func (s *AgentHTTP) ListMCPServers(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		if _, ok := s.requireAdmin(ctx, w); !ok {
			return nil
		}
		rows, err := s.ext.ListServers(ctx)
		if err != nil {
			return err
		}
		servers := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			servers = append(servers, map[string]any{"id": row.ID, "name": row.Name, "url": row.URL, "tools": rawJSON(row.AllowedTools), "risk": row.RiskClass, "enabled": row.Enabled})
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"servers": servers})
		return nil
	})
}

func (s *AgentHTTP) SetMCPServerStatus(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		if _, ok := s.requireAdmin(ctx, w); !ok {
			return nil
		}
		id, err := strconv.ParseUint(c.Vars().Get("id"), 10, 64)
		if err != nil || id == 0 {
			return invalidAgentInput("invalid server id")
		}
		var input mcpServerRequest
		if err := decodeJSONBody(r, &input); err != nil {
			return err
		}
		if err := s.ext.SetServerEnabled(ctx, id, input.Enabled); err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"id": id, "enabled": input.Enabled})
		return nil
	})
}

type skillRequest struct {
	Name    string `json:"name"`
	Version int64  `json:"version"`
	Content string `json:"content"`
}

func (s *AgentHTTP) PublishSkill(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		if _, ok := s.requireAdmin(ctx, w); !ok {
			return nil
		}
		var input skillRequest
		if err := decodeJSONBody(r, &input); err != nil {
			return err
		}
		input.Name = strings.TrimSpace(input.Name)
		if input.Name == "" || len(input.Name) > 100 || strings.ContainsAny(input.Name, " \t\n/") {
			return invalidAgentInput("invalid skill name")
		}
		if input.Version <= 0 {
			return invalidAgentInput("invalid skill version")
		}
		if input.Content == "" || len(input.Content) > 200000 || !strings.Contains(input.Content, "name:") {
			return invalidAgentInput("skill content must be a SKILL.md document")
		}
		row, err := s.ext.PublishSkill(ctx, input.Name, input.Version, input.Content)
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusCreated, map[string]any{"id": row.ID, "name": row.Name, "version": row.Version})
		return nil
	})
}

func (s *AgentHTTP) ListSkills(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		rows, err := s.ext.ListSkills(ctx, true)
		if err != nil {
			return err
		}
		skills := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			skills = append(skills, map[string]any{"name": row.Name, "version": row.Version, "published": row.Published, "createdAt": row.CreatedAt})
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"skills": skills})
		return nil
	})
}
