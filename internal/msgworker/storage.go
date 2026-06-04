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
	CollectionMessages  = "messages"   // group messages (read扩散)
	CollectionInboxes   = "inboxes"    // user inboxes (write扩散 for P2P)
	CollectionTopicSeqs = "topic_seqs" // topic max seq backup
)

// StoredMessage represents a message stored in MongoDB.
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
	UserID    int64     `bson:"user_id"`
	MsgID     int64     `bson:"msg_id"`
	Topic     string    `bson:"topic"`
	SenderID  int64     `bson:"sender_id"`
	MsgType   int32     `bson:"msg_type"`
	Content   []byte    `bson:"content"`
	Timestamp int64     `bson:"timestamp"`
	TopicSeq  uint64    `bson:"topic_seq"`
	Read      bool      `bson:"read"`
	CreatedAt time.Time `bson:"created_at"`
}

// TopicSeqBackup stores the max seq for a topic as a fallback.
type TopicSeqBackup struct {
	Topic     string    `bson:"topic"`
	MaxSeq    uint64    `bson:"max_seq"`
	UpdatedAt time.Time `bson:"updated_at"`
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
	// Index for group messages: topic + topic_seq
	_, err := s.db.Collection(CollectionMessages).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "topic", Value: 1}, {Key: "topic_seq", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("create messages index: %w", err)
	}

	// Index for inbox: user_id + topic + topic_seq
	_, err = s.db.Collection(CollectionInboxes).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "topic", Value: 1}, {Key: "topic_seq", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("create inbox index: %w", err)
	}

	// Index for topic_seqs
	_, err = s.db.Collection(CollectionTopicSeqs).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "topic", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("create topic_seq index: %w", err)
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

// SaveP2PMessage saves a P2P message using write扩散.
// Writes the message to the receiver's inbox.
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
		UserID:    receiverID,
		MsgID:     msgID,
		Topic:     msg.GetTopic(),
		SenderID:  msg.GetSenderId(),
		MsgType:   msg.GetMsgType(),
		Content:   msg.GetContent(),
		Timestamp: msg.GetTimestamp(),
		TopicSeq:  topicSeq,
		Read:      false,
		CreatedAt: now,
	}

	_, err = s.db.Collection(CollectionInboxes).InsertOne(ctx, inbox)
	if err != nil {
		return nil, fmt.Errorf("insert inbox message: %w", err)
	}

	s.log.Debugf("p2p message saved: msg_id=%d, topic=%s, receiver=%d", msgID, msg.GetTopic(), receiverID)
	return []int64{receiverID}, nil
}

// SaveGroupMessage saves a group message using read扩散.
// Only stores one copy in the group messages collection.
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
		return fmt.Errorf("insert group message: %w", err)
	}

	s.log.Debugf("group message saved: msg_id=%d, topic=%s", msgID, msg.GetTopic())
	return nil
}

// SaveSystemMessage saves a system notification using write扩散.
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
		UserID:    targetUID,
		MsgID:     msgID,
		Topic:     msg.GetTopic(),
		SenderID:  msg.GetSenderId(),
		MsgType:   msg.GetMsgType(),
		Content:   msg.GetContent(),
		Timestamp: msg.GetTimestamp(),
		TopicSeq:  topicSeq,
		Read:      false,
		CreatedAt: now,
	}

	_, err := s.db.Collection(CollectionInboxes).InsertOne(ctx, inbox)
	if err != nil {
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
