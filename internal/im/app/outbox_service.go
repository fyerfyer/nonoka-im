package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"nonoka-im/internal/alert"
	"nonoka-im/internal/im/domain"
	"nonoka-im/internal/messaging"
	"nonoka-im/internal/metrics"

	"github.com/go-kratos/kratos/v2/log"
)

const (
	defaultOutboxMaxAttempts = 50
	defaultOutboxPruneLimit  = 500
	defaultOutboxRetention   = 24 * time.Hour
)

// OutboxService relays the transactional agent-event outbox to Kafka and
// exposes the admin operations used by the DLQ panel.
type OutboxService struct {
	Repo      OutboxRepository
	Publisher messaging.EventPublisher
	Metrics   *metrics.Metrics
	Log       *log.Helper

	MaxAttempts int
	Retention   time.Duration
	PruneLimit  int
	Backoff     time.Duration

	// Notifier is optional; when set, a failed row or an old backlog emits a
	// generic alert with a cooldown to avoid notification storms.
	Notifier         alert.Notifier
	AlertMinInterval time.Duration
	AlertMaxAge      time.Duration
	lastAlert        time.Time
}

func (s *OutboxService) log() *log.Helper {
	if s.Log != nil {
		return s.Log
	}
	return log.NewHelper(log.DefaultLogger)
}

func (s *OutboxService) retention() time.Duration {
	if s.Retention > 0 {
		return s.Retention
	}
	return defaultOutboxRetention
}

func (s *OutboxService) maxAttempts() int {
	if s.MaxAttempts > 0 {
		return s.MaxAttempts
	}
	return defaultOutboxMaxAttempts
}

func (s *OutboxService) backoff(attempts int) time.Duration {
	base := s.Backoff
	if base <= 0 {
		base = time.Second
	}
	if attempts < 1 {
		attempts = 1
	}
	delay := base * time.Duration(1<<min(attempts-1, 8))
	if delay > 5*time.Minute {
		return 5 * time.Minute
	}
	return delay
}

// Run relays due rows and publishes backlog gauges until ctx is canceled.
func (s *OutboxService) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := s.Relay(ctx, 32); err != nil {
				s.log().Warnf("relay agent outbox failed: %v", err)
			}
			if pending, failed, age, err := s.Repo.OutboxStats(ctx); err == nil {
				if s.Metrics != nil {
					s.Metrics.SetAgentOutboxStats(pending, failed, age.Seconds())
				}
				s.maybeAlert(ctx, pending, failed, age)
			}
		}
	}
}

// Relay claims and publishes up to limit due rows.
func (s *OutboxService) Relay(ctx context.Context, limit int) (int, error) {
	if s.Publisher == nil {
		return 0, fmt.Errorf("agent event publisher is nil")
	}
	if limit <= 0 {
		limit = 32
	}
	processed := 0
	for i := 0; i < limit; i++ {
		row, err := s.Repo.ClaimOutbox(ctx)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return processed, nil
			}
			return processed, err
		}
		if row.Attempts > s.maxAttempts() {
			s.log().Errorf("agent outbox row %s exceeded %d attempts; marking failed (topic=%s seq=%d last_error=%s)",
				row.ID, s.maxAttempts(), row.Topic, row.TopicSeq, row.LastError)
			if s.Metrics != nil {
				s.Metrics.IncAgentOutboxPublishFailure()
			}
			if err := s.Repo.AbandonOutbox(ctx, row.ID); err != nil {
				return processed, fmt.Errorf("abandon exhausted agent event %s: %w", row.ID, err)
			}
			continue
		}
		var ev messaging.MessageEvent
		if err := unmarshalEvent(row.Payload, &ev); err != nil {
			_ = s.Repo.FailOutbox(ctx, row.ID, err, s.backoff(row.Attempts))
			return processed, fmt.Errorf("decode agent outbox event: %w", err)
		}
		if err := s.Publisher.Publish(ctx, ev); err != nil {
			if s.Metrics != nil {
				s.Metrics.IncAgentOutboxPublishFailure()
			}
			if releaseErr := s.Repo.FailOutbox(ctx, row.ID, err, s.backoff(row.Attempts)); releaseErr != nil {
				return processed, fmt.Errorf("publish agent event: %w (release: %v)", err, releaseErr)
			}
			return processed, err
		}
		if err := s.Repo.MarkOutboxPublished(ctx, row.ID); err != nil {
			return processed, err
		}
		if err := s.Repo.PrunePublishedOutbox(ctx); err != nil {
			s.log().Warnf("prune published agent events: %v", err)
		}
		processed++
	}
	return processed, nil
}

func unmarshalEvent(payload []byte, ev *messaging.MessageEvent) error {
	return json.Unmarshal(payload, ev)
}

// ListFailed returns terminal rows for the admin DLQ view.
func (s *OutboxService) ListFailed(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	return s.Repo.ListFailedOutbox(ctx, limit)
}

// Replay moves one failed row back to pending.
func (s *OutboxService) Replay(ctx context.Context, id string) error {
	return s.Repo.ReplayOutbox(ctx, id)
}

// ReplayMany moves the selected failed rows back to pending. Replaying is
// idempotent: the event_id and run uniqueness constraints absorb duplicates.
func (s *OutboxService) ReplayMany(ctx context.Context, ids []string) (int, error) {
	return s.Repo.ReplayOutboxMany(ctx, ids)
}

func (s *OutboxService) maybeAlert(ctx context.Context, pending, failed int64, age time.Duration) {
	if s.Notifier == nil {
		return
	}
	maxAge := s.AlertMaxAge
	if maxAge <= 0 {
		maxAge = 5 * time.Minute
	}
	cooldown := s.AlertMinInterval
	if cooldown <= 0 {
		cooldown = 15 * time.Minute
	}
	if failed == 0 && age < maxAge {
		return
	}
	if !s.lastAlert.IsZero() && time.Since(s.lastAlert) < cooldown {
		return
	}
	s.lastAlert = time.Now()
	if err := s.Notifier.Notify(ctx, alert.Event{
		Type: "agent_outbox", Pending: pending, Failed: failed,
		OldestAgeSeconds: age.Seconds(), At: time.Now().UTC(),
	}); err != nil {
		s.log().Warnf("agent outbox alert: %v", err)
	}
}

// Stats returns the outbox backlog gauges.
func (s *OutboxService) Stats(ctx context.Context) (pending, failed int64, oldestAge time.Duration, err error) {
	return s.Repo.OutboxStats(ctx)
}
