package worker

import (
	"context"

	"nonoka-im/internal/agent/domain"
)

// RunQueue is the replay loop's narrow persistence surface.
type RunQueue interface {
	QueuedRuns(ctx context.Context, limit int) ([]domain.AgentRun, error)
}

// DispatcherStore resolves trigger targets and records queued runs.
type DispatcherStore interface {
	ActiveAgentsByBot(ctx context.Context, ids []int64) ([]domain.Agent, error)
	EnqueueRun(ctx context.Context, candidate *domain.AgentRun) (bool, error)
}
