package executor

import (
	"context"
	"time"

	"nonoka-im/internal/agent/domain"
)

// Store is the persistence surface needed to claim and execute one run. The
// concrete PostgreSQL adapter implements it; the executor never imports data.
type Store interface {
	ClaimRun(ctx context.Context, runID uint64, lease time.Duration) (*domain.AgentRun, bool, error)
	Heartbeat(ctx context.Context, id uint64, attemptID string, lease time.Duration) (bool, error)
	GetAgent(ctx context.Context, id int64) (*domain.Agent, error)
	GetVersion(ctx context.Context, agentID, version int64) (*domain.AgentVersion, error)
	LoadRunState(ctx context.Context, run *domain.AgentRun) (*domain.AgentSession, *domain.AgentContextSnapshot, []domain.AgentTurn, error)
	ApprovalForRun(ctx context.Context, runID uint64) (*domain.AgentApproval, error)
	ConsumeApproval(ctx context.Context, id uint64, attemptID string) (string, bool, error)
	UpdateRunFenced(ctx context.Context, id uint64, attemptID string, values map[string]interface{}) error
	DeleteCheckpoint(ctx context.Context, checkpointID string) error
	FailRun(ctx context.Context, run *domain.AgentRun, failure domain.Failure, maxAttempts int, update map[string]interface{}) error
}

// Registry resolves MCP server records for version resolution.
type Registry interface {
	GetServer(ctx context.Context, name string) (*domain.MCPServer, error)
}

// Authorizer is the subset of the authorization boundary the executor needs.
type Authorizer interface {
	CanAccessTopic(ctx context.Context, userID int64, topic string) error
	CanUseAgent(ctx context.Context, userID, agentID int64) error
}
