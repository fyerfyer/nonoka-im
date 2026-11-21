package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"nonoka-im/internal/agent/app/admin"
	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/authz"
	"nonoka-im/internal/httpapi"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// AgentHTTP is the thin transport adapter for agent management resources.
// Business rules and persistence live in internal/agent/app/admin.
type AgentHTTP struct {
	agents    *admin.AgentService
	runs      *admin.RunService
	approvals *admin.ApprovalService
	mcp       *admin.MCPService
	skills    *admin.SkillService
	idem      *httpapi.Idempotency
}

func NewAgentHTTP(agents *admin.AgentService, runs *admin.RunService, approvals *admin.ApprovalService, mcp *admin.MCPService, skills *admin.SkillService, idem *httpapi.Idempotency) *AgentHTTP {
	return &AgentHTTP{agents: agents, runs: runs, approvals: approvals, mcp: mcp, skills: skills, idem: idem}
}

type agentRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Instruction string   `json:"instruction"`
	Model       string   `json:"model"`
	Tools       []string `json:"tools"`
	BaseURL     string   `json:"base_url"`
	APIKey      string   `json:"api_key"`
	MaxTokens   int      `json:"max_tokens"`
	MCPBindings []struct {
		Server string `json:"server"`
		Tool   string `json:"tool"`
	} `json:"mcp_bindings"`
	Skills []struct {
		Name    string `json:"name"`
		Version int64  `json:"version"`
	} `json:"skills"`
	Revision int64  `json:"revision"`
	Status   string `json:"status"`
	Kind     string `json:"kind"`
	Target   string `json:"target"`
	Enabled  bool   `json:"enabled"`
}

func (r *agentRequest) input() admin.Input {
	input := admin.Input{
		Name: r.Name, Description: r.Description, Instruction: r.Instruction, Model: r.Model,
		Tools: r.Tools, BaseURL: r.BaseURL, APIKey: r.APIKey, MaxTokens: r.MaxTokens,
		Revision: r.Revision, Status: r.Status, Kind: r.Kind, Target: r.Target, Enabled: r.Enabled,
	}
	for _, binding := range r.MCPBindings {
		input.MCPBindings = append(input.MCPBindings, admin.MCPBindingInput{Server: binding.Server, Tool: binding.Tool})
	}
	for _, skill := range r.Skills {
		input.Skills = append(input.Skills, admin.SkillBindingInput{Name: skill.Name, Version: skill.Version})
	}
	return input
}

type agentInputError struct{ error }

func invalidAgentInput(format string, args ...any) error {
	return agentInputError{fmt.Errorf(format, args...)}
}

func parseAgentID(c khttp.Context) (int64, error) {
	id, err := parsePositiveInt64(c.Vars().Get("id"))
	if err != nil {
		return 0, invalidAgentInput("invalid agent id")
	}
	return id, nil
}

func parsePositiveInt64(raw string) (int64, error) {
	var id int64
	if _, err := fmt.Sscanf(raw, "%d", &id); err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid id")
	}
	return id, nil
}

func writeAgentJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s *AgentHTTP) handle(c khttp.Context, fn func(context.Context, http.ResponseWriter, *http.Request) error) error {
	h := c.Middleware(func(ctx context.Context, _ interface{}) (interface{}, error) {
		uid := extractUserIDFromContext(ctx)
		if uid <= 0 {
			httpapi.Write(c.Response(), ctx, &httpapi.Error{Status: http.StatusUnauthorized, Code: "UNAUTHORIZED", Message: "authentication required"})
			return nil, nil
		}
		if s.idem == nil {
			if err := fn(ctx, c.Response(), c.Request()); err != nil {
				httpapi.Write(c.Response(), ctx, agentHTTPError(err))
			}
			return nil, nil
		}
		s.idem.Execute(ctx, c.Response(), c.Request(), uid, func(w http.ResponseWriter) error {
			if err := fn(ctx, w, c.Request()); err != nil {
				return agentHTTPError(err)
			}
			return nil
		})
		return nil, nil
	})
	_, err := h(c, nil)
	return err
}

func decodeAgent(r *http.Request, dst *agentRequest) error {
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return agentInputError{fmt.Errorf("invalid request: %w", err)}
	}
	return nil
}

func agentHTTPError(err error) *httpapi.Error {
	var inputErr agentInputError
	switch {
	case errors.As(err, &inputErr):
		return httpapi.BadRequest("INVALID_ARGUMENT", err.Error())
	case errors.Is(err, admin.ErrInvalidInput):
		return httpapi.BadRequest("INVALID_ARGUMENT", err.Error())
	case errors.Is(err, admin.ErrAdminRequired):
		return httpapi.Forbidden("platform admin required")
	case errors.Is(err, admin.ErrForbidden), errors.Is(err, authz.ErrDenied):
		return httpapi.Forbidden("access denied")
	case errors.Is(err, admin.ErrNotFound):
		return httpapi.NotFound(err.Error())
	case errors.Is(err, domain.ErrDraftConflict):
		return httpapi.Conflict("DRAFT_CONFLICT", err.Error())
	case errors.Is(err, domain.ErrRunState):
		return httpapi.Conflict("RUN_STATE", err.Error())
	case errors.Is(err, domain.ErrApprovalExpired):
		return httpapi.Conflict("APPROVAL_EXPIRED", err.Error())
	case errors.Is(err, domain.ErrApprovalGone):
		return httpapi.Conflict("APPROVAL_NOT_PENDING", err.Error())
	case errors.Is(err, domain.ErrSideEffectCommitted):
		return httpapi.Conflict("SIDE_EFFECT_COMMITTED", "run already committed a side effect; retry requires acknowledgeSideEffect=true")
	case errors.Is(err, domain.ErrMCPConflict):
		return httpapi.Conflict("MCP_EXISTS", err.Error())
	case errors.Is(err, domain.ErrAgentNotFound), errors.Is(err, domain.ErrVersionNotFound),
		errors.Is(err, domain.ErrRunNotFound), errors.Is(err, domain.ErrApprovalNotFound),
		errors.Is(err, domain.ErrSkillNotFound):
		return httpapi.NotFound(err.Error())
	default:
		return httpapi.Internal(err)
	}
}

// rawJSON renders an empty snapshot column as an empty JSON array instead of
// an invalid empty RawMessage.
func rawJSON(s string) json.RawMessage {
	if s == "" {
		return json.RawMessage("[]")
	}
	return json.RawMessage(s)
}

func decodeJSONBody(r *http.Request, dst any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return agentInputError{fmt.Errorf("invalid request: %w", err)}
	}
	return nil
}
