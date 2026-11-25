// Package admin contains the agent platform management usecases. Transport
// adapters (internal/service) depend on this package instead of the data layer.
package admin

import (
	"context"
	"errors"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/authz"
)

var (
	ErrInvalidInput  = errors.New("invalid input")
	ErrAdminRequired = errors.New("platform admin required")
	ErrForbidden     = errors.New("forbidden")
	ErrNotFound      = errors.New("not found")
	ErrConflict      = errors.New("already exists")
	ErrNeedsAuth     = errors.New("mcp connection required")
)

// Store is the agent persistence surface used by management usecases.
type Store interface {
	ListAgents(ctx context.Context, limit, offset int) ([]domain.Agent, int64, error)
	GetAgent(ctx context.Context, id int64) (*domain.Agent, error)
	CreateAgent(ctx context.Context, ownerID int64, spec domain.AgentSpec, grant func(context.Context, int64) error) (*domain.Agent, error)
	UpdateDraft(ctx context.Context, id, revision int64, spec domain.AgentSpec, updateKey bool) (*domain.Agent, error)
	Publish(ctx context.Context, id, revision int64, note string, publishedBy int64) (*domain.AgentVersion, error)
	RollbackPublish(ctx context.Context, id, version int64) error
	RestoreDraft(ctx context.Context, id, version, expectedRevision int64) (*domain.Agent, error)
	SetAgentStatus(ctx context.Context, id int64, status string) error
	DeleteAgent(ctx context.Context, id int64) ([]string, error)
	GetVersion(ctx context.Context, agentID, version int64) (*domain.AgentVersion, error)
	ListVersions(ctx context.Context, agentID int64) ([]domain.AgentVersion, error)
	EnqueueRun(ctx context.Context, candidate *domain.AgentRun) (bool, error)
	ListRunsPage(ctx context.Context, agentID int64, limit, offset int) ([]domain.AgentRun, int64, error)
	GetRun(ctx context.Context, id uint64) (*domain.AgentRun, error)
	GetRunByReplyMsgID(ctx context.Context, msgID int64) (*domain.AgentRun, error)
	GetRunByReplyClientMsgID(ctx context.Context, clientMsgID string) (*domain.AgentRun, error)
	ListRunCitations(ctx context.Context, runID uint64) ([]domain.AgentRunCitation, error)
	SetRunAction(ctx context.Context, id uint64, action string, acknowledgeSideEffect bool) error
	ListToolInvocations(ctx context.Context, runID uint64) ([]domain.ToolInvocation, error)
	ListRunSteps(ctx context.Context, runID uint64) ([]domain.AgentRunStep, error)
	ListApprovals(ctx context.Context, approverID int64, status string, limit, offset int) ([]domain.AgentApproval, int64, error)
	GetApproval(ctx context.Context, id uint64) (*domain.AgentApproval, error)
	DecideAndRequeue(ctx context.Context, id uint64, decision string, actorID int64) error
}

// PlatformStore is the MCP/skill registry surface.
type PlatformStore interface {
	// MCP servers/tools.
	CreateServer(ctx context.Context, spec domain.MCPServer) (*domain.MCPServer, error)
	GetServerByID(ctx context.Context, id uint64) (*domain.MCPServer, error)
	ListServersForUser(ctx context.Context, userID int64, includeAll bool) ([]domain.MCPServer, error)
	ListAllServers(ctx context.Context, enabledOnly bool) ([]domain.MCPServer, error)
	UpdateServer(ctx context.Context, server domain.MCPServer) error
	SetServerEnabled(ctx context.Context, id uint64, enabled bool) error
	SetServerReview(ctx context.Context, id uint64, status, reason string, reviewer int64) error
	SetServerHealth(ctx context.Context, id uint64, status, lastErr string) error
	DeleteServer(ctx context.Context, id uint64) error
	ReplaceTools(ctx context.Context, serverID uint64, tools []domain.MCPTool) (bool, error)
	ListTools(ctx context.Context, serverID uint64) ([]domain.MCPTool, error)
	GetTool(ctx context.Context, serverID uint64, name string) (*domain.MCPTool, error)

	// MCP connections.
	UpsertConnection(ctx context.Context, conn domain.MCPConnection) (*domain.MCPConnection, error)
	GetConnection(ctx context.Context, serverID uint64, userID int64) (*domain.MCPConnection, error)
	ListConnections(ctx context.Context, userID int64) ([]domain.MCPConnection, error)
	DeleteConnection(ctx context.Context, serverID uint64, userID int64) error

	// Skills.
	CreateSkill(ctx context.Context, ownerID int64, slug, displayName, description, visibility, reviewStatus string) (*domain.Skill, error)
	GetSkillByID(ctx context.Context, id uint64) (*domain.Skill, error)
	GetSkillByOwnerSlug(ctx context.Context, ownerID int64, slug string) (*domain.Skill, error)
	ListSkillsForUser(ctx context.Context, userID int64, includeAll bool) ([]domain.Skill, error)
	UpdateSkillMeta(ctx context.Context, id uint64, displayName, description, visibility, reviewStatus string) error
	SetSkillReview(ctx context.Context, id uint64, status string) error
	DeleteSkill(ctx context.Context, id uint64) error
	CreateSkillVersion(ctx context.Context, version domain.SkillVersion) (*domain.SkillVersion, error)
	GetSkillVersion(ctx context.Context, skillID uint64, version int64) (*domain.SkillVersion, error)
	ListSkillVersions(ctx context.Context, skillID uint64) ([]domain.SkillVersion, error)
	SetSkillVersionReview(ctx context.Context, skillID uint64, version int64, status, reason string, reviewer int64) error
}

// Authorizer is the narrow OpenFGA admin surface.
type Authorizer interface {
	CanViewAgent(ctx context.Context, userID, agentID int64) error
	CanUseAgent(ctx context.Context, userID, agentID int64) error
	CanManageAgent(ctx context.Context, userID, agentID int64) error
	CanOwnAgent(ctx context.Context, userID, agentID int64) error
	CanManagePlatform(ctx context.Context, userID int64) error
	CanAccessTopic(ctx context.Context, userID int64, topic string) error
	CanManageGroup(ctx context.Context, userID int64, groupID string) error
	SetAgentOwner(ctx context.Context, agentID, userID int64) error
	SetAgentToolBinding(ctx context.Context, agentID int64, tool string, enabled bool) error
	SetAgentShare(ctx context.Context, agentID int64, kind, target, role string, enabled bool) error
	ListAgentShares(ctx context.Context, agentID int64) ([]authz.AgentShare, error)
	DeleteAgentTuples(ctx context.Context, agentID int64, tools []string) error
	SetGroupMember(ctx context.Context, groupID string, userID int64, enabled bool) error
}

// MCPDiscovery is implemented by the SSRF-safe MCP adapter.
type MCPDiscovery interface {
	Discover(ctx context.Context, target domain.MCPTarget) (*domain.MCPDiscoveryResult, error)
	Test(ctx context.Context, target domain.MCPTarget) error
}

// CredentialSealer encrypts MCP credentials at rest.
type CredentialSealer interface {
	Seal(plain string) (string, error)
	Open(sealed string) (string, error)
}

// GroupDirectory is the group mutation surface used when sharing an agent.
type GroupDirectory interface {
	AddMember(ctx context.Context, groupID string, userID int64) error
}

// UserDirectory resolves target users for shares.
type UserDirectory interface {
	UserExists(ctx context.Context, userID int64) (bool, error)
}

// ModelCatalog validates platform model references and resolves the model
// display metadata used by the management API.
type ModelCatalog interface {
	Lookup(ctx context.Context, modelID int64) (*domain.Model, error)
}

// GatewayKeys owns per-agent Bifrost virtual-key lifecycle.
type GatewayKeys interface {
	EnsureAgentKey(ctx context.Context, agentID int64, models []domain.Model) error
	RevokeAgentKey(ctx context.Context, agentID int64) error
	AgentKeyStatus(ctx context.Context, agentID int64) (string, error)
}

// Input is the transport-neutral agent draft request.
type Input struct {
	Name              string
	Description       string
	Instruction       string
	ModelID           int64
	Tools             []string
	CapabilityPresets []string
	ToolPolicies      map[string]string
	MaxTokens         int
	MCPBindings       []MCPBindingInput
	Skills            []SkillBindingInput
	Revision          int64
	Status            string
	Kind              string
	Target            string
	Role              string
	Enabled           bool
	Note              string
	Version           int64
}

type MCPBindingInput struct {
	ServerID uint64
	Tool     string
	Policy   string
}

type SkillBindingInput struct {
	SkillID uint64
	Version int64
}
