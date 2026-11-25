package worker

import (
	"context"
	"time"

	"nonoka-im/internal/agent/domain"
)

// RunQueue is the replay loop's narrow persistence surface.
type RunQueue interface {
	QueuedRuns(ctx context.Context, limit int) ([]domain.AgentRun, error)
	QueueStats(ctx context.Context) (queued int64, oldestAge time.Duration, err error)
}

// DispatcherStore resolves trigger targets and records queued runs.
// DispatcherAuthorizer is the narrow topic authorization surface used while
// dispatching trigger events.
type DispatcherAuthorizer interface {
	CanAccessTopic(ctx context.Context, userID int64, topic string) error
}

type DispatcherStore interface {
	ActiveAgentsByBot(ctx context.Context, ids []int64) ([]domain.Agent, error)
	EnqueueRun(ctx context.Context, candidate *domain.AgentRun) (bool, error)
}
