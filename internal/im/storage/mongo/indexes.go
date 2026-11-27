package mongostore

import (
	"context"
	"errors"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"strings"
)

// EnsureIndexes creates necessary indexes. It skips indexes that already exist
// and does NOT drop existing indexes, making it safe for production use.
func (s *Store) EnsureIndexes(ctx context.Context) error {
	// Index for group messages: topic + topic_seq
	if err := s.createIndex(ctx, CollectionMessages, "topic_seq_unique", mongo.IndexModel{
		Keys:    bson.D{{Key: "topic", Value: 1}, {Key: "topic_seq", Value: 1}},
		Options: options.Index().SetUnique(true),
	}); err != nil {
		return fmt.Errorf("create messages index: %w", err)
	}

	// Unique index on client_msg_id + sender_id for deduplication (messages collection)
	if err := s.createIndex(ctx, CollectionMessages, "client_msg_id_dedup", mongo.IndexModel{
		Keys: bson.D{{Key: "sender_id", Value: 1}, {Key: "client_msg_id", Value: 1}},
		Options: options.Index().SetUnique(true).SetPartialFilterExpression(
			bson.M{"client_msg_id": bson.M{"$exists": true}},
		),
	}); err != nil {
		return fmt.Errorf("create messages dedup index: %w", err)
	}

	if err := s.createIndex(ctx, CollectionMessages, "messages_topic_time", mongo.IndexModel{
		Keys: bson.D{{Key: "topic", Value: 1}, {Key: "timestamp", Value: -1}},
	}); err != nil {
		return fmt.Errorf("create messages topic/time index: %w", err)
	}

	// Index for inbox: user_id + topic + topic_seq
	if err := s.createIndex(ctx, CollectionInboxes, "inbox_unique", mongo.IndexModel{
		Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "topic", Value: 1}, {Key: "topic_seq", Value: 1}},
		Options: options.Index().SetUnique(true),
	}); err != nil {
		return fmt.Errorf("create inbox index: %w", err)
	}

	if err := s.createIndex(ctx, CollectionInboxes, "inbox_topic_time", mongo.IndexModel{
		Keys: bson.D{{Key: "topic", Value: 1}, {Key: "timestamp", Value: -1}},
	}); err != nil {
		return fmt.Errorf("create inbox topic/time index: %w", err)
	}

	// Recovery reads the highest persisted seq for a topic without a user
	// prefix. The inbox unique index starts with user_id, so it cannot serve
	// this topic-only lookup.
	if err := s.createIndex(ctx, CollectionInboxes, "inbox_topic_seq", mongo.IndexModel{
		Keys: bson.D{{Key: "topic", Value: 1}, {Key: "topic_seq", Value: -1}},
	}); err != nil {
		return fmt.Errorf("create inbox topic/seq index: %w", err)
	}

	// Unique index on client_msg_id + user_id for inbox deduplication.
	// Uses user_id (not sender_id) because both sender and receiver have
	// separate inbox entries for the same message, and they must not
	// conflict with each other.
	if err := s.createIndex(ctx, CollectionInboxes, "inbox_client_msg_id_dedup", mongo.IndexModel{
		Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "client_msg_id", Value: 1}},
		Options: options.Index().SetUnique(true).SetPartialFilterExpression(
			bson.M{"client_msg_id": bson.M{"$exists": true}},
		),
	}); err != nil {
		return fmt.Errorf("create inbox dedup index: %w", err)
	}

	// Index for topic_seqs
	if err := s.createIndex(ctx, CollectionTopicSeqs, "topic_unique", mongo.IndexModel{
		Keys:    bson.D{{Key: "topic", Value: 1}},
		Options: options.Index().SetUnique(true),
	}); err != nil {
		return fmt.Errorf("create topic_seq index: %w", err)
	}

	// Index for mention_inboxes: user_id + topic + topic_seq
	if err := s.createIndex(ctx, CollectionMentionInboxes, "mention_inbox_unique", mongo.IndexModel{
		Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "topic", Value: 1}, {Key: "topic_seq", Value: 1}},
		Options: options.Index().SetUnique(true),
	}); err != nil {
		return fmt.Errorf("create mention_inbox index: %w", err)
	}

	// Unique index on user_id + client_msg_id for mention inbox deduplication.
	if err := s.createIndex(ctx, CollectionMentionInboxes, "mention_client_msg_id_dedup", mongo.IndexModel{
		Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "client_msg_id", Value: 1}},
		Options: options.Index().SetUnique(true).SetPartialFilterExpression(
			bson.M{"client_msg_id": bson.M{"$exists": true}},
		),
	}); err != nil {
		return fmt.Errorf("create mention_inbox dedup index: %w", err)
	}

	// Index for delivery_status: user_id + topic + topic_seq (for fast ACK lookups)
	if err := s.createIndex(ctx, CollectionDeliveryStatus, "delivery_status_unique", mongo.IndexModel{
		Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "topic", Value: 1}, {Key: "topic_seq", Value: 1}},
		Options: options.Index().SetUnique(true),
	}); err != nil {
		return fmt.Errorf("create delivery_status index: %w", err)
	}

	if err := s.createIndex(ctx, CollectionAgentOutbox, "agent_outbox_event_unique", mongo.IndexModel{
		Keys:    bson.D{{Key: "event_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	}); err != nil {
		return fmt.Errorf("create agent outbox event index: %w", err)
	}
	if err := s.createIndex(ctx, CollectionAgentOutbox, "agent_outbox_due", mongo.IndexModel{
		Keys: bson.D{{Key: "status", Value: 1}, {Key: "next_attempt_at", Value: 1}, {Key: "locked_until", Value: 1}},
	}); err != nil {
		return fmt.Errorf("create agent outbox due index: %w", err)
	}
	if err := s.createIndex(ctx, CollectionAgentOutbox, "agent_outbox_topic_order", mongo.IndexModel{Keys: bson.D{{Key: "topic", Value: 1}, {Key: "topic_seq", Value: 1}, {Key: "status", Value: 1}}}); err != nil {
		return fmt.Errorf("create agent outbox topic index: %w", err)
	}

	if err := s.createIndex(ctx, CollectionSearchOutbox, "search_outbox_due", mongo.IndexModel{
		Keys: bson.D{{Key: "status", Value: 1}, {Key: "next_attempt_at", Value: 1}, {Key: "locked_until", Value: 1}},
	}); err != nil {
		return fmt.Errorf("create search outbox due index: %w", err)
	}

	return nil
}

// createIndex creates an index if it does not already exist. It ignores
// "already exists" errors to make startup idempotent and safe in production.
// If an index with the same name but different key spec exists (e.g., after
// a schema migration), it drops only that conflicting index and recreates it.
func (s *Store) createIndex(ctx context.Context, collection, indexName string, model mongo.IndexModel) error {
	// Set the index name so we can detect if it already exists.
	if model.Options == nil {
		model.Options = options.Index()
	}
	model.Options.SetName(indexName)

	_, err := s.collection(collection).Indexes().CreateOne(ctx, model)
	if err == nil {
		return nil
	}
	// Ignore "already exists" errors for idempotent index creation.
	if isIndexAlreadyExistsError(err) {
		s.log.Debugf("index %s on %s already exists, skipping", indexName, collection)
		return nil
	}
	// Handle key spec conflict: same name but different keys. Drop only
	// the conflicting index (never drop all) and recreate.
	if isIndexKeySpecsConflictError(err) {
		s.log.Warnf("index %s on %s has key spec conflict, dropping and recreating", indexName, collection)
		dropErr := s.collection(collection).Indexes().DropOne(ctx, indexName)
		if dropErr != nil {
			return fmt.Errorf("drop conflicting index %s on %s: %w", indexName, collection, dropErr)
		}
		_, err = s.collection(collection).Indexes().CreateOne(ctx, model)
		if err != nil {
			return fmt.Errorf("recreate index %s on %s after drop: %w", indexName, collection, err)
		}
		return nil
	}
	return err
}

// isIndexAlreadyExistsError checks if the error indicates the index already exists.
func isIndexAlreadyExistsError(err error) bool {
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		// 68 = IndexAlreadyExists. A duplicate-key error while building a
		// unique index (11000) is NOT "already exists": the index was not
		// created and callers must see the failure.
		return cmdErr.Code == 68
	}
	return strings.Contains(err.Error(), "IndexAlreadyExists")
}

// isIndexKeySpecsConflictError checks if the error indicates an index name
// conflict where the existing index has a different key specification.
func isIndexKeySpecsConflictError(err error) bool {
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.Code == 85 || cmdErr.Code == 86
	}
	return strings.Contains(err.Error(), "IndexKeySpecsConflict")
}
