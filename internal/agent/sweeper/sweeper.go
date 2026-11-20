// Package sweeper recovers abandoned runs, expires approvals and prunes
// expired checkpoints. It is intentionally independent from the dispatch and
// execution paths.
package sweeper

import (
	"context"
	"errors"
	"time"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/metrics"

	"github.com/go-kratos/kratos/v2/log"
)

type Sweeper struct {
	Store    Store
	Metrics  *metrics.Metrics
	Log      *log.Helper
	Interval time.Duration
	// MaxAttempts bounds how many times a run may be reclaimed before it is
	// marked failed instead of requeued.
	MaxAttempts int
}

func (s *Sweeper) Run(ctx context.Context) {
	interval := s.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Sweep(ctx)
		}
	}
}

// Sweep performs one pass. It is exported so integration tests can drive it
// deterministically instead of waiting for the ticker.
func (s *Sweeper) Sweep(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	start := time.Now()
	requeued, failed, err := s.Store.SweepStaleRuns(ctx, s.MaxAttempts)
	if s.Metrics != nil {
		s.Metrics.SweepsTotal.Inc()
		s.Metrics.SweepDuration.Observe(time.Since(start).Seconds())
	}
	if err != nil {
		s.log().Errorf("sweep stale runs: %v", err)
	} else {
		if s.Metrics != nil {
			s.Metrics.StaleRunsRequeuedTotal.Add(float64(requeued))
			s.Metrics.StaleRunsFailedTotal.Add(float64(failed))
		}
		if requeued > 0 || failed > 0 {
			s.log().Infof("swept stale runs: requeued=%d failed=%d", requeued, failed)
		}
	}

	// Expired approvals resolve as rejections: requeue the waiting run so the
	// resume path feeds the model the rejection result.
	runIDs, err := s.Store.ExpirePendingApprovals(ctx, time.Now())
	if err != nil {
		s.log().Errorf("expire pending approvals: %v", err)
	}
	for _, runID := range runIDs {
		if s.Metrics != nil {
			s.Metrics.ApprovalsTotal.WithLabelValues(domain.ApprovalExpired).Inc()
		}
		if err := s.Store.ClaimWaitingRun(ctx, runID); err != nil && !errors.Is(err, domain.ErrRunState) {
			s.log().Errorf("requeue run %d after approval expiry: %v", runID, err)
		}
	}

	if pruned, err := s.Store.PruneExpiredCheckpoints(ctx, time.Now(), 500); err != nil {
		s.log().Errorf("prune checkpoints: %v", err)
	} else if pruned > 0 {
		s.log().Infof("pruned %d expired checkpoints", pruned)
	}
}

func (s *Sweeper) log() *log.Helper {
	if s.Log != nil {
		return s.Log
	}
	return log.NewHelper(log.DefaultLogger)
}
