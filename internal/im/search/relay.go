package search

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"nonoka-im/internal/im/app"
	"nonoka-im/internal/im/domain"

	"github.com/go-kratos/kratos/v2/log"
)

const (
	relayMaxAttempts = 50
	relayBackoffBase = time.Second
	defaultBatchSize = 1000
)

// Relay drains the durable search-index outbox into Meilisearch. It is a
// derived-index worker: failures never block message delivery or agent runs.
type Relay struct {
	Repo      app.SearchIndexRepository
	Index     *Client
	Log       *log.Helper
	Stats     func(pending, failed int64, oldestAge time.Duration)
	OnError   func()
	BatchSize int
	ready     bool
	lastPrune time.Time
}

func (r *Relay) batchSize() int {
	if r.BatchSize > 0 {
		return r.BatchSize
	}
	return defaultBatchSize
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
			batch := r.batchSize()
			// Drain several batches per tick so a burst does not take
			// minutes to clear at one fixed batch per second.
			for i := 0; i < 4; i++ {
				n, err := r.Relay(ctx, batch)
				if err != nil && r.Log != nil {
					r.Log.Warnf("search index relay: %v", err)
				}
				if err != nil || n < batch {
					break
				}
			}
			if r.Stats != nil {
				if pending, failed, age, err := r.Repo.SearchIndexStats(ctx); err == nil {
					r.Stats(pending, failed, age)
				}
			}
			if r.lastPrune.IsZero() || time.Since(r.lastPrune) > 10*time.Minute {
				r.lastPrune = time.Now()
				if err := r.Repo.PrunePublishedSearchTasks(ctx); err != nil && r.Log != nil {
					r.Log.Warnf("prune search index tasks: %v", err)
				}
			}
		}
	}
}

// Relay claims up to limit tasks, applies all upserts in one Meilisearch
// request, then marks the tasks complete. Deletes stay one request each because
// the Meilisearch delete API is per-document.
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
		limit = r.batchSize()
	}
	tasks, err := r.Repo.ClaimSearchIndexTasks(ctx, limit)
	if err != nil {
		return 0, err
	}
	if len(tasks) == 0 {
		return 0, nil
	}

	var docs []Document
	var upserts []*domain.SearchIndexTask
	var deletes []*domain.SearchIndexTask
	var completed []string
	for _, task := range tasks {
		if task.Attempts > relayMaxAttempts {
			if err := r.Repo.AbandonSearchIndexTask(ctx, task.ID); err != nil {
				return len(completed), fmt.Errorf("abandon search task %s: %w", task.ID, err)
			}
			if r.OnError != nil {
				r.OnError()
			}
			continue
		}
		switch task.Op {
		case domain.SearchIndexDelete:
			deletes = append(deletes, task)
		case domain.SearchIndexUpsert:
			var doc Document
			if err := json.Unmarshal(task.Payload, &doc); err != nil {
				if r.OnError != nil {
					r.OnError()
				}
				_ = r.Repo.FailSearchIndexTask(ctx, task.ID, fmt.Errorf("decode search document: %w", err), relayBackoff(task.Attempts))
				continue
			}
			docs = append(docs, doc)
			upserts = append(upserts, task)
		default:
			if r.OnError != nil {
				r.OnError()
			}
			_ = r.Repo.FailSearchIndexTask(ctx, task.ID, fmt.Errorf("unknown search task op %q", task.Op), relayBackoff(task.Attempts))
		}
	}

	if len(docs) > 0 {
		if err := r.Index.IndexDocuments(ctx, docs); err != nil {
			if r.OnError != nil {
				r.OnError()
			}
			for _, task := range upserts {
				if ferr := r.Repo.FailSearchIndexTask(ctx, task.ID, err, relayBackoff(task.Attempts)); ferr != nil {
					return len(completed), fmt.Errorf("index task %s: %w (release: %v)", task.ID, err, ferr)
				}
			}
			return len(completed), err
		}
		for _, task := range upserts {
			completed = append(completed, task.ID)
		}
	}

	for _, task := range deletes {
		if err := r.Index.DeleteDocument(ctx, task.DocID); err != nil {
			if r.OnError != nil {
				r.OnError()
			}
			if ferr := r.Repo.FailSearchIndexTask(ctx, task.ID, err, relayBackoff(task.Attempts)); ferr != nil {
				return len(completed), fmt.Errorf("delete task %s: %w (release: %v)", task.ID, err, ferr)
			}
			return len(completed), err
		}
		completed = append(completed, task.ID)
	}

	for _, id := range completed {
		if err := r.Repo.CompleteSearchIndexTask(ctx, id); err != nil {
			return len(completed), err
		}
	}
	return len(completed), nil
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
