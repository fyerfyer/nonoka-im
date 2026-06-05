package msgworker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	pb "nonoka-im/api/im/v1"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	// Collection names
	CollectionMessages       = "messages"        // group messages (read扩散)
	CollectionInboxes        = "inboxes"         // user inboxes (write扩散 for P2P)
	CollectionTopicSeqs      = "topic_seqs"      // topic max seq backup
	CollectionMentionInboxes = "mention_inboxes" // group @mention inbox (for large groups)
	CollectionDeliveryStatus = "delivery_status" // message delivery tracking per user
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
	db  *mongo.Database
	log *log.Helper
}

// NewMessageStorage creates a new MessageStorage.
func NewMessageStorage(db *mongo.Database, logger log.Logger) *MessageStorage {
	return &MessageStorage{
		db:  db,
		log: log.NewHelper(logger),
	}
}

// EnsureIndexes creates necessary indexes.
func (s *MessageStorage) EnsureIndexes(ctx context.Context) error {
	// Drop all old indexes first to avoid conflicts when index options change.
	for _, coll := range []string{CollectionMessages, CollectionInboxes, CollectionTopicSeqs, CollectionMentionInboxes, CollectionDeliveryStatus} {
		_ = s.db.Collection(coll).Indexes().DropAll(ctx)
	}

	// Index for group messages: topic + topic_seq
	_, err := s.db.Collection(CollectionMessages).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "topic", Value: 1}, {Key: "topic_seq", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("create messages index: %w", err)
	}

	// Unique index on client_msg_id + sender_id for deduplication (messages collection)
	// Only index documents where client_msg_id exists and is not null.
	_, err = s.db.Collection(CollectionMessages).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "sender_id", Value: 1}, {Key: "client_msg_id", Value: 1}},
		Options: options.Index().SetUnique(true).SetPartialFilterExpression(
			bson.M{"client_msg_id": bson.M{"$exists": true}},
		),
	})
	if err != nil {
		return fmt.Errorf("create messages dedup index: %w", err)
	}

	// Index for inbox: user_id + topic + topic_seq
	_, err = s.db.Collection(CollectionInboxes).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "topic", Value: 1}, {Key: "topic_seq", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("create inbox index: %w", err)
	}

	// Unique index on client_msg_id + sender_id for inbox deduplication
	_, err = s.db.Collection(CollectionInboxes).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "sender_id", Value: 1}, {Key: "client_msg_id", Value: 1}},
		Options: options.Index().SetUnique(true).SetPartialFilterExpression(
			bson.M{"client_msg_id": bson.M{"$exists": true}},
		),
	})
	if err != nil {
		return fmt.Errorf("create inbox dedup index: %w", err)
	}

	// Index for topic_seqs
	_, err = s.db.Collection(CollectionTopicSeqs).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "topic", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("create topic_seq index: %w", err)
	}

	// Index for mention_inboxes: user_id + topic + topic_seq
	_, err = s.db.Collection(CollectionMentionInboxes).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "topic", Value: 1}, {Key: "topic_seq", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("create mention_inbox index: %w", err)
	}

	// Unique index on client_msg_id + sender_id for mention inbox deduplication
	_, err = s.db.Collection(CollectionMentionInboxes).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "sender_id", Value: 1}, {Key: "client_msg_id", Value: 1}},
		Options: options.Index().SetUnique(true).SetPartialFilterExpression(
			bson.M{"client_msg_id": bson.M{"$exists": true}},
		),
	})
	if err != nil {
		return fmt.Errorf("create mention_inbox dedup index: %w", err)
	}

	// Index for delivery_status: user_id + topic + topic_seq (for fast ACK lookups)
	_, err = s.db.Collection(CollectionDeliveryStatus).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "topic", Value: 1}, {Key: "topic_seq", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("create delivery_status index: %w", err)
	}

	return nil
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

// IsDuplicateError checks if a MongoDB error is a duplicate key error.
func IsDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	return mongo.IsDuplicateKeyError(err)
}

// SaveP2PMessage saves a P2P message using write扩散.
// Writes the message to the receiver's inbox.
// Returns ErrDuplicateKey if the message already exists (idempotent).
func (s *MessageStorage) SaveP2PMessage(ctx context.Context, msg *pb.UpstreamMessage, msgID int64, topicSeq uint64) ([]int64, error) {
	uid1, uid2, err := ExtractUserIDsFromP2PTopic(msg.GetTopic())
	if err != nil {
		return nil, err
	}

	var receiverID int64
	if msg.GetSenderId() == uid1 {
		receiverID = uid2
	} else {
		receiverID = uid1
	}

	now := time.Now()
	inbox := &InboxMessage{
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
	}

	_, err = s.db.Collection(CollectionInboxes).InsertOne(ctx, inbox)
	if err != nil {
		if IsDuplicateError(err) {
			s.log.Debugf("duplicate p2p message ignored: client_msg_id=%s, sender=%d",
				msg.GetClientMsgId(), msg.GetSenderId())
			return []int64{receiverID}, nil
		}
		return nil, fmt.Errorf("insert inbox message: %w", err)
	}

	s.log.Debugf("p2p message saved: msg_id=%d, topic=%s, receiver=%d", msgID, msg.GetTopic(), receiverID)
	return []int64{receiverID}, nil
}

// SaveGroupMessage saves a group message using read扩散.
// Only stores one copy in the group messages collection.
// Returns ErrDuplicateKey if the message already exists (idempotent).
func (s *MessageStorage) SaveGroupMessage(ctx context.Context, msg *pb.UpstreamMessage, msgID int64, topicSeq uint64) error {
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

	_, err := s.db.Collection(CollectionMessages).InsertOne(ctx, stored)
	if err != nil {
		if IsDuplicateError(err) {
			s.log.Debugf("duplicate group message ignored: client_msg_id=%s, sender=%d",
				msg.GetClientMsgId(), msg.GetSenderId())
			return nil
		}
		return fmt.Errorf("insert group message: %w", err)
	}

	s.log.Debugf("group message saved: msg_id=%d, topic=%s", msgID, msg.GetTopic())
	return nil
}

// SaveSystemMessage saves a system notification using write扩散.
// Returns ErrDuplicateKey if the message already exists (idempotent).
func (s *MessageStorage) SaveSystemMessage(ctx context.Context, msg *pb.UpstreamMessage, msgID int64, topicSeq uint64) ([]int64, error) {
	parts := strings.Split(msg.GetTopic(), "_")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid system topic format: %s", msg.GetTopic())
	}
	var targetUID int64
	if _, err := fmt.Sscanf(parts[1], "%d", &targetUID); err != nil {
		return nil, fmt.Errorf("parse system topic user ID: %w", err)
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

	_, err := s.db.Collection(CollectionInboxes).InsertOne(ctx, inbox)
	if err != nil {
		if IsDuplicateError(err) {
			s.log.Debugf("duplicate system message ignored: client_msg_id=%s, sender=%d",
				msg.GetClientMsgId(), msg.GetSenderId())
			return []int64{targetUID}, nil
		}
		return nil, fmt.Errorf("insert system inbox message: %w", err)
	}

	return []int64{targetUID}, nil
}

// BackupTopicSeq saves the current max seq for a topic to MongoDB as a fallback.
// Uses $max for atomic concurrency-safe updates.
func (s *MessageStorage) BackupTopicSeq(ctx context.Context, topic string, seq uint64) error {
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

	_, err := s.db.Collection(CollectionTopicSeqs).UpdateOne(ctx, filter, update, opts)
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

	_, err := s.db.Collection(CollectionMentionInboxes).InsertMany(ctx, docs)
	if err != nil {
		// Check if all errors are duplicate key errors (idempotent batch insert)
		if bulkErr, ok := err.(mongo.BulkWriteException); ok {
			allDuplicate := true
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
// For P2P/system topics, queries the inbox collection. For group topics, queries messages collection.
func (s *MessageStorage) GetOfflineMessages(ctx context.Context, userID int64, topic string, lastSeq uint64, limit int) ([]*InboxMessage, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	filter := bson.M{
		"user_id": userID,
		"topic":   topic,
		"topic_seq": bson.M{"$gt": lastSeq},
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "topic_seq", Value: 1}}).
		SetLimit(int64(limit))

	cursor, err := s.db.Collection(CollectionInboxes).Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find offline messages: %w", err)
	}
	defer cursor.Close(ctx)

	var results []*InboxMessage
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("decode offline messages: %w", err)
	}
	return results, nil
}

// GetGroupMessages retrieves group messages with seq > lastSeq.
func (s *MessageStorage) GetGroupMessages(ctx context.Context, topic string, lastSeq uint64, limit int) ([]*StoredMessage, error) {
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

	cursor, err := s.db.Collection(CollectionMessages).Find(ctx, filter, opts)
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
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	filter := bson.M{
		"user_id": userID,
		"topic":   topic,
		"topic_seq": bson.M{"$gt": lastSeq},
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "topic_seq", Value: 1}}).
		SetLimit(int64(limit))

	cursor, err := s.db.Collection(CollectionMentionInboxes).Find(ctx, filter, opts)
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
	err := s.db.Collection(CollectionTopicSeqs).FindOne(ctx, bson.M{"topic": topic}).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return 0, nil
		}
		return 0, fmt.Errorf("get topic max seq: %w", err)
	}
	return result.MaxSeq, nil
}

// UpdateDeliveryStatus marks a message as delivered for a user.
// It updates the inbox/mention_inbox directly and also records in delivery_status.
func (s *MessageStorage) UpdateDeliveryStatus(ctx context.Context, userID int64, topic string, topicSeq uint64) error {
	now := time.Now()

	// Try to update inbox first
	inboxFilter := bson.M{"user_id": userID, "topic": topic, "topic_seq": topicSeq}
	inboxUpdate := bson.M{"$set": bson.M{"delivered_at": now}}
	inboxRes, err := s.db.Collection(CollectionInboxes).UpdateOne(ctx, inboxFilter, inboxUpdate)
	if err != nil {
		return fmt.Errorf("update inbox delivery status: %w", err)
	}

	// If not found in inbox, try mention_inbox
	if inboxRes.MatchedCount == 0 {
		mentionFilter := bson.M{"user_id": userID, "topic": topic, "topic_seq": topicSeq}
		mentionUpdate := bson.M{"$set": bson.M{"delivered_at": now}}
		mentionRes, err := s.db.Collection(CollectionMentionInboxes).UpdateOne(ctx, mentionFilter, mentionUpdate)
		if err != nil {
			return fmt.Errorf("update mention inbox delivery status: %w", err)
		}

		// Also record in delivery_status for read扩散 (group) messages
		if mentionRes.MatchedCount == 0 {
			ds := &DeliveryStatus{
				UserID:      userID,
				Topic:       topic,
				TopicSeq:    topicSeq,
				DeliveredAt: now,
			}
			_, err := s.db.Collection(CollectionDeliveryStatus).InsertOne(ctx, ds)
			if err != nil {
				if IsDuplicateError(err) {
					return nil // already recorded, ignore
				}
				return fmt.Errorf("insert delivery status: %w", err)
			}
		}
	}

	return nil
}
