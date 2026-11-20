package sweeper

import (
	"context"
	"time"
)

// Store is the recovery surface used by the sweeper. The PostgreSQL adapter
// implements it; the sweeper never imports data.
type Store interface {
	SweepStaleRuns(ctx context.Context, maxAttempts int) (requeued, failed int64, err error)
	ExpirePendingApprovals(ctx context.Context, now time.Time) ([]uint64, error)
	ClaimWaitingRun(ctx context.Context, id uint64) error
	PruneExpiredCheckpoints(ctx context.Context, now time.Time, limit int) (int64, error)
}
