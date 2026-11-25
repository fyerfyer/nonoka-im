package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"nonoka-im/internal/im/app"
	"nonoka-im/internal/im/domain"

	"github.com/go-kratos/kratos/v2/log"
)

const (
	relayMaxAttempts = 50
	relayBackoffBase = time.Second
)

// Relay drains the durable search-index outbox into Meilisearch. It is a
// derived-index worker: failures never block message delivery or agent runs.
type Relay struct {
	Repo    app.SearchIndexRepository
	Index   *Client
	Log     *log.Helper
	Stats   func(pending, failed int64, oldestAge time.Duration)
	OnError func()
	ready   bool
}

func (r *Relay) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := r.Relay(ctx, 32); err != nil && r.Log != nil {
				r.Log.Warnf("search index relay: %v", err)
			}
			if r.Stats != nil {
				if pending, failed, age, err := r.Repo.SearchIndexStats(ctx); err == nil {
					r.Stats(pending, failed, age)
				}
			}
		}
	}
}

func (r *Relay) Relay(ctx context.Context, limit int) (int, error) {
	if r.Repo == nil || r.Index == nil || !r.Index.Enabled() {
		return 0, nil
	}
	if !r.ready {
		if err := r.Index.EnsureIndex(ctx); err != nil {
			return 0, err
		}
		r.ready = true
	}
	if limit <= 0 {
		limit = 32
	}
	processed := 0
	for i := 0; i < limit; i++ {
		task, err := r.Repo.ClaimSearchIndexTask(ctx)
		if errors.Is(err, domain.ErrNotFound) {
			return processed, nil
		}
		if err != nil {
			return processed, err
		}
		if task.Attempts > relayMaxAttempts {
			if err := r.Repo.AbandonSearchIndexTask(ctx, task.ID); err != nil {
				return processed, fmt.Errorf("abandon search task %s: %w", task.ID, err)
			}
			if r.OnError != nil {
				r.OnError()
			}
			continue
		}
		if err := r.apply(ctx, task); err != nil {
			if r.OnError != nil {
				r.OnError()
			}
			if ferr := r.Repo.FailSearchIndexTask(ctx, task.ID, err, relayBackoff(task.Attempts)); ferr != nil {
				return processed, fmt.Errorf("index task %s: %w (release: %v)", task.ID, err, ferr)
			}
			return processed, err
		}
		if err := r.Repo.CompleteSearchIndexTask(ctx, task.ID); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func (r *Relay) apply(ctx context.Context, task *domain.SearchIndexTask) error {
	switch task.Op {
	case domain.SearchIndexDelete:
		return r.Index.DeleteDocument(ctx, task.DocID)
	case domain.SearchIndexUpsert:
		var doc Document
		if err := json.Unmarshal(task.Payload, &doc); err != nil {
			return fmt.Errorf("decode search document: %w", err)
		}
		return r.Index.IndexDocuments(ctx, []Document{doc})
	default:
		return fmt.Errorf("unknown search task op %q", task.Op)
	}
}

func relayBackoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	delay := relayBackoffBase * time.Duration(1<<min(attempts-1, 8))
	if delay > 5*time.Minute {
		return 5 * time.Minute
	}
	return delay
}
