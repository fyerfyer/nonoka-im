package service

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"nonoka-im/internal/agent/app/admin"
	"nonoka-im/internal/agent/domain"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

type MCPServerView struct {
	ID            uint64        `json:"id"`
	OwnerUserID   int64         `json:"ownerUserId"`
	Name          string        `json:"name"`
	DisplayName   string        `json:"displayName"`
	Description   string        `json:"description"`
	URL           string        `json:"url"`
	Transport     string        `json:"transport"`
	AuthType      string        `json:"authType"`
	HasCredential bool          `json:"hasCredential"`
	Visibility    string        `json:"visibility"`
	ReviewStatus  string        `json:"reviewStatus"`
	ReviewReason  string        `json:"reviewReason,omitempty"`
	HealthStatus  string        `json:"healthStatus"`
	LastCheckedAt *time.Time    `json:"lastCheckedAt,omitempty"`
	LastError     string        `json:"lastError,omitempty"`
	Enabled       bool          `json:"enabled"`
	ToolsRevision int64         `json:"toolsRevision"`
	Tools         []MCPToolView `json:"tools,omitempty"`
	CreatedAt     time.Time     `json:"createdAt"`
	UpdatedAt     time.Time     `json:"updatedAt"`
}

type MCPToolView struct {
	ID          uint64 `json:"id"`
	ServerID    uint64 `json:"serverId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Risk        string `json:"risk"`
	SchemaHash  string `json:"schemaHash"`
	InputSchema string `json:"inputSchema,omitempty"`
	Annotations string `json:"annotations,omitempty"`
	Enabled     bool   `json:"enabled"`
}

type MCPConnectionView struct {
	ID            uint64     `json:"id"`
	ServerID      uint64     `json:"serverId"`
	AuthType      string     `json:"authType"`
	Status        string     `json:"status"`
	HasCredential bool       `json:"hasCredential"`
	ExpiresAt     *time.Time `json:"expiresAt,omitempty"`
	LastUsedAt    *time.Time `json:"lastUsedAt,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

type mcpServerRequest struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Transport   string `json:"transport"`
	AuthType    string `json:"authType"`
	HeaderName  string `json:"headerName"`
	Credential  string `json:"credential"`
	Visibility  string `json:"visibility"`
	Enabled     *bool  `json:"enabled"`
}

type mcpConnectionRequest struct {
	AuthType   string `json:"authType"`
	HeaderName string `json:"headerName"`
	Credential string `json:"credential"`
}

type mcpReviewRequest struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

func (s *AgentHTTP) ListMCPServers(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		rows, err := s.mcp.List(ctx, extractUserIDFromContext(ctx))
		if err != nil {
			return err
		}
		servers := make([]MCPServerView, 0, len(rows))
		for i := range rows {
			tools, _ := s.mcp.ListTools(ctx, extractUserIDFromContext(ctx), rows[i].ID)
			servers = append(servers, mcpServerView(&rows[i], tools))
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"servers": servers})
		return nil
	})
}

func (s *AgentHTTP) GetMCPServer(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := parseUintVar(c, "id", "invalid server id")
		if err != nil {
			return err
		}
		server, err := s.mcp.Get(ctx, extractUserIDFromContext(ctx), id)
		if err != nil {
			return err
		}
		tools, err := s.mcp.ListTools(ctx, extractUserIDFromContext(ctx), id)
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, mcpServerView(server, tools))
		return nil
	})
}

func (s *AgentHTTP) RegisterMCPServer(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		var input mcpServerRequest
		if err := decodeJSONBody(r, &input); err != nil {
			return err
		}
		enabled := true
		if input.Enabled != nil {
			enabled = *input.Enabled
		}
		server, err := s.mcp.Register(ctx, extractUserIDFromContext(ctx), admin.MCPInput{
			Name: input.Name, DisplayName: input.DisplayName, Description: input.Description,
			URL: input.URL, Transport: input.Transport, AuthType: input.AuthType,
			HeaderName: input.HeaderName, Credential: input.Credential,
			Visibility: input.Visibility, Enabled: enabled,
		})
		if err != nil {
			return err
		}
		tools, _ := s.mcp.ListTools(ctx, extractUserIDFromContext(ctx), server.ID)
		writeAgentJSON(w, http.StatusCreated, mcpServerView(server, tools))
		return nil
	})
}

func (s *AgentHTTP) UpdateMCPServer(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := parseUintVar(c, "id", "invalid server id")
		if err != nil {
			return err
		}
		var input mcpServerRequest
		if err := decodeJSONBody(r, &input); err != nil {
			return err
		}
		enabled := true
		if input.Enabled != nil {
			enabled = *input.Enabled
		}
		server, err := s.mcp.Update(ctx, extractUserIDFromContext(ctx), id, admin.MCPInput{
			Name: input.Name, DisplayName: input.DisplayName, Description: input.Description,
			URL: input.URL, Transport: input.Transport, AuthType: input.AuthType,
			HeaderName: input.HeaderName, Credential: input.Credential,
			Visibility: input.Visibility, Enabled: enabled,
		})
		if err != nil {
			return err
		}
		tools, _ := s.mcp.ListTools(ctx, extractUserIDFromContext(ctx), id)
		writeAgentJSON(w, http.StatusOK, mcpServerView(server, tools))
		return nil
	})
}

func (s *AgentHTTP) DeleteMCPServer(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := parseUintVar(c, "id", "invalid server id")
		if err != nil {
			return err
		}
		if err := s.mcp.Delete(ctx, extractUserIDFromContext(ctx), id); err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		return nil
	})
}

func (s *AgentHTTP) DiscoverMCPServer(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := parseUintVar(c, "id", "invalid server id")
		if err != nil {
			return err
		}
		tools, err := s.mcp.Discover(ctx, extractUserIDFromContext(ctx), id)
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"tools": mcpToolViews(tools)})
		return nil
	})
}

func (s *AgentHTTP) TestMCPServer(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := parseUintVar(c, "id", "invalid server id")
		if err != nil {
			return err
		}
		if err := s.mcp.Test(ctx, extractUserIDFromContext(ctx), id); err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
		return nil
	})
}

func (s *AgentHTTP) SetMCPServerStatus(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := parseUintVar(c, "id", "invalid server id")
		if err != nil {
			return err
		}
		var input mcpServerRequest
		if err := decodeJSONBody(r, &input); err != nil {
			return err
		}
		if input.Enabled == nil {
			return invalidAgentInput("enabled is required")
		}
		if err := s.mcp.SetStatus(ctx, extractUserIDFromContext(ctx), id, *input.Enabled); err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"id": id, "enabled": *input.Enabled})
		return nil
	})
}

func (s *AgentHTTP) ReviewMCPServer(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := parseUintVar(c, "id", "invalid server id")
		if err != nil {
			return err
		}
		var input mcpReviewRequest
		if err := decodeJSONBody(r, &input); err != nil {
			return err
		}
		if err := s.mcp.Review(ctx, extractUserIDFromContext(ctx), id, input.Status, input.Reason); err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"id": id, "reviewStatus": input.Status})
		return nil
	})
}

func (s *AgentHTTP) ListMCPTools(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := parseUintVar(c, "id", "invalid server id")
		if err != nil {
			return err
		}
		tools, err := s.mcp.ListTools(ctx, extractUserIDFromContext(ctx), id)
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"tools": mcpToolViews(tools)})
		return nil
	})
}

func (s *AgentHTTP) ListMCPConnections(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		rows, err := s.mcp.ListConnections(ctx, extractUserIDFromContext(ctx))
		if err != nil {
			return err
		}
		out := make([]MCPConnectionView, 0, len(rows))
		for i := range rows {
			row := &rows[i]
			out = append(out, MCPConnectionView{
				ID: row.ID, ServerID: row.ServerID, AuthType: row.AuthType, Status: row.Status,
				HasCredential: row.CredentialEnc != "", ExpiresAt: row.ExpiresAt,
				LastUsedAt: row.LastUsedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
			})
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"connections": out})
		return nil
	})
}

func (s *AgentHTTP) ConnectMCPServer(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := parseUintVar(c, "id", "invalid server id")
		if err != nil {
			return err
		}
		var input mcpConnectionRequest
		if err := decodeJSONBody(r, &input); err != nil {
			return err
		}
		conn, err := s.mcp.Connect(ctx, extractUserIDFromContext(ctx), id, admin.MCPConnectionInput{
			AuthType: input.AuthType, HeaderName: input.HeaderName, Credential: input.Credential,
		})
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, MCPConnectionView{
			ID: conn.ID, ServerID: conn.ServerID, AuthType: conn.AuthType, Status: conn.Status,
			HasCredential: conn.CredentialEnc != "", ExpiresAt: conn.ExpiresAt,
			LastUsedAt: conn.LastUsedAt, CreatedAt: conn.CreatedAt, UpdatedAt: conn.UpdatedAt,
		})
		return nil
	})
}

func (s *AgentHTTP) DisconnectMCPServer(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := parseUintVar(c, "id", "invalid server id")
		if err != nil {
			return err
		}
		if err := s.mcp.Disconnect(ctx, extractUserIDFromContext(ctx), id); err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
		return nil
	})
}

func parseUintVar(c khttp.Context, name, message string) (uint64, error) {
	id, err := strconv.ParseUint(c.Vars().Get(name), 10, 64)
	if err != nil || id == 0 {
		return 0, invalidAgentInput("%s", message)
	}
	return id, nil
}

func mcpServerView(server *domain.MCPServer, tools []domain.MCPTool) MCPServerView {
	return MCPServerView{
		ID: server.ID, OwnerUserID: server.OwnerUserID, Name: server.Name, DisplayName: server.DisplayName,
		Description: server.Description, URL: server.URL, Transport: server.Transport,
		AuthType: server.AuthType, HasCredential: server.CredentialEnc != "",
		Visibility: server.Visibility, ReviewStatus: server.ReviewStatus, ReviewReason: server.ReviewReason,
		HealthStatus: server.HealthStatus, LastCheckedAt: server.LastCheckedAt, LastError: server.LastError,
		Enabled: server.Enabled, ToolsRevision: server.ToolsRevision, Tools: mcpToolViews(tools),
		CreatedAt: server.CreatedAt, UpdatedAt: server.UpdatedAt,
	}
}

func mcpToolViews(rows []domain.MCPTool) []MCPToolView {
	out := make([]MCPToolView, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		out = append(out, MCPToolView{
			ID: row.ID, ServerID: row.ServerID, Name: row.Name, Description: row.Description,
			Risk: row.Risk, SchemaHash: row.SchemaHash, InputSchema: row.InputSchema,
			Annotations: row.Annotations, Enabled: row.Enabled,
		})
	}
	return out
}
