package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	agentv1 "nonoka-im/api/agent/v1"
	"nonoka-im/internal/agent/app"
	"nonoka-im/internal/agent/app/admin"
	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/audit"
	"nonoka-im/internal/authz"
	"nonoka-im/internal/httpapi"
	"nonoka-im/internal/llm"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"go.opentelemetry.io/otel/trace"
)

// AgentHTTP is the thin transport adapter for agent management resources.
// Business rules and persistence live in internal/agent/app/admin.
type AgentHTTP struct {
	agentv1.UnimplementedAgentRunServiceServer
	agentv1.UnimplementedAgentServiceServer

	agents    *admin.AgentService
	runs      *admin.RunService
	approvals *admin.ApprovalService
	mcp       *admin.MCPService
	skills    *admin.SkillService
	models    *llm.Service
	quota     *app.QuotaService
	auditor   *audit.Service
	idem      *httpapi.Idempotency
}

func NewAgentHTTP(agents *admin.AgentService, runs *admin.RunService, approvals *admin.ApprovalService, mcp *admin.MCPService, skills *admin.SkillService, models *llm.Service, quota *app.QuotaService, auditor *audit.Service, idem *httpapi.Idempotency) *AgentHTTP {
	return &AgentHTTP{agents: agents, runs: runs, approvals: approvals, mcp: mcp, skills: skills, models: models, quota: quota, auditor: auditor, idem: idem}
}

type agentRequest struct {
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	Instruction       string            `json:"instruction"`
	ModelID           int64             `json:"modelId"`
	Tools             []string          `json:"tools"`
	CapabilityPresets []string          `json:"capabilityPresets"`
	ToolPolicies      map[string]string `json:"toolPolicies"`
	MaxTokens         int               `json:"maxTokens"`
	MCPBindings       []struct {
		ServerID uint64 `json:"serverId"`
		Tool     string `json:"tool"`
		Policy   string `json:"policy"`
	} `json:"mcpBindings"`
	Skills []struct {
		SkillID uint64 `json:"skillId"`
		Version int64  `json:"version"`
	} `json:"skills"`
	Revision int64  `json:"revision"`
	Status   string `json:"status"`
	Kind     string `json:"kind"`
	Target   string `json:"target"`
	Role     string `json:"role"`
	Enabled  bool   `json:"enabled"`
	Note     string `json:"note"`
	Version  int64  `json:"version"`
}

func (r *agentRequest) input() admin.Input {
	input := admin.Input{
		Name: r.Name, Description: r.Description, Instruction: r.Instruction,
		ModelID: r.ModelID,
		Tools:   r.Tools, CapabilityPresets: r.CapabilityPresets, ToolPolicies: r.ToolPolicies,
		MaxTokens: r.MaxTokens, Revision: r.Revision, Status: r.Status, Kind: r.Kind,
		Target: r.Target, Role: r.Role, Enabled: r.Enabled, Note: r.Note,
		Version: r.Version,
	}
	for _, binding := range r.MCPBindings {
		input.MCPBindings = append(input.MCPBindings, admin.MCPBindingInput{ServerID: binding.ServerID, Tool: binding.Tool, Policy: binding.Policy})
	}
	for _, skill := range r.Skills {
		input.Skills = append(input.Skills, admin.SkillBindingInput{SkillID: skill.SkillID, Version: skill.Version})
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
		ctx = audit.WithRequestMeta(ctx, requestMeta(ctx, c.Request()))
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

func requestMeta(ctx context.Context, r *http.Request) audit.RequestMeta {
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("TRUST_PROXY_HEADERS")), "true") {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			ip = strings.TrimSpace(strings.Split(forwarded, ",")[0])
		}
	}
	traceID := ""
	if sc := trace.SpanContextFromContext(ctx); sc.HasTraceID() {
		traceID = sc.TraceID().String()
	}
	return audit.RequestMeta{
		RequestID: httpapi.RequestID(ctx), TraceID: traceID, IP: ip,
		UserAgent: r.UserAgent(),
	}
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
	case errors.Is(err, admin.ErrInvalidInput), errors.Is(err, domain.ErrInvalidSkill):
		return httpapi.BadRequest("INVALID_ARGUMENT", err.Error())
	case errors.Is(err, domain.ErrInvalidMCPTarget):
		return httpapi.BadRequest("INVALID_MCP_TARGET", err.Error())
	case errors.Is(err, admin.ErrAdminRequired):
		return httpapi.Forbidden("platform admin required")
	case errors.Is(err, app.ErrForbidden):
		return httpapi.Forbidden("platform admin required")
	case errors.Is(err, app.ErrInvalidInput):
		return httpapi.BadRequest("INVALID_ARGUMENT", err.Error())
	case errors.Is(err, admin.ErrNeedsAuth), errors.Is(err, domain.ErrMCPAuthRequired):
		return httpapi.Conflict("MCP_AUTH_REQUIRED", err.Error())
	case errors.Is(err, admin.ErrForbidden), errors.Is(err, authz.ErrDenied), errors.Is(err, llm.ErrForbidden):
		return httpapi.Forbidden("access denied")
	case errors.Is(err, admin.ErrNotFound), errors.Is(err, llm.ErrNotFound):
		return httpapi.NotFound(err.Error())
	case errors.Is(err, llm.ErrInvalidInput):
		return httpapi.BadRequest("INVALID_ARGUMENT", err.Error())
	case errors.Is(err, domain.ErrModelNotFound):
		return httpapi.NotFound(err.Error())
	case errors.Is(err, domain.ErrModelDisabled), errors.Is(err, domain.ErrModelNotCurated):
		return httpapi.BadRequest("MODEL_UNAVAILABLE", err.Error())
	case errors.Is(err, domain.ErrGatewayKeyNotFound):
		return httpapi.Conflict("GATEWAY_KEY_MISSING", "agent gateway key is not configured; publish the agent first")
	case errors.Is(err, llm.ErrGatewayRejected), errors.Is(err, llm.ErrGatewayUnavailable):
		return &httpapi.Error{Status: http.StatusBadGateway, Code: "MODEL_GATEWAY_ERROR", Message: err.Error()}
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
	case errors.Is(err, admin.ErrConflict), errors.Is(err, domain.ErrMCPConflict):
		return httpapi.Conflict("MCP_EXISTS", err.Error())
	case errors.Is(err, domain.ErrSkillConflict):
		return httpapi.Conflict("SKILL_EXISTS", err.Error())
	case errors.Is(err, domain.ErrMCPReviewRequired):
		return httpapi.Conflict("MCP_NOT_APPROVED", err.Error())
	case errors.Is(err, domain.ErrAgentNotFound), errors.Is(err, domain.ErrVersionNotFound),
		errors.Is(err, domain.ErrRunNotFound), errors.Is(err, domain.ErrApprovalNotFound),
		errors.Is(err, domain.ErrSkillNotFound), errors.Is(err, domain.ErrSkillVersionNotFound),
		errors.Is(err, domain.ErrMCPNotFound), errors.Is(err, domain.ErrMCPToolNotFound),
		errors.Is(err, domain.ErrMCPConnectionNotFound):
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
