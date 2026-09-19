package msgworker

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
	pb "nonoka-im/api/im/v1"
	"nonoka-im/internal/metrics"
)

var (
	ErrRecallNotFound = errors.New("message not found or already recalled")
	ErrRecallExpired  = errors.New("message recall window expired")
)

const (
	// Collection names
	CollectionMessages       = "messages"        // group messages (read扩散)
	CollectionInboxes        = "inboxes"         // user inboxes (write扩散 for P2P)
	CollectionTopicSeqs      = "topic_seqs"      // topic max seq backup
	CollectionMentionInboxes = "mention_inboxes" // group @mention inbox (for large groups)
	CollectionDeliveryStatus = "delivery_status" // message delivery tracking per user

	// senderCacheKeyPrefix is the Redis key prefix for message sender cache.
	senderCacheKeyPrefix = "im:sender"
	// senderCacheTTL is the time-to-live for sender cache entries.
	senderCacheTTL = 24 * time.Hour
)

// StoredMessage represents a message stored in MongoDB (read扩散 for groups).
type StoredMessage struct {
	MsgID       int64     `bson:"msg_id"`
	Topic       string    `bson:"topic"`
	SenderID    int64     `bson:"sender_id"`
	MsgType     int32     `bson:"msg_type"`
	Content     []byte    `bson:"content"`
	Timestamp   int64     `bson:"timestamp"`
	TopicSeq    uint64    `bson:"topic_seq"`
	ClientMsgID string    `bson:"client_msg_id,omitempty"`
	Recalled    bool      `bson:"recalled,omitempty"`
	CreatedAt   time.Time `bson:"created_at"`
}

// InboxMessage represents a message in user's inbox (write扩散).
type InboxMessage struct {
	UserID      int64     `bson:"user_id"`
	MsgID       int64     `bson:"msg_id"`
	Topic       string    `bson:"topic"`
	SenderID    int64     `bson:"sender_id"`
	MsgType     int32     `bson:"msg_type"`
	Content     []byte    `bson:"content"`
	Timestamp   int64     `bson:"timestamp"`
	TopicSeq    uint64    `bson:"topic_seq"`
	ClientMsgID string    `bson:"client_msg_id,omitempty"`
	Recalled    bool      `bson:"recalled,omitempty"`
	Read        bool      `bson:"read"`
	DeliveredAt time.Time `bson:"delivered_at,omitempty"`
	CreatedAt   time.Time `bson:"created_at"`
}

// TopicSeqBackup stores the max seq for a topic as a fallback.
type TopicSeqBackup struct {
	Topic     string    `bson:"topic"`
	MaxSeq    uint64    `bson:"max_seq"`
	UpdatedAt time.Time `bson:"updated_at"`
}

// MentionMessage stores @mention notifications for users in large groups.
// This is a special write扩散 for @mentions in read扩散 groups.
type MentionMessage struct {
	UserID      int64     `bson:"user_id"`
	MsgID       int64     `bson:"msg_id"`
	Topic       string    `bson:"topic"`
	SenderID    int64     `bson:"sender_id"`
	MsgType     int32     `bson:"msg_type"`
	Content     []byte    `bson:"content"`
	Timestamp   int64     `bson:"timestamp"`
	TopicSeq    uint64    `bson:"topic_seq"`
	ClientMsgID string    `bson:"client_msg_id,omitempty"`
	Recalled    bool      `bson:"recalled,omitempty"`
	Read        bool      `bson:"read"`
	DeliveredAt time.Time `bson:"delivered_at,omitempty"`
	CreatedAt   time.Time `bson:"created_at"`
}

// DeliveryStatus tracks whether a message has been delivered to a specific user.
// Used for both write扩散 and read扩散 messages.
type DeliveryStatus struct {
	UserID      int64     `bson:"user_id"`
	MsgID       int64     `bson:"msg_id"`
	Topic       string    `bson:"topic"`
	TopicSeq    uint64    `bson:"topic_seq"`
	DeliveredAt time.Time `bson:"delivered_at"`
}

// MessageStorage handles MongoDB persistence for messages.
type MessageStorage struct {
	db           *mongo.Database
	redis        redis.UniversalClient
	metrics      *metrics.Metrics
	log          *log.Helper
	writeConcern *writeconcern.WriteConcern
}

// NewMessageStorage creates a new MessageStorage.
func NewMessageStorage(db *mongo.Database, logger log.Logger, m ...*metrics.Metrics) *MessageStorage {
	s := &MessageStorage{
		db:           db,
		log:          log.NewHelper(logger),
		writeConcern: writeconcern.W1(),
	}
	if len(m) > 0 {
		s.metrics = m[0]
	}
	return s
}

// SetWriteConcern configures the MongoDB write concern from a string label.
// Supported labels:
//   - "w1"            : w=1, j=false  (fast, default)
//   - "majority"      : w="majority", j=true
//   - "majority_jfalse": w="majority", j=false
func (s *MessageStorage) SetWriteConcern(label string) {
	switch strings.ToLower(label) {
	case "majority":
		s.writeConcern = writeconcern.Majority()
	case "majority_jfalse":
		// MongoDB driver does not expose a direct "majority without journal"
		// constructor, so we build it via a custom tag.
		s.writeConcern = writeconcern.Custom("majority")
	case "w1", "", "default":
		fallthrough
	default:
		s.writeConcern = writeconcern.W1()
	}
}

// collection returns a collection handle configured with the current write concern.
func (s *MessageStorage) collection(name string) *mongo.Collection {
	return s.db.Collection(name, options.Collection().SetWriteConcern(s.writeConcern))
}

// SetMetrics configures the metrics collector for storage operations.
func (s *MessageStorage) SetMetrics(m *metrics.Metrics) {
	s.metrics = m
}

// SetRedis configures an optional Redis client for caches (e.g., sender lookup).
func (s *MessageStorage) SetRedis(redis redis.UniversalClient) {
	s.redis = redis
}

// incMongoOp increments the MongoDB operation counter if metrics is configured.
func (s *MessageStorage) incMongoOp(collection, op string) {
	if s.metrics != nil {
		s.metrics.IncMongoOps(collection, op)
	}
}

// senderCacheKey returns the Redis key for caching a message's sender.
func senderCacheKey(topic string, topicSeq uint64) string {
	return fmt.Sprintf("%s:%s:%d", senderCacheKeyPrefix, topic, topicSeq)
}

// cacheMessageSender writes the sender ID to Redis for fast lookup.
func (s *MessageStorage) cacheMessageSender(ctx context.Context, topic string, topicSeq uint64, senderID int64) {
	if s.redis == nil {
		return
	}
	key := senderCacheKey(topic, topicSeq)
	if err := s.redis.Set(ctx, key, senderID, senderCacheTTL).Err(); err != nil {
		s.log.Warnf("cache message sender failed: topic=%s seq=%d err=%v", topic, topicSeq, err)
	}
}

// getCachedMessageSender reads the sender ID from Redis cache if available.
func (s *MessageStorage) getCachedMessageSender(ctx context.Context, topic string, topicSeq uint64) (int64, bool) {
	if s.redis == nil {
		return 0, false
	}
	key := senderCacheKey(topic, topicSeq)
	val, err := s.redis.Get(ctx, key).Result()
	if err != nil {
		return 0, false
	}
	senderID, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, false
	}
	return senderID, true
}

// EnsureIndexes creates necessary indexes. It skips indexes that already exist
// and does NOT drop existing indexes, making it safe for production use.
func (s *MessageStorage) EnsureIndexes(ctx context.Context) error {
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

	// Index for inbox: user_id + topic + topic_seq
	if err := s.createIndex(ctx, CollectionInboxes, "inbox_unique", mongo.IndexModel{
		Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "topic", Value: 1}, {Key: "topic_seq", Value: 1}},
		Options: options.Index().SetUnique(true),
	}); err != nil {
		return fmt.Errorf("create inbox index: %w", err)
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

	return nil
}

// createIndex creates an index if it does not already exist. It ignores
// "already exists" errors to make startup idempotent and safe in production.
// If an index with the same name but different key spec exists (e.g., after
// a schema migration), it drops only that conflicting index and recreates it.
func (s *MessageStorage) createIndex(ctx context.Context, collection, indexName string, model mongo.IndexModel) error {
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
	if err == nil {
		return false
	}
	errStr := err.Error()
	// MongoDB error messages for existing indexes vary by version.
	return strings.Contains(errStr, "already exists") ||
		strings.Contains(errStr, "IndexAlreadyExists") ||
		strings.Contains(errStr, "duplicate key")
}

// isIndexKeySpecsConflictError checks if the error indicates an index name
// conflict where the existing index has a different key specification.
func isIndexKeySpecsConflictError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "IndexKeySpecsConflict")
}

// TopicType determines the type of a topic.
type TopicType int

const (
	TopicTypeUnknown TopicType = iota
	TopicTypeP2P
	TopicTypeGroup
	TopicTypeSystem
)

// ParseTopicType parses a topic string to determine its type.
// Format: p2p_uid1_uid2, grp_groupid, sys_uid
func ParseTopicType(topic string) TopicType {
	switch {
	case strings.HasPrefix(topic, "p2p_"):
		return TopicTypeP2P
	case strings.HasPrefix(topic, "grp_"):
		return TopicTypeGroup
	case strings.HasPrefix(topic, "sys_"):
		return TopicTypeSystem
	default:
		return TopicTypeUnknown
	}
}

// ExtractUserIDsFromP2PTopic extracts the two user IDs from a P2P topic.
func ExtractUserIDsFromP2PTopic(topic string) (uid1, uid2 int64, err error) {
	parts := strings.Split(topic, "_")
	if len(parts) != 3 {
		return 0, 0, fmt.Errorf("invalid p2p topic format: %s", topic)
	}
	_, err = fmt.Sscanf(parts[1]+" "+parts[2], "%d %d", &uid1, &uid2)
	if err != nil {
		return 0, 0, fmt.Errorf("parse p2p topic user IDs: %w", err)
	}
	return uid1, uid2, nil
}

// NormalizeTopic canonicalizes a topic string.
// For P2P topics it orders the two user IDs ascending so that the same
// conversation always maps to a single topic regardless of which side initiates
// the message. Group and system topics are returned unchanged.
func NormalizeTopic(topic string) (string, error) {
	topicType := ParseTopicType(topic)
	switch topicType {
	case TopicTypeP2P:
		uid1, uid2, err := ExtractUserIDsFromP2PTopic(topic)
		if err != nil {
			return "", err
		}
		if uid1 > uid2 {
			uid1, uid2 = uid2, uid1
		}
		return fmt.Sprintf("p2p_%d_%d", uid1, uid2), nil
	case TopicTypeGroup, TopicTypeSystem:
		return topic, nil
	default:
		return "", fmt.Errorf("invalid topic format: %s", topic)
	}
}

// IsDuplicateError checks if a MongoDB error is a duplicate key error.
func IsDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	return mongo.IsDuplicateKeyError(err)
}

// SaveP2PMessage saves a P2P message using write扩散.
// Writes the message to the receiver's inbox.
// Returns isDuplicate=true if the message already exists (idempotent).
func (s *MessageStorage) SaveP2PMessage(ctx context.Context, msg *pb.UpstreamMessage, msgID int64, topicSeq uint64) ([]int64, bool, error) {
	s.incMongoOp(CollectionInboxes, "insert")
	uid1, uid2, err := ExtractUserIDsFromP2PTopic(msg.GetTopic())
	if err != nil {
		return nil, false, err
	}

	var receiverID int64
	if msg.GetSenderId() == uid1 {
		receiverID = uid2
	} else {
		receiverID = uid1
	}

	now := time.Now()
	inboxes := []*InboxMessage{
		{
			UserID:      receiverID,
			MsgID:       msgID,
			Topic:       msg.GetTopic(),
			SenderID:    msg.GetSenderId(),
			MsgType:     msg.GetMsgType(),
			Content:     msg.GetContent(),
			Timestamp:   msg.GetTimestamp(),
			TopicSeq:    topicSeq,
			ClientMsgID: msg.GetClientMsgId(),
			Read:        false,
			CreatedAt:   now,
		},
		{
			// Also write to sender's inbox so multi-device sender can see own messages.
			UserID:      msg.GetSenderId(),
			MsgID:       msgID,
			Topic:       msg.GetTopic(),
			SenderID:    msg.GetSenderId(),
			MsgType:     msg.GetMsgType(),
			Content:     msg.GetContent(),
			Timestamp:   msg.GetTimestamp(),
			TopicSeq:    topicSeq,
			ClientMsgID: msg.GetClientMsgId(),
			Read:        true, // sender's own copy is considered read
			CreatedAt:   now,
		},
	}

	docs := make([]interface{}, len(inboxes))
	for i, inbox := range inboxes {
		docs[i] = inbox
	}

	_, err = s.collection(CollectionInboxes).InsertMany(ctx, docs, options.InsertMany().SetOrdered(false))
	if err != nil {
		// With unordered inserts, duplicate errors are returned as BulkWriteException.
		// If every write error is a duplicate key, the operation is idempotent.
		if bulkErr, ok := err.(mongo.BulkWriteException); ok {
			allDuplicate := len(bulkErr.WriteErrors) > 0
			for _, we := range bulkErr.WriteErrors {
				if !IsDuplicateError(we) {
					allDuplicate = false
					break
				}
			}
			if allDuplicate {
				s.log.Debugf("duplicate p2p message ignored: client_msg_id=%s, sender=%d",
					msg.GetClientMsgId(), msg.GetSenderId())
				s.cacheMessageSender(ctx, msg.GetTopic(), topicSeq, msg.GetSenderId())
				return []int64{receiverID, msg.GetSenderId()}, true, nil
			}
		}
		return nil, false, fmt.Errorf("insert p2p inbox messages: %w", err)
	}

	s.cacheMessageSender(ctx, msg.GetTopic(), topicSeq, msg.GetSenderId())
	s.log.Debugf("p2p message saved: msg_id=%d, topic=%s, receiver=%d, sender=%d", msgID, msg.GetTopic(), receiverID, msg.GetSenderId())
	return []int64{receiverID, msg.GetSenderId()}, false, nil
}

// SaveGroupMessage saves a group message using read扩散.
// Only stores one copy in the group messages collection.
// Returns isDuplicate=true if the message already exists (idempotent).
func (s *MessageStorage) SaveGroupMessage(ctx context.Context, msg *pb.UpstreamMessage, msgID int64, topicSeq uint64) (bool, error) {
	s.incMongoOp(CollectionMessages, "insert")
	now := time.Now()
	stored := &StoredMessage{
		MsgID:       msgID,
		Topic:       msg.GetTopic(),
		SenderID:    msg.GetSenderId(),
		MsgType:     msg.GetMsgType(),
		Content:     msg.GetContent(),
		Timestamp:   msg.GetTimestamp(),
		TopicSeq:    topicSeq,
		ClientMsgID: msg.GetClientMsgId(),
		CreatedAt:   now,
	}

	_, err := s.collection(CollectionMessages).InsertOne(ctx, stored, options.InsertOne())
	if err != nil {
		if IsDuplicateError(err) {
			s.log.Debugf("duplicate group message ignored: client_msg_id=%s, sender=%d",
				msg.GetClientMsgId(), msg.GetSenderId())
			return true, nil
		}
		return false, fmt.Errorf("insert group message: %w", err)
	}

	s.cacheMessageSender(ctx, msg.GetTopic(), topicSeq, msg.GetSenderId())
	s.log.Debugf("group message saved: msg_id=%d, topic=%s", msgID, msg.GetTopic())
	return false, nil
}

// SaveGroupMessagesBatch inserts multiple group messages in one unordered
// InsertMany call. Duplicate key errors are ignored. This is used by the
// group message batcher to amortize MongoDB round-trip cost.
func (s *MessageStorage) SaveGroupMessagesBatch(ctx context.Context, msgs []*pb.UpstreamMessage, msgIDs []int64, topicSeqs []uint64) (int, bool, error) {
	if len(msgs) == 0 {
		return 0, false, nil
	}
	if len(msgs) != len(msgIDs) || len(msgs) != len(topicSeqs) {
		return 0, false, fmt.Errorf("batch length mismatch: msgs=%d ids=%d seqs=%d", len(msgs), len(msgIDs), len(topicSeqs))
	}

	s.incMongoOp(CollectionMessages, "insertMany")
	now := time.Now()
	docs := make([]interface{}, len(msgs))
	for i, msg := range msgs {
		docs[i] = &StoredMessage{
			MsgID:       msgIDs[i],
			Topic:       msg.GetTopic(),
			SenderID:    msg.GetSenderId(),
			MsgType:     msg.GetMsgType(),
			Content:     msg.GetContent(),
			Timestamp:   msg.GetTimestamp(),
			TopicSeq:    topicSeqs[i],
			ClientMsgID: msg.GetClientMsgId(),
			CreatedAt:   now,
		}
	}

	res, err := s.collection(CollectionMessages).InsertMany(ctx, docs, options.InsertMany().SetOrdered(false))
	if err != nil {
		if bulkErr, ok := err.(mongo.BulkWriteException); ok {
			allDuplicate := len(bulkErr.WriteErrors) > 0
			for _, we := range bulkErr.WriteErrors {
				if !IsDuplicateError(we) {
					allDuplicate = false
					break
				}
			}
			if allDuplicate {
				s.log.Debugf("duplicate group message batch ignored: count=%d", len(msgs))
				return 0, true, nil
			}
			// Partial success: some documents were inserted. Return inserted count
			// and mark the operation as not fully duplicate so the caller can
			// continue with per-message fallback if needed.
			inserted := len(res.InsertedIDs)
			s.log.Debugf("partial duplicate group message batch: count=%d inserted=%d", len(msgs), inserted)
			return inserted, false, nil
		}
		return 0, false, fmt.Errorf("insert group message batch: %w", err)
	}

	for i, msg := range msgs {
		s.cacheMessageSender(ctx, msg.GetTopic(), topicSeqs[i], msg.GetSenderId())
	}
	s.log.Debugf("group message batch saved: count=%d", len(msgs))
	return len(msgs), false, nil
}

// SaveSystemMessage saves a system notification using write扩散.
// Returns isDuplicate=true if the message already exists (idempotent).
func (s *MessageStorage) SaveSystemMessage(ctx context.Context, msg *pb.UpstreamMessage, msgID int64, topicSeq uint64) ([]int64, bool, error) {
	s.incMongoOp(CollectionInboxes, "insert")
	parts := strings.Split(msg.GetTopic(), "_")
	if len(parts) != 2 {
		return nil, false, fmt.Errorf("invalid system topic format: %s", msg.GetTopic())
	}
	var targetUID int64
	if _, err := fmt.Sscanf(parts[1], "%d", &targetUID); err != nil {
		return nil, false, fmt.Errorf("parse system topic user ID: %w", err)
	}

	now := time.Now()
	inbox := &InboxMessage{
		UserID:      targetUID,
		MsgID:       msgID,
		Topic:       msg.GetTopic(),
		SenderID:    msg.GetSenderId(),
		MsgType:     msg.GetMsgType(),
		Content:     msg.GetContent(),
		Timestamp:   msg.GetTimestamp(),
		TopicSeq:    topicSeq,
		ClientMsgID: msg.GetClientMsgId(),
		Read:        false,
		CreatedAt:   now,
	}

	_, err := s.collection(CollectionInboxes).InsertOne(ctx, inbox, options.InsertOne())
	if err != nil {
		if IsDuplicateError(err) {
			s.log.Debugf("duplicate system message ignored: client_msg_id=%s, sender=%d",
				msg.GetClientMsgId(), msg.GetSenderId())
			return []int64{targetUID}, true, nil
		}
		return nil, false, fmt.Errorf("insert system inbox message: %w", err)
	}

	s.cacheMessageSender(ctx, msg.GetTopic(), topicSeq, msg.GetSenderId())
	return []int64{targetUID}, false, nil
}

// BackupTopicSeq saves the current max seq for a topic to MongoDB as a fallback.
// Uses $max for atomic concurrency-safe updates.
func (s *MessageStorage) BackupTopicSeq(ctx context.Context, topic string, seq uint64) error {
	s.incMongoOp(CollectionTopicSeqs, "update")
	filter := bson.M{"topic": topic}
	update := bson.M{
		"$max": bson.M{
			"max_seq": seq,
		},
		"$setOnInsert": bson.M{
			"topic":      topic,
			"updated_at": time.Now(),
		},
	}
	opts := options.UpdateOne().SetUpsert(true)

	_, err := s.collection(CollectionTopicSeqs).UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("backup topic seq: %w", err)
	}
	return nil
}

// SaveMentionInbox saves @mention notifications for specified users.
// This implements the special write扩散 for @mentions in large groups (read扩散).
// Returns ErrDuplicateKey if a mention already exists (idempotent).
func (s *MessageStorage) SaveMentionInbox(ctx context.Context, msg *pb.UpstreamMessage, msgID int64, topicSeq uint64, mentionedUserIDs []int64) error {
	if len(mentionedUserIDs) == 0 {
		return nil
	}

	s.incMongoOp(CollectionMentionInboxes, "insert")
	now := time.Now()
	var docs []interface{}
	for _, uid := range mentionedUserIDs {
		docs = append(docs, &MentionMessage{
			UserID:      uid,
			MsgID:       msgID,
			Topic:       msg.GetTopic(),
			SenderID:    msg.GetSenderId(),
			MsgType:     msg.GetMsgType(),
			Content:     msg.GetContent(),
			Timestamp:   msg.GetTimestamp(),
			TopicSeq:    topicSeq,
			ClientMsgID: msg.GetClientMsgId(),
			Read:        false,
			CreatedAt:   now,
		})
	}

	_, err := s.collection(CollectionMentionInboxes).InsertMany(ctx, docs, options.InsertMany().SetOrdered(false))
	if err != nil {
		// With unordered inserts, duplicate errors are returned as BulkWriteException.
		// If every write error is a duplicate key, the operation is idempotent.
		if bulkErr, ok := err.(mongo.BulkWriteException); ok {
			allDuplicate := len(bulkErr.WriteErrors) > 0
			for _, we := range bulkErr.WriteErrors {
				if !IsDuplicateError(we) {
					allDuplicate = false
					break
				}
			}
			if allDuplicate {
				s.log.Debugf("duplicate mention inbox ignored: client_msg_id=%s",
					msg.GetClientMsgId())
				return nil
			}
		}
		return fmt.Errorf("insert mention inbox: %w", err)
	}

	s.log.Debugf("mention inbox saved: msg_id=%d, topic=%s, users=%v", msgID, msg.GetTopic(), mentionedUserIDs)
	return nil
}

// GetOfflineMessages retrieves offline messages for a user in a topic with seq > lastSeq.
// For P2P/system topics, queries the inbox collection.
// For group topics, also queries the messages collection (read扩散) and merges results.
func (s *MessageStorage) GetOfflineMessages(ctx context.Context, userID int64, topic string, lastSeq uint64, limit int) ([]*InboxMessage, error) {
	s.incMongoOp(CollectionInboxes, "find")
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	filter := bson.M{
		"user_id":   userID,
		"topic":     topic,
		"topic_seq": bson.M{"$gt": lastSeq},
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "topic_seq", Value: 1}}).
		SetLimit(int64(limit))

	cursor, err := s.collection(CollectionInboxes).Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find offline messages: %w", err)
	}
	defer cursor.Close(ctx)

	var results []*InboxMessage
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("decode offline messages: %w", err)
	}

	// For group topics, also fetch from messages collection (read扩散).
	topicType := ParseTopicType(topic)
	if topicType == TopicTypeGroup {
		groupMsgs, err := s.GetGroupMessages(ctx, topic, lastSeq, limit)
		if err != nil {
			s.log.Warnf("get group messages for offline pull failed: user_id=%d, topic=%s, err=%v", userID, topic, err)
		} else if len(groupMsgs) > 0 {
			// Merge group messages into results, converting StoredMessage -> InboxMessage.
			existing := make(map[uint64]bool, len(results))
			for _, m := range results {
				existing[m.TopicSeq] = true
			}
			for _, gm := range groupMsgs {
				if !existing[gm.TopicSeq] {
					results = append(results, &InboxMessage{
						UserID:      userID,
						MsgID:       gm.MsgID,
						Topic:       gm.Topic,
						SenderID:    gm.SenderID,
						MsgType:     gm.MsgType,
						Content:     gm.Content,
						Timestamp:   gm.Timestamp,
						TopicSeq:    gm.TopicSeq,
						ClientMsgID: gm.ClientMsgID,
						CreatedAt:   gm.CreatedAt,
					})
					existing[gm.TopicSeq] = true
				}
			}
			// Re-sort by topic_seq after merge.
			if len(results) > 1 {
				// Already sorted by individual queries, but merge may be out of order.
				// Use a simple sort to ensure order.
				// Actually both are sorted ascending, so we can merge with two-pointer.
				// But for simplicity, just sort.
				// Since the original results were already sorted, just append and sort.
			}
		}
	}

	// Ensure final sort by topic_seq and enforce the requested limit.
	if len(results) > 1 {
		sort.Slice(results, func(i, j int) bool {
			return results[i].TopicSeq < results[j].TopicSeq
		})
	}
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// GetGroupMessages retrieves group messages with seq > lastSeq.
func (s *MessageStorage) GetGroupMessages(ctx context.Context, topic string, lastSeq uint64, limit int) ([]*StoredMessage, error) {
	s.incMongoOp(CollectionMessages, "find")
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	filter := bson.M{
		"topic":     topic,
		"topic_seq": bson.M{"$gt": lastSeq},
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "topic_seq", Value: 1}}).
		SetLimit(int64(limit))

	cursor, err := s.collection(CollectionMessages).Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find group messages: %w", err)
	}
	defer cursor.Close(ctx)

	var results []*StoredMessage
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("decode group messages: %w", err)
	}
	return results, nil
}

// GetMentionMessages retrieves unread @mention messages for a user.
func (s *MessageStorage) GetMentionMessages(ctx context.Context, userID int64, topic string, lastSeq uint64, limit int) ([]*MentionMessage, error) {
	s.incMongoOp(CollectionMentionInboxes, "find")
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	filter := bson.M{
		"user_id":   userID,
		"topic":     topic,
		"topic_seq": bson.M{"$gt": lastSeq},
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "topic_seq", Value: 1}}).
		SetLimit(int64(limit))

	cursor, err := s.collection(CollectionMentionInboxes).Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find mention messages: %w", err)
	}
	defer cursor.Close(ctx)

	var results []*MentionMessage
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("decode mention messages: %w", err)
	}
	return results, nil
}

// GetTopicMaxSeq returns the current max seq for a topic from MongoDB backup.
func (s *MessageStorage) GetTopicMaxSeq(ctx context.Context, topic string) (uint64, error) {
	var result TopicSeqBackup
	err := s.collection(CollectionTopicSeqs).FindOne(ctx, bson.M{"topic": topic}).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return 0, nil
		}
		return 0, fmt.Errorf("get topic max seq: %w", err)
	}
	return result.MaxSeq, nil
}

// UpdateDeliveryStatus marks a message as delivered for a user.
// It updates inbox and mention_inbox in parallel (a message may exist in both
// for group @mentions that are also delivered via write扩散). If neither
// collection contains the message, it falls back to delivery_status for read扩散
// group messages. This keeps the common case to a single parallel round-trip.
func (s *MessageStorage) UpdateDeliveryStatus(ctx context.Context, userID int64, topic string, topicSeq uint64) error {
	s.incMongoOp(CollectionDeliveryStatus, "update")
	now := time.Now()
	filter := bson.M{"user_id": userID, "topic": topic, "topic_seq": topicSeq}
	update := bson.M{"$set": bson.M{"delivered_at": now}}

	type updateResult struct {
		matched int64
		err     error
	}
	results := make(chan updateResult, 2)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		res, err := s.collection(CollectionInboxes).UpdateOne(ctx, filter, update, options.UpdateOne())
		matched := int64(0)
		if err == nil && res != nil {
			matched = res.MatchedCount
		}
		results <- updateResult{matched: matched, err: err}
	}()
	go func() {
		defer wg.Done()
		res, err := s.collection(CollectionMentionInboxes).UpdateOne(ctx, filter, update, options.UpdateOne())
		matched := int64(0)
		if err == nil && res != nil {
			matched = res.MatchedCount
		}
		results <- updateResult{matched: matched, err: err}
	}()
	wg.Wait()
	close(results)

	anyMatched := false
	for r := range results {
		if r.err != nil {
			return fmt.Errorf("update delivery status: %w", r.err)
		}
		if r.matched > 0 {
			anyMatched = true
		}
	}

	if anyMatched {
		return nil
	}

	// Fall back to delivery_status for read扩散 group messages.
	ds := &DeliveryStatus{
		UserID:      userID,
		Topic:       topic,
		TopicSeq:    topicSeq,
		DeliveredAt: now,
	}
	_, err := s.collection(CollectionDeliveryStatus).InsertOne(ctx, ds, options.InsertOne())
	if err != nil {
		if IsDuplicateError(err) {
			return nil // already recorded, ignore
		}
		return fmt.Errorf("insert delivery status: %w", err)
	}

	return nil
}

// UpdateReadStatus marks messages as read up to a certain seq for a user.
// It updates both inbox and mention_inbox collections.
func (s *MessageStorage) UpdateReadStatus(ctx context.Context, userID int64, topic string, upToSeq uint64) error {
	s.incMongoOp(CollectionInboxes, "update")
	now := time.Now()
	filter := bson.M{
		"user_id":   userID,
		"topic":     topic,
		"topic_seq": bson.M{"$lte": upToSeq},
		"read":      bson.M{"$ne": true},
	}
	update := bson.M{"$set": bson.M{"read": true, "read_at": now}}

	// Update inbox
	inboxRes, err := s.collection(CollectionInboxes).UpdateMany(ctx, filter, update, options.UpdateMany())
	if err != nil {
		return fmt.Errorf("update inbox read status: %w", err)
	}

	// Update mention inbox
	mentionRes, err := s.collection(CollectionMentionInboxes).UpdateMany(ctx, filter, update, options.UpdateMany())
	if err != nil {
		return fmt.Errorf("update mention inbox read status: %w", err)
	}

	s.log.Debugf("read status updated: user_id=%d, topic=%s, up_to_seq=%d, inbox=%d, mention=%d",
		userID, topic, upToSeq, inboxRes.ModifiedCount, mentionRes.ModifiedCount)
	return nil
}

// RecallMessage marks a message as recalled in every storage representation.
// The caller must authorize the sender before invoking this method.
func (s *MessageStorage) RecallMessage(ctx context.Context, topic string, topicSeq uint64, msgID int64, senderID int64, window time.Duration) error {
	filter := bson.M{"topic": topic, "topic_seq": topicSeq, "sender_id": senderID, "recalled": bson.M{"$ne": true}}
	if msgID > 0 {
		filter["msg_id"] = msgID
	}
	if window > 0 {
		filter["created_at"] = bson.M{"$gte": time.Now().Add(-window)}
	}
	update := bson.M{"$set": bson.M{"recalled": true, "content": []byte(""), "recalled_at": time.Now()}}
	matched := int64(0)
	for _, name := range []string{CollectionMessages, CollectionInboxes, CollectionMentionInboxes} {
		result, err := s.collection(name).UpdateMany(ctx, filter, update, options.UpdateMany())
		if err != nil {
			return fmt.Errorf("recall message in %s: %w", name, err)
		}
		matched += result.MatchedCount
	}
	if matched == 0 {
		return ErrRecallNotFound
	}
	return nil
}

// GetMessageSender returns the sender_id of a message by topic and topic_seq.
// It first checks Redis cache, then searches inbox, mention_inbox, and messages
// collections in parallel (#25).
func (s *MessageStorage) GetMessageSender(ctx context.Context, topic string, topicSeq uint64) (int64, error) {
	if senderID, ok := s.getCachedMessageSender(ctx, topic, topicSeq); ok {
		return senderID, nil
	}

	type result struct {
		senderID int64
		err      error
	}

	ch := make(chan result, 3)

	// Query all three collections concurrently
	go func() {
		var doc struct {
			SenderID int64 `bson:"sender_id"`
		}
		err := s.collection(CollectionInboxes).FindOne(ctx, bson.M{
			"topic":     topic,
			"topic_seq": topicSeq,
		}).Decode(&doc)
		if err == nil {
			ch <- result{senderID: doc.SenderID}
			return
		}
		ch <- result{err: err}
	}()

	go func() {
		var doc struct {
			SenderID int64 `bson:"sender_id"`
		}
		err := s.collection(CollectionMentionInboxes).FindOne(ctx, bson.M{
			"topic":     topic,
			"topic_seq": topicSeq,
		}).Decode(&doc)
		if err == nil {
			ch <- result{senderID: doc.SenderID}
			return
		}
		ch <- result{err: err}
	}()

	go func() {
		var doc struct {
			SenderID int64 `bson:"sender_id"`
		}
		err := s.collection(CollectionMessages).FindOne(ctx, bson.M{
			"topic":     topic,
			"topic_seq": topicSeq,
		}).Decode(&doc)
		if err == nil {
			ch <- result{senderID: doc.SenderID}
			return
		}
		ch <- result{err: err}
	}()

	var lastErr error
	for i := 0; i < 3; i++ {
		r := <-ch
		if r.err == nil {
			s.cacheMessageSender(ctx, topic, topicSeq, r.senderID)
			return r.senderID, nil
		}
		lastErr = r.err
	}

	if lastErr == mongo.ErrNoDocuments {
		return 0, fmt.Errorf("message not found: topic=%s, seq=%d", topic, topicSeq)
	}
	return 0, fmt.Errorf("find message sender: %w", lastErr)
}
