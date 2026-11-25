package mongostore

import (
	"context"
	"errors"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"nonoka-im/internal/im/domain"
	"strings"
	"time"
)

// SaveP2P persists a P2P message using write fan-out. When an agent outbox
// row or search indexing is enabled, the message, event and search task are
// committed in one MongoDB transaction.
func (s *Store) SaveP2P(ctx context.Context, msg *domain.Message, outbox *domain.OutboxEvent) ([]int64, bool, error) {
	if outbox == nil && !s.searchEnabled {
		return s.saveP2P(ctx, msg)
	}
	sess, err := s.db.Client().StartSession()
	if err != nil {
		return nil, false, fmt.Errorf("start mongo transaction session: %w", err)
	}
	defer sess.EndSession(ctx)
	var recipients []int64
	var duplicate bool
	_, err = sess.WithTransaction(ctx, func(txCtx context.Context) (any, error) {
		recipients, duplicate, err = s.saveP2P(txCtx, msg)
		if err != nil {
			return nil, err
		}
		if duplicate {
			return nil, errTransactionalDuplicate
		}
		if err := s.enqueueOutbox(txCtx, outbox); err != nil {
			return nil, err
		}
		if s.searchEnabled {
			if err := s.enqueueSearchTask(txCtx, msg); err != nil {
				return nil, err
			}
		}
		return nil, nil
	})
	if errors.Is(err, errTransactionalDuplicate) || errors.Is(err, errAgentEventDuplicate) {
		return recipients, true, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("persist p2p message and side indexes: %w", err)
	}
	return recipients, duplicate, nil
}

// SaveGroup persists a group message using read fan-out. When an agent outbox
// row or search indexing is enabled, the message, event and search task are
// committed in one MongoDB transaction.
func (s *Store) SaveGroup(ctx context.Context, msg *domain.Message, outbox *domain.OutboxEvent) (bool, error) {
	if outbox == nil && !s.searchEnabled {
		return s.saveGroup(ctx, msg)
	}
	sess, err := s.db.Client().StartSession()
	if err != nil {
		return false, fmt.Errorf("start mongo transaction session: %w", err)
	}
	defer sess.EndSession(ctx)
	var duplicate bool
	_, err = sess.WithTransaction(ctx, func(txCtx context.Context) (any, error) {
		duplicate, err = s.saveGroup(txCtx, msg)
		if err != nil {
			return nil, err
		}
		if duplicate {
			return nil, errTransactionalDuplicate
		}
		if err := s.enqueueOutbox(txCtx, outbox); err != nil {
			return nil, err
		}
		if s.searchEnabled {
			if err := s.enqueueSearchTask(txCtx, msg); err != nil {
				return nil, err
			}
		}
		return nil, nil
	})
	if errors.Is(err, errTransactionalDuplicate) || errors.Is(err, errAgentEventDuplicate) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("persist group message and side indexes: %w", err)
	}
	return duplicate, nil
}

func (s *Store) enqueueOutbox(ctx context.Context, outbox *domain.OutboxEvent) error {
	if outbox == nil {
		return nil
	}
	if strings.TrimSpace(outbox.EventID) == "" {
		return fmt.Errorf("agent event id is required")
	}
	coll := s.collection(CollectionAgentOutbox)
	var existing outboxRow
	err := coll.FindOne(ctx, bson.M{"event_id": outbox.EventID}).Decode(&existing)
	switch {
	case err == nil:
		if existing.MsgID != outbox.MsgID {
			s.log.Warnf("agent outbox event %s belongs to message %d, got %d; treating as duplicate", outbox.EventID, existing.MsgID, outbox.MsgID)
		}
		s.incAgentOutboxDuplicate()
		return errAgentEventDuplicate
	case errors.Is(err, mongo.ErrNoDocuments):
	default:
		return err
	}
	now := time.Now()
	_, err = coll.InsertOne(ctx, outboxRow{
		EventID: outbox.EventID, MsgID: outbox.MsgID, Topic: outbox.Topic, TopicSeq: outbox.TopicSeq,
		Payload: outbox.Payload, Status: domain.OutboxPending, NextAttemptAt: now, CreatedAt: now,
	})
	if mongo.IsDuplicateKeyError(err) {
		s.incAgentOutboxDuplicate()
		return errAgentEventDuplicate
	}
	return err
}

func (s *Store) incAgentOutboxDuplicate() {
	if s.metrics != nil {
		s.metrics.IncAgentOutboxDuplicate()
	}
}

// ClaimOutbox claims the oldest due row that has no earlier unpublished
// sibling in the same topic. A short lease makes rows recoverable after a
// relay crash.
func (s *Store) ClaimOutbox(ctx context.Context) (*domain.OutboxEvent, error) {
	now := time.Now()
	due := bson.M{"$or": []bson.M{
		{"status": domain.OutboxPending, "next_attempt_at": bson.M{"$lte": now}},
		{"status": domain.OutboxProcessing, "locked_until": bson.M{"$lt": now}},
	}}
	update := bson.M{"$set": bson.M{"status": domain.OutboxProcessing, "locked_until": now.Add(time.Minute)}, "$inc": bson.M{"attempts": 1}}
	coll := s.collection(CollectionAgentOutbox)
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: due}},
		bson.D{{Key: "$lookup", Value: bson.M{
			"from": CollectionAgentOutbox,
			"let":  bson.M{"topic": "$topic", "seq": "$topic_seq"},
			"pipeline": []bson.M{
				{"$match": bson.M{"$expr": bson.M{"$and": []bson.M{
					{"$eq": []any{"$topic", "$$topic"}},
					{"$gt": []any{"$topic_seq", 0}},
					{"$lt": []any{"$topic_seq", "$$seq"}},
					{"$not": bson.M{"$in": []any{"$status", []string{domain.OutboxPublished, domain.OutboxFailed}}}},
				}}}},
				{"$limit": 1},
			},
			"as": "older_unpublished",
		}}},
		bson.D{{Key: "$match", Value: bson.M{"older_unpublished": bson.M{"$size": 0}}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "created_at", Value: 1}}}},
		bson.D{{Key: "$limit", Value: 1}},
	}
	for attempt := 0; attempt < 3; attempt++ {
		cursor, err := coll.Aggregate(ctx, pipeline)
		if err != nil {
			return nil, err
		}
		var candidates []outboxRow
		err = cursor.All(ctx, &candidates)
		_ = cursor.Close(ctx)
		if err != nil {
			return nil, err
		}
		if len(candidates) == 0 {
			return nil, domain.ErrNotFound
		}
		filter := bson.M{"_id": candidates[0].ID, "$or": due["$or"]}
		var row outboxRow
		err = coll.FindOneAndUpdate(ctx, filter, update, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&row)
		if err == nil {
			out := toDomainOutbox(&row)
			return &out, nil
		}
		if err != mongo.ErrNoDocuments {
			return nil, err
		}
	}
	return nil, domain.ErrNotFound
}

func (s *Store) MarkOutboxPublished(ctx context.Context, id string) error {
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	now := time.Now()
	_, err = s.collection(CollectionAgentOutbox).UpdateOne(ctx, bson.M{"_id": objectID, "status": domain.OutboxProcessing}, bson.M{"$set": bson.M{"status": domain.OutboxPublished, "published_at": now}, "$unset": bson.M{"locked_until": "", "last_error": ""}})
	return err
}

func (s *Store) FailOutbox(ctx context.Context, id string, eventErr error, backoff time.Duration) error {
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	_, err = s.collection(CollectionAgentOutbox).UpdateOne(ctx, bson.M{"_id": objectID, "status": domain.OutboxProcessing}, bson.M{"$set": bson.M{"status": domain.OutboxPending, "next_attempt_at": time.Now().Add(backoff), "last_error": eventErr.Error()}, "$unset": bson.M{"locked_until": ""}})
	return err
}

func (s *Store) AbandonOutbox(ctx context.Context, id string) error {
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	_, err = s.collection(CollectionAgentOutbox).UpdateOne(ctx, bson.M{"_id": objectID, "status": domain.OutboxProcessing}, bson.M{"$set": bson.M{"status": domain.OutboxFailed, "last_error": "attempts exceeded"}, "$unset": bson.M{"locked_until": ""}})
	return err
}

func (s *Store) PrunePublishedOutbox(ctx context.Context) error {
	cutoff := time.Now().Add(-agentEventPublishedRetention)
	coll := s.collection(CollectionAgentOutbox)
	cursor, err := coll.Find(ctx, bson.M{"status": domain.OutboxPublished, "published_at": bson.M{"$lt": cutoff}}, options.Find().SetProjection(bson.M{"_id": 1}).SetLimit(agentEventPruneLimit))
	if err != nil {
		return err
	}
	var rows []outboxRow
	err = cursor.All(ctx, &rows)
	_ = cursor.Close(ctx)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	ids := make([]any, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].ID)
	}
	_, err = coll.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": ids}})
	return err
}

func (s *Store) OutboxStats(ctx context.Context) (pending, failed int64, oldestAge time.Duration, err error) {
	coll := s.collection(CollectionAgentOutbox)
	pending, err = coll.CountDocuments(ctx, bson.M{"status": bson.M{"$in": []string{domain.OutboxPending, domain.OutboxProcessing}}})
	if err != nil {
		return 0, 0, 0, err
	}
	failed, err = coll.CountDocuments(ctx, bson.M{"status": domain.OutboxFailed})
	if err != nil {
		return 0, 0, 0, err
	}
	var oldest outboxRow
	err = coll.FindOne(ctx, bson.M{"status": bson.M{"$in": []string{domain.OutboxPending, domain.OutboxProcessing}}}, options.FindOne().SetSort(bson.D{{Key: "created_at", Value: 1}})).Decode(&oldest)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return pending, failed, 0, nil
	}
	if err != nil {
		return 0, 0, 0, err
	}
	return pending, failed, time.Since(oldest.CreatedAt), nil
}

func (s *Store) ListFailedOutbox(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	cursor, err := s.collection(CollectionAgentOutbox).Find(ctx, bson.M{"status": domain.OutboxFailed}, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	var rows []outboxRow
	if err := cursor.All(ctx, &rows); err != nil {
		_ = cursor.Close(ctx)
		return nil, err
	}
	_ = cursor.Close(ctx)
	out := make([]domain.OutboxEvent, 0, len(rows))
	for i := range rows {
		out = append(out, toDomainOutbox(&rows[i]))
	}
	return out, nil
}

func (s *Store) ReplayOutbox(ctx context.Context, id string) error {
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	result := s.collection(CollectionAgentOutbox).FindOneAndUpdate(ctx,
		bson.M{"_id": objectID, "status": domain.OutboxFailed},
		bson.M{"$set": bson.M{"status": domain.OutboxPending, "attempts": 0, "next_attempt_at": time.Now(), "last_error": ""}, "$unset": bson.M{"locked_until": ""}},
		options.FindOneAndUpdate().SetReturnDocument(options.After))
	if err := result.Err(); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return fmt.Errorf("failed agent outbox row %s not found", id)
		}
		return err
	}
	return nil
}

// ReplayOutboxMany atomically moves the selected failed rows back to pending.
// Rows that are no longer failed are ignored; the returned count is the number
// actually requeued.
func (s *Store) ReplayOutboxMany(ctx context.Context, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	objectIDs := make([]bson.ObjectID, 0, len(ids))
	for _, id := range ids {
		objectID, err := bson.ObjectIDFromHex(id)
		if err != nil {
			return 0, fmt.Errorf("invalid outbox id %q", id)
		}
		objectIDs = append(objectIDs, objectID)
	}
	result, err := s.collection(CollectionAgentOutbox).UpdateMany(ctx,
		bson.M{"_id": bson.M{"$in": objectIDs}, "status": domain.OutboxFailed},
		bson.M{"$set": bson.M{"status": domain.OutboxPending, "attempts": 0, "next_attempt_at": time.Now(), "last_error": ""}, "$unset": bson.M{"locked_until": ""}},
	)
	if err != nil {
		return 0, err
	}
	return int(result.ModifiedCount), nil
}

func toDomainOutbox(row *outboxRow) domain.OutboxEvent {
	return domain.OutboxEvent{
		ID: row.ID.Hex(), EventID: row.EventID, MsgID: row.MsgID, Topic: row.Topic,
		TopicSeq: row.TopicSeq, Payload: row.Payload, Status: row.Status, Attempts: row.Attempts,
		NextAttemptAt: row.NextAttemptAt, LockedUntil: row.LockedUntil, LastError: row.LastError,
		CreatedAt: row.CreatedAt, PublishedAt: row.PublishedAt,
	}
}
