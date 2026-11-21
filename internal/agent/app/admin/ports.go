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
)

// Store is the agent persistence surface used by management usecases.
type Store interface {
	ListAgents(ctx context.Context, limit, offset int) ([]domain.Agent, int64, error)
	GetAgent(ctx context.Context, id int64) (*domain.Agent, error)
	CreateAgent(ctx context.Context, ownerID int64, spec domain.AgentSpec, grant func(context.Context, int64) error) (*domain.Agent, error)
	UpdateDraft(ctx context.Context, id, revision int64, spec domain.AgentSpec, updateKey bool) (*domain.Agent, error)
	Publish(ctx context.Context, id, revision int64) (*domain.AgentVersion, error)
	RollbackPublish(ctx context.Context, id, version int64) error
	SetAgentStatus(ctx context.Context, id int64, status string) error
	DeleteAgent(ctx context.Context, id int64) ([]string, error)
	GetVersion(ctx context.Context, agentID, version int64) (*domain.AgentVersion, error)
	ListRunsPage(ctx context.Context, agentID int64, limit, offset int) ([]domain.AgentRun, int64, error)
	GetRun(ctx context.Context, id uint64) (*domain.AgentRun, error)
	SetRunAction(ctx context.Context, id uint64, action string, acknowledgeSideEffect bool) error
	ListApprovals(ctx context.Context, approverID int64, status string, limit, offset int) ([]domain.AgentApproval, int64, error)
	GetApproval(ctx context.Context, id uint64) (*domain.AgentApproval, error)
	DecideAndRequeue(ctx context.Context, id uint64, decision string, actorID int64) error
}

// PlatformStore is the MCP/skill registry surface.
type PlatformStore interface {
	RegisterServer(ctx context.Context, name, transport, url, tools, risk string) (*domain.MCPServer, error)
	GetServer(ctx context.Context, name string) (*domain.MCPServer, error)
	ListServers(ctx context.Context) ([]domain.MCPServer, error)
	SetServerEnabled(ctx context.Context, id uint64, enabled bool) error
	PublishSkill(ctx context.Context, name string, version int64, content string) (*domain.Skill, error)
	GetSkill(ctx context.Context, name string, version int64) (*domain.Skill, error)
	ListSkills(ctx context.Context, publishedOnly bool) ([]domain.Skill, error)
}

// Authorizer is the narrow OpenFGA admin surface.
type Authorizer interface {
	CanViewAgent(ctx context.Context, userID, agentID int64) error
	CanUseAgent(ctx context.Context, userID, agentID int64) error
	CanManageAgent(ctx context.Context, userID, agentID int64) error
	CanAccessTopic(ctx context.Context, userID int64, topic string) error
	CanManageGroup(ctx context.Context, userID int64, groupID string) error
	SetAgentOwner(ctx context.Context, agentID, userID int64) error
	SetAgentToolBinding(ctx context.Context, agentID int64, tool string, enabled bool) error
	SetAgentShare(ctx context.Context, agentID int64, kind, target string, enabled bool) error
	ListAgentShares(ctx context.Context, agentID int64) ([]authz.AgentShare, error)
	DeleteAgentTuples(ctx context.Context, agentID int64, tools []string) error
	SetGroupMember(ctx context.Context, groupID string, userID int64, enabled bool) error
}

// GroupDirectory is the group mutation surface used when sharing an agent.
type GroupDirectory interface {
	AddMember(ctx context.Context, groupID string, userID int64) error
}

// UserDirectory resolves target users for shares.
type UserDirectory interface {
	UserExists(ctx context.Context, userID int64) (bool, error)
}

// APIKeySealer seals model credentials at rest.
type APIKeySealer interface {
	Seal(apiKey string) (sealed string, update bool, err error)
}

// Input is the transport-neutral agent draft request.
type Input struct {
	Name        string
	Description string
	Instruction string
	Model       string
	Tools       []string
	BaseURL     string
	APIKey      string
	MaxTokens   int
	MCPBindings []MCPBindingInput
	Skills      []SkillBindingInput
	Revision    int64
	Status      string
	Kind        string
	Target      string
	Enabled     bool
}

type MCPBindingInput struct {
	Server string
	Tool   string
}

type SkillBindingInput struct {
	Name    string
	Version int64
}
