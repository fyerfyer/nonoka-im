package integration

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/gateway"
	"nonoka-im/internal/msgworker"

	pb "nonoka-im/api/im/v1"
	"github.com/segmentio/kafka-go"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
)

// recordingPusher is a test double that records BatchPushToUsers calls.
type recordingPusher struct {
	mu      sync.Mutex
	batches []batchRecord
	closed  bool
	failErr error
}

type batchRecord struct {
	userIDs []int64
	msg     *pb.MessagePush
}

func (p *recordingPusher) BatchPushToUsers(ctx context.Context, userIDs []int64, msg *pb.MessagePush) (int32, []int64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0, userIDs, fmt.Errorf("pusher closed")
	}
	p.batches = append(p.batches, batchRecord{userIDs: append([]int64{}, userIDs...), msg: msg})
	return int32(len(userIDs)), nil, p.failErr
}

func (p *recordingPusher) PushToUser(ctx context.Context, userID int64, msg *pb.MessagePush) (int32, error) {
	_, _, err := p.BatchPushToUsers(ctx, []int64{userID}, msg)
	return 1, err
}

func (p *recordingPusher) PushReceiptToUser(ctx context.Context, userID int64, receipt *pb.SendReceipt) (int32, error) {
	return 1, nil
}

func (p *recordingPusher) BatchPushReceiptToUsers(ctx context.Context, userIDs []int64, receipt *pb.SendReceipt) (int32, []int64, error) {
	return int32(len(userIDs)), nil, nil
}

func (p *recordingPusher) Close() error {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	return nil
}

func (p *recordingPusher) Batches() []batchRecord {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]batchRecord, len(p.batches))
	copy(out, p.batches)
	return out
}

// staticGroupMemberService is a test double for group membership.
type staticGroupMemberService struct {
	members map[string][]int64
}

func (s *staticGroupMemberService) GetGroupMembers(ctx context.Context, groupID string) ([]int64, error) {
	return s.members[groupID], nil
}

// TestMsgWorker_GroupMention_NoDuplicatePush verifies that a user who is both
// @mentioned and a group member receives the group message only once.
func TestMsgWorker_GroupMention_NoDuplicatePush(t *testing.T) {
	worker, db, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	senderID := int64(100)
	mentionedUserID := int64(200)
	otherMemberID := int64(300)
	groupID := "42"
	topic := "grp_" + groupID

	pusher := &recordingPusher{}
	worker.SetGroupMemberService(&staticGroupMemberService{
		members: map[string][]int64{
			groupID: {senderID, mentionedUserID, otherMemberID},
		},
	})
	worker.SetPusher(pusher)

	upstream := &v1.UpstreamMessage{
		SenderId:         senderID,
		Topic:            topic,
		MsgType:          int32(v1.MsgType_MSG_TYPE_TEXT),
		Content:          []byte("hello @200"),
		ClientMsgId:      "group-mention-001",
		Timestamp:        time.Now().UnixMilli(),
		MentionedUserIds: []int64{mentionedUserID},
	}
	data, _ := proto.Marshal(upstream)

	if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
		t.Fatalf("handle group message failed: %v", err)
	}

	// Count how many times mentionedUserID appears in push batches.
	pushCount := 0
	for _, b := range pusher.Batches() {
		for _, uid := range b.userIDs {
			if uid == mentionedUserID {
				pushCount++
			}
		}
	}
	if pushCount != 1 {
		t.Fatalf("expected mentioned user to receive exactly 1 push, got %d", pushCount)
	}

	// Verify other member still got pushed.
	otherPushed := false
	for _, b := range pusher.Batches() {
		for _, uid := range b.userIDs {
			if uid == otherMemberID {
				otherPushed = true
			}
		}
	}
	if !otherPushed {
		t.Fatalf("expected other member %d to receive a push", otherMemberID)
	}

	// Verify only one group message stored.
	msgColl := db.Collection(msgworker.CollectionMessages)
	count := countMongoDocs(t, msgColl, bson.M{"topic": topic})
	if count != 1 {
		t.Fatalf("expected 1 group message, got %d", count)
	}

	t.Logf("group mention dedup verified: mentioned user got 1 push, other member got 1 push")
}

// TestMsgWorker_UpdateDeliveryStatus_BothCollections verifies that ACK updates
// the delivered_at field in both inbox and mention_inbox without misattributing
// the MatchedCount between collections.
func TestMsgWorker_UpdateDeliveryStatus_BothCollections(t *testing.T) {
	_, db, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	userID := int64(100)
	topic := "grp_42"
	topicSeq := uint64(5)
	now := time.Now()

	// Insert an inbox message for this user/topic/seq.
	inboxColl := db.Collection(msgworker.CollectionInboxes)
	if _, err := inboxColl.InsertOne(ctx, msgworker.InboxMessage{
		UserID:    userID,
		MsgID:     1,
		Topic:     topic,
		TopicSeq:  topicSeq,
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert inbox message failed: %v", err)
	}

	// Insert a mention_inbox message for the same user/topic/seq.
	mentionColl := db.Collection(msgworker.CollectionMentionInboxes)
	if _, err := mentionColl.InsertOne(ctx, msgworker.MentionMessage{
		UserID:    userID,
		MsgID:     1,
		Topic:     topic,
		TopicSeq:  topicSeq,
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert mention inbox message failed: %v", err)
	}

	storage := msgworker.NewMessageStorage(db, testLogger)
	if err := storage.UpdateDeliveryStatus(ctx, userID, topic, topicSeq); err != nil {
		t.Fatalf("update delivery status failed: %v", err)
	}

	var inbox msgworker.InboxMessage
	if err := inboxColl.FindOne(ctx, bson.M{"user_id": userID, "topic": topic, "topic_seq": topicSeq}).Decode(&inbox); err != nil {
		t.Fatalf("find inbox message failed: %v", err)
	}
	if inbox.DeliveredAt.IsZero() {
		t.Fatal("expected inbox delivered_at to be set")
	}

	var mention msgworker.MentionMessage
	if err := mentionColl.FindOne(ctx, bson.M{"user_id": userID, "topic": topic, "topic_seq": topicSeq}).Decode(&mention); err != nil {
		t.Fatalf("find mention inbox message failed: %v", err)
	}
	if mention.DeliveredAt.IsZero() {
		t.Fatal("expected mention_inbox delivered_at to be set")
	}

	t.Log("UpdateDeliveryStatus correctly updated both inbox and mention_inbox")
}

// TestMsgWorker_KafkaConsumer_ParallelWorkers verifies that a Kafka consumer
// with multiple workers processes all messages from a multi-partition topic.
func TestMsgWorker_KafkaConsumer_ParallelWorkers(t *testing.T) {
	if err := waitForKafka(testKafkaBroker); err != nil {
		t.Fatalf("kafka not ready: %v", err)
	}

	topic := "msgworker-parallel-test-" + fmt.Sprintf("%d", time.Now().UnixNano())
	if err := cleanupAndCreateTopic(testKafkaBroker, topic, 4); err != nil {
		t.Fatalf("failed to create kafka topic: %v", err)
	}
	defer func() {
		conn, _ := kafka.Dial("tcp", testKafkaBroker)
		if conn != nil {
			_ = conn.DeleteTopics(topic)
			conn.Close()
		}
	}()
	if err := waitForTopicReady(testKafkaBroker, topic); err != nil {
		t.Fatalf("topic not ready: %v", err)
	}

	db := setupMongoDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, coll := range []string{"messages", "inboxes", "topic_seqs"} {
		_ = db.Collection(coll).Drop(ctx)
	}

	storage := msgworker.NewMessageStorage(db, testLogger)
	if err := storage.EnsureIndexes(ctx); err != nil {
		t.Fatalf("failed to ensure indexes: %v", err)
	}

	var processed int64
	handler := func(ctx context.Context, key, value []byte, headers map[string]string) error {
		atomic.AddInt64(&processed, 1)
		return nil
	}

	kafkaCfg := msgworker.KafkaConsumerConfig{
		Brokers:     []string{testKafkaBroker},
		Topic:       topic,
		GroupID:     "msgworker-parallel-group-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		StartOffset: kafka.FirstOffset,
		WorkerCount: 4,
	}

	consumer := msgworker.NewKafkaConsumer(kafkaCfg, handler, testLogger)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = consumer.Start(ctx)
	}()

	// Wait for consumer group to join.
	time.Sleep(1 * time.Second)

	producer := gateway.NewKafkaProducer(gateway.KafkaConfig{
		Brokers: []string{testKafkaBroker},
		Topic:   topic,
	}, testLogger)
	defer producer.Close()

	const messageCount = 20
	for i := 0; i < messageCount; i++ {
		upstream := &v1.UpstreamMessage{
			SenderId:    1,
			Topic:       "p2p_1_2",
			MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
			Content:     []byte("parallel test message"),
			ClientMsgId: fmt.Sprintf("parallel-msg-%d", i),
			Timestamp:   time.Now().UnixMilli(),
		}
		if err := producer.Produce(ctx, upstream); err != nil {
			t.Fatalf("failed to produce message %d: %v", i, err)
		}
	}

	// Wait for workers to process all messages.
	time.Sleep(3 * time.Second)

	cancel()
	wg.Wait()
	_ = consumer.Stop()

	if processed != messageCount {
		t.Fatalf("expected %d messages processed, got %d", messageCount, processed)
	}

	t.Logf("parallel kafka consumer verified: %d messages processed with %d workers", messageCount, kafkaCfg.WorkerCount)
}
