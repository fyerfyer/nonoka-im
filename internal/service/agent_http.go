package service

import (
	"encoding/json"
	"errors"
	"net/http"

	agentv1 "nonoka-im/api/agent/v1"
	"nonoka-im/internal/agent/app"
	"nonoka-im/internal/agent/app/admin"
	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/audit"
	"nonoka-im/internal/authz"
	"nonoka-im/internal/httpapi"
	"nonoka-im/internal/llm"
)

// AgentHTTP is the thin transport adapter for agent management resources.
// Business rules and persistence live in internal/agent/app/admin.
type AgentHTTP struct {
	agentv1.UnimplementedAgentRunServiceServer
	agentv1.UnimplementedAgentServiceServer
	agentv1.UnimplementedAgentRegistryServiceServer
	agentv1.UnimplementedAgentQuotaServiceServer
	agentv1.UnimplementedAgentAuditServiceServer

	agents    *admin.AgentService
	runs      *admin.RunService
	approvals *admin.ApprovalService
	mcp       *admin.MCPService
	skills    *admin.SkillService
	models    *llm.Service
	quota     *app.QuotaService
	auditor   *audit.Service
}

func NewAgentHTTP(agents *admin.AgentService, runs *admin.RunService, approvals *admin.ApprovalService, mcp *admin.MCPService, skills *admin.SkillService, models *llm.Service, quota *app.QuotaService, auditor *audit.Service) *AgentHTTP {
	return &AgentHTTP{agents: agents, runs: runs, approvals: approvals, mcp: mcp, skills: skills, models: models, quota: quota, auditor: auditor}
}

func writeAgentJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func agentHTTPError(err error) *httpapi.Error {
	switch {
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
