package mongostore

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"nonoka-im/internal/im/domain"
	"nonoka-im/internal/im/search"
)

// searchIndexRow is the durable, transactional outbox for Meilisearch. It is
// deliberately separate from the agent trigger outbox: search is a derived
// read model and must not block or pollute agent delivery.
type searchIndexRow struct {
	ID            bson.ObjectID `bson:"_id,omitempty"`
	Op            string        `bson:"op"`
	DocID         string        `bson:"doc_id"`
	Payload       []byte        `bson:"payload,omitempty"`
	Status        string        `bson:"status"`
	Attempts      int           `bson:"attempts"`
	NextAttemptAt time.Time     `bson:"next_attempt_at"`
	LockedUntil   time.Time     `bson:"locked_until,omitempty"`
	LastError     string        `bson:"last_error,omitempty"`
	CreatedAt     time.Time     `bson:"created_at"`
	PublishedAt   time.Time     `bson:"published_at,omitempty"`
}

// enqueueSearchTask must be called inside the same transaction as the message
// insert. It is a no-op when search is disabled.
func (s *Store) enqueueSearchTask(ctx context.Context, msg *domain.Message) error {
	if !s.searchEnabled || msg == nil {
		return nil
	}
	doc, ok := search.BuildDocument(msg, search.ContentText(msg.GetMsgType(), msg.GetContent()))
	if !ok {
		return nil
	}
	payload, err := search.MarshalDocument(doc)
	if err != nil {
		return fmt.Errorf("marshal search document: %w", err)
	}
	return s.insertSearchTask(ctx, &searchIndexRow{
		Op: domain.SearchIndexUpsert, DocID: doc.ID, Payload: payload,
		Status: domain.SearchIndexPending, NextAttemptAt: time.Now(), CreatedAt: time.Now(),
	})
}

func (s *Store) enqueueSearchDelete(ctx context.Context, msgID int64, topic string) error {
	if !s.searchEnabled {
		return nil
	}
	docID := fmt.Sprintf("%d", msgID)
	return s.insertSearchTask(ctx, &searchIndexRow{
		Op: domain.SearchIndexDelete, DocID: docID, Status: domain.SearchIndexPending,
		NextAttemptAt: time.Now(), CreatedAt: time.Now(),
	})
}

func (s *Store) insertSearchTask(ctx context.Context, row *searchIndexRow) error {
	_, err := s.collection(CollectionSearchOutbox).InsertOne(ctx, row, options.InsertOne())
	if err != nil && !mongo.IsDuplicateKeyError(err) {
		return fmt.Errorf("enqueue search index task: %w", err)
	}
	return nil
}

// ClaimSearchIndexTask leases the oldest due task. Replaying a task is safe:
// upserts use msg_id as the primary key and deletes are idempotent.
func (s *Store) ClaimSearchIndexTask(ctx context.Context) (*domain.SearchIndexTask, error) {
	now := time.Now()
	due := bson.M{"$or": []bson.M{
		{"status": domain.SearchIndexPending, "next_attempt_at": bson.M{"$lte": now}},
		{"status": domain.SearchIndexProcessing, "locked_until": bson.M{"$lt": now}},
	}}
	update := bson.M{
		"$set": bson.M{"status": domain.SearchIndexProcessing, "locked_until": now.Add(time.Minute)},
		"$inc": bson.M{"attempts": 1},
	}
	var row searchIndexRow
	err := s.collection(CollectionSearchOutbox).FindOneAndUpdate(ctx, due, update,
		options.FindOneAndUpdate().
			SetSort(bson.D{{Key: "next_attempt_at", Value: 1}}).
			SetReturnDocument(options.After),
	).Decode(&row)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	task := toDomainSearchTask(&row)
	return &task, nil
}

// ClaimSearchIndexTasks leases up to limit due tasks. Each claim remains an
// atomic FindOneAndUpdate, but claims run with bounded parallelism: a single
// relay batch of 1000 otherwise costs 1000 sequential MongoDB round-trips.
func (s *Store) ClaimSearchIndexTasks(ctx context.Context, limit int) ([]*domain.SearchIndexTask, error) {
	if limit <= 0 {
		limit = 100
	}
	const claimWorkers = 8
	workers := min(limit, claimWorkers)
	var (
		mu        sync.Mutex
		tasks     = make([]*domain.SearchIndexTask, 0, limit)
		remaining = int64(limit)
		firstErr  error
	)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if atomic.AddInt64(&remaining, -1) < 0 {
					return
				}
				task, err := s.ClaimSearchIndexTask(ctx)
				if errors.Is(err, domain.ErrNotFound) {
					return
				}
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					return
				}
				mu.Lock()
				tasks = append(tasks, task)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if firstErr != nil {
		return tasks, firstErr
	}
	return tasks, nil
}

func (s *Store) CompleteSearchIndexTask(ctx context.Context, id string) error {
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	_, err = s.collection(CollectionSearchOutbox).UpdateOne(ctx,
		bson.M{"_id": objectID, "status": domain.SearchIndexProcessing},
		bson.M{"$set": bson.M{"status": domain.SearchIndexPublished, "published_at": time.Now()}, "$unset": bson.M{"locked_until": ""}},
	)
	return err
}

func (s *Store) FailSearchIndexTask(ctx context.Context, id string, taskErr error, backoff time.Duration) error {
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	message := ""
	if taskErr != nil {
		message = taskErr.Error()
	}
	_, err = s.collection(CollectionSearchOutbox).UpdateOne(ctx,
		bson.M{"_id": objectID, "status": domain.SearchIndexProcessing},
		bson.M{"$set": bson.M{
			"status": domain.SearchIndexPending, "next_attempt_at": time.Now().Add(backoff),
			"last_error": message,
		}, "$unset": bson.M{"locked_until": ""}},
	)
	return err
}

func (s *Store) AbandonSearchIndexTask(ctx context.Context, id string) error {
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	_, err = s.collection(CollectionSearchOutbox).UpdateOne(ctx,
		bson.M{"_id": objectID, "status": domain.SearchIndexProcessing},
		bson.M{"$set": bson.M{"status": domain.SearchIndexFailed, "last_error": "attempts exceeded"}, "$unset": bson.M{"locked_until": ""}},
	)
	return err
}

func (s *Store) SearchIndexStats(ctx context.Context) (pending, failed int64, oldestAge time.Duration, err error) {
	coll := s.collection(CollectionSearchOutbox)
	pending, err = coll.CountDocuments(ctx, bson.M{"status": bson.M{"$in": []string{domain.SearchIndexPending, domain.SearchIndexProcessing}}})
	if err != nil {
		return 0, 0, 0, err
	}
	failed, err = coll.CountDocuments(ctx, bson.M{"status": domain.SearchIndexFailed})
	if err != nil {
		return 0, 0, 0, err
	}
	var oldest searchIndexRow
	err = coll.FindOne(ctx, bson.M{"status": bson.M{"$in": []string{domain.SearchIndexPending, domain.SearchIndexProcessing}}},
		options.FindOne().SetSort(bson.D{{Key: "created_at", Value: 1}})).Decode(&oldest)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return pending, failed, 0, nil
	}
	if err != nil {
		return 0, 0, 0, err
	}
	return pending, failed, time.Since(oldest.CreatedAt), nil
}

func (s *Store) PrunePublishedSearchTasks(ctx context.Context) error {
	cutoff := time.Now().Add(-24 * time.Hour)
	_, err := s.collection(CollectionSearchOutbox).DeleteMany(ctx, bson.M{
		"status": domain.SearchIndexPublished, "published_at": bson.M{"$lt": cutoff},
	})
	return err
}

func toDomainSearchTask(row *searchIndexRow) domain.SearchIndexTask {
	return domain.SearchIndexTask{
		ID: row.ID.Hex(), Op: row.Op, DocID: row.DocID, Payload: row.Payload,
		Status: row.Status, Attempts: row.Attempts, NextAttemptAt: row.NextAttemptAt,
		LockedUntil: row.LockedUntil, LastError: row.LastError,
		CreatedAt: row.CreatedAt, PublishedAt: row.PublishedAt,
	}
}
