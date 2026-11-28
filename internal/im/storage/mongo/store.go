package mongostore

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
	"nonoka-im/internal/metrics"
)

var (
	ErrRecallNotFound         = errors.New("message not found or already recalled")
	ErrRecallExpired          = errors.New("message recall window expired")
	errTransactionalDuplicate = errors.New("message already persisted")
)

const (
	// Collection names
	CollectionMessages       = "messages"            // group messages (read扩散)
	CollectionInboxes        = "inboxes"             // user inboxes (write扩散 for P2P)
	CollectionTopicSeqs      = "topic_seqs"          // topic max seq backup
	CollectionMentionInboxes = "mention_inboxes"     // group @mention inbox (for large groups)
	CollectionDeliveryStatus = "delivery_status"     // message delivery tracking per user
	CollectionAgentOutbox    = "agent_event_outbox"  // transactional Agent trigger events
	CollectionSearchOutbox   = "search_index_outbox" // durable Meilisearch index tasks

	// agentEventMaxAttempts caps per-row publish attempts in the outbox relay.
	// Rows exceeding it are parked with status "failed" (never deleted) so an
	// operator can inspect and replay them manually.
	agentEventMaxAttempts = 50
	// agentEventPublishedRetention bounds how long published outbox rows are
	// kept before the relay prunes them during normal operation.
	agentEventPublishedRetention = 24 * time.Hour
	// agentEventPruneLimit bounds a single prune pass so cleanup cannot stall
	// the relay when a large backlog of published rows accumulates.
	agentEventPruneLimit = 500

	// senderCacheKeyPrefix is the Redis key prefix for message sender cache.
	senderCacheKeyPrefix = "im:sender"
	// senderCacheTTL is the time-to-live for sender cache entries.
	senderCacheTTL = 24 * time.Hour
)

// storedMessageRow represents a message stored in MongoDB (read扩散 for groups).
type storedMessageRow struct {
	MsgID       int64     `bson:"msg_id"`
	Topic       string    `bson:"topic"`
	SenderID    int64     `bson:"sender_id"`
	MsgType     int32     `bson:"msg_type"`
	Content     []byte    `bson:"content"`
	ContentText string    `bson:"content_text,omitempty"`
	Timestamp   int64     `bson:"timestamp"`
	TopicSeq    uint64    `bson:"topic_seq"`
	ClientMsgID string    `bson:"client_msg_id,omitempty"`
	Recalled    bool      `bson:"recalled,omitempty"`
	CreatedAt   time.Time `bson:"created_at"`
}

// inboxMessageRow represents a message in user's inbox (write扩散).
type inboxMessageRow struct {
	UserID      int64     `bson:"user_id"`
	MsgID       int64     `bson:"msg_id"`
	Topic       string    `bson:"topic"`
	SenderID    int64     `bson:"sender_id"`
	MsgType     int32     `bson:"msg_type"`
	Content     []byte    `bson:"content"`
	ContentText string    `bson:"content_text,omitempty"`
	Timestamp   int64     `bson:"timestamp"`
	TopicSeq    uint64    `bson:"topic_seq"`
	ClientMsgID string    `bson:"client_msg_id,omitempty"`
	Recalled    bool      `bson:"recalled,omitempty"`
	Read        bool      `bson:"read"`
	DeliveredAt time.Time `bson:"delivered_at,omitempty"`
	CreatedAt   time.Time `bson:"created_at"`
}

// topicSeqBackupRow stores the max seq for a topic as a fallback.
type topicSeqBackupRow struct {
	Topic     string    `bson:"topic"`
	MaxSeq    uint64    `bson:"max_seq"`
	UpdatedAt time.Time `bson:"updated_at"`
}

// mentionMessageRow stores @mention notifications for users in large groups.
// This is a special write扩散 for @mentions in read扩散 groups.
type mentionMessageRow struct {
	UserID      int64     `bson:"user_id"`
	MsgID       int64     `bson:"msg_id"`
	Topic       string    `bson:"topic"`
	SenderID    int64     `bson:"sender_id"`
	MsgType     int32     `bson:"msg_type"`
	Content     []byte    `bson:"content"`
	ContentText string    `bson:"content_text,omitempty"`
	Timestamp   int64     `bson:"timestamp"`
	TopicSeq    uint64    `bson:"topic_seq"`
	ClientMsgID string    `bson:"client_msg_id,omitempty"`
	Recalled    bool      `bson:"recalled,omitempty"`
	Read        bool      `bson:"read"`
	DeliveredAt time.Time `bson:"delivered_at,omitempty"`
	CreatedAt   time.Time `bson:"created_at"`
}

// deliveryStatusRow tracks whether a message has been delivered to a specific user.
// Used for both write扩散 and read扩散 messages.
type deliveryStatusRow struct {
	UserID      int64     `bson:"user_id"`
	MsgID       int64     `bson:"msg_id"`
	Topic       string    `bson:"topic"`
	TopicSeq    uint64    `bson:"topic_seq"`
	DeliveredAt time.Time `bson:"delivered_at"`
}

// outboxRow is written in the same MongoDB transaction as the IM
// message. The relay publishes Payload to Kafka and then marks the row
// published. Rows are retained after publishing for audit and duplicate
// detection; event_id is the idempotency boundary.
type outboxRow struct {
	ID            bson.ObjectID `bson:"_id,omitempty"`
	EventID       string        `bson:"event_id"`
	MsgID         int64         `bson:"msg_id"`
	Topic         string        `bson:"topic"`
	TopicSeq      uint64        `bson:"topic_seq"`
	Payload       []byte        `bson:"payload"`
	Status        string        `bson:"status"`
	Attempts      int           `bson:"attempts"`
	NextAttemptAt time.Time     `bson:"next_attempt_at"`
	LockedUntil   time.Time     `bson:"locked_until,omitempty"`
	LastError     string        `bson:"last_error,omitempty"`
	CreatedAt     time.Time     `bson:"created_at"`
	PublishedAt   time.Time     `bson:"published_at,omitempty"`
}

// Sentinels used inside the message+outbox transaction. DuplicateKeyError from
// MongoDB aborts a transaction on the server; swallowing it makes the later
// commit fail with NoSuchTransaction and the driver retries the callback until
// its deadline. Returning this sentinel lets WithTransaction abort cleanly with
// a deterministic duplicate outcome.
var errAgentEventDuplicate = errors.New("agent event already recorded")

// Store handles MongoDB persistence for messages.
type Store struct {
	db            *mongo.Database
	redis         redis.UniversalClient
	metrics       *metrics.Metrics
	log           *log.Helper
	writeConcern  *writeconcern.WriteConcern
	searchEnabled bool
}

// NewStore creates a new Store.
func NewStore(db *mongo.Database, logger log.Logger, m ...*metrics.Metrics) *Store {
	s := &Store{
		db:           db,
		log:          log.NewHelper(logger),
		writeConcern: writeconcern.Majority(),
	}
	if len(m) > 0 {
		s.metrics = m[0]
	}
	return s
}

// SetWriteConcern configures the MongoDB write concern from a string label.
// Supported labels:
//   - "majority"       : w="majority", journal follows the server default
//     (durable for a replica set; recommended for IM messages)
//   - "majority_jfalse": w="majority", j=false (lower latency, crash-loss window)
//   - "w1"             : w=1, j=false (fastest, not crash-safe; opt-in only)
//
// SetSearchEnabled makes message writes enqueue durable search-index tasks in
// the same MongoDB transaction. It must be set before the consumer starts.
func (s *Store) SetSearchEnabled(enabled bool) { s.searchEnabled = enabled }

func (s *Store) SetWriteConcern(label string) {
	switch strings.ToLower(label) {
	case "majority":
		s.writeConcern = writeconcern.Majority()
	case "majority_jfalse":
		journal := false
		s.writeConcern = &writeconcern.WriteConcern{W: writeconcern.WCMajority, Journal: &journal}
	case "w1", "", "default":
		fallthrough
	default:
		s.writeConcern = writeconcern.W1()
	}
}

// collection returns a collection handle configured with the current write concern.
func (s *Store) collection(name string) *mongo.Collection {
	return s.db.Collection(name, options.Collection().SetWriteConcern(s.writeConcern))
}

// SetMetrics configures the metrics collector for storage operations.
func (s *Store) SetMetrics(m *metrics.Metrics) {
	s.metrics = m
}

// SetRedis configures an optional Redis client for caches (e.g., sender lookup).
func (s *Store) SetRedis(redis redis.UniversalClient) {
	s.redis = redis
}

// incMongoOp increments the MongoDB operation counter if metrics is configured.
func (s *Store) incMongoOp(collection, op string) {
	if s.metrics != nil {
		s.metrics.IncMongoOps(collection, op)
	}
}

// senderCacheKey returns the Redis key for caching a message's sender.
func senderCacheKey(topic string, topicSeq uint64) string {
	return fmt.Sprintf("%s:%s:%d", senderCacheKeyPrefix, topic, topicSeq)
}

// cacheMessageSender writes the sender ID to Redis for fast lookup.
func (s *Store) cacheMessageSender(ctx context.Context, topic string, topicSeq uint64, senderID int64) {
	if s.redis == nil {
		return
	}
	key := senderCacheKey(topic, topicSeq)
	if err := s.redis.Set(ctx, key, senderID, senderCacheTTL).Err(); err != nil {
		s.log.Warnf("cache message sender failed: topic=%s seq=%d err=%v", topic, topicSeq, err)
	}
}

// getCachedMessageSender reads the sender ID from Redis cache if available.
func (s *Store) getCachedMessageSender(ctx context.Context, topic string, topicSeq uint64) (int64, bool) {
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

// IsDuplicateError checks if a MongoDB error is a duplicate key error.
func IsDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	return mongo.IsDuplicateKeyError(err)
}

// Public read methods map storage rows to domain types so the application
// layer never sees MongoDB structs.

// Exported row aliases are used by storage/adapters that need to decode the
// MongoDB document shape without exposing the private struct names.
type (
	StoredMessage  = storedMessageRow
	InboxMessage   = inboxMessageRow
	MentionMessage = mentionMessageRow
	TopicSeqBackup = topicSeqBackupRow
)
