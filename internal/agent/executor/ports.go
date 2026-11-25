package executor

import (
	"context"
	"time"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/port"
)

// Store is the persistence surface needed to claim and execute one run. The
// concrete PostgreSQL adapter implements it; the executor never imports data.
type Store interface {
	ClaimRun(ctx context.Context, runID uint64, lease time.Duration) (*domain.AgentRun, bool, error)
	Heartbeat(ctx context.Context, id uint64, attemptID string, lease time.Duration) (bool, error)
	GetAgent(ctx context.Context, id int64) (*domain.Agent, error)
	GetVersion(ctx context.Context, agentID, version int64) (*domain.AgentVersion, error)
	GetRun(ctx context.Context, id uint64) (*domain.AgentRun, error)
	LoadRunState(ctx context.Context, run *domain.AgentRun) (*domain.AgentSession, *domain.AgentContextSnapshot, []domain.AgentTurn, error)
	ApprovalForRun(ctx context.Context, runID uint64) (*domain.AgentApproval, error)
	ConsumeApproval(ctx context.Context, id uint64, attemptID string) (string, bool, error)
	UpdateRunFenced(ctx context.Context, id uint64, attemptID string, values map[string]interface{}) error
	ReleaseRun(ctx context.Context, id uint64, attemptID string) (bool, error)
	DeleteCheckpoint(ctx context.Context, checkpointID string) error
	FailRun(ctx context.Context, run *domain.AgentRun, failure domain.Failure, maxAttempts int, update map[string]interface{}) error
}

// Registry resolves MCP server/tool records for version resolution.
type Registry interface {
	GetServerByID(ctx context.Context, id uint64) (*domain.MCPServer, error)
	GetTool(ctx context.Context, serverID uint64, name string) (*domain.MCPTool, error)
}

// ModelResolver resolves the platform model reference frozen into an agent
// version plus the per-agent or playground virtual key. It must reject
// disabled/uncurated models and never fall back to a global provider
// credential.
type ModelResolver interface {
	ResolveForRun(ctx context.Context, agentID, modelID int64) (port.ModelEndpoint, error)
}

// Authorizer is the subset of the authorization boundary the executor needs.
type Authorizer interface {
	CanAccessTopic(ctx context.Context, userID int64, topic string) error
	CanUseAgent(ctx context.Context, userID, agentID int64) error
}
