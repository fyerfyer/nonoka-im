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

// failingRouter is a GatewayRouter test double that always returns an error.
type failingRouter struct{}

func (f *failingRouter) ResolveUserNodes(ctx context.Context, userIDs []int64) (map[string][]int64, error) {
	return nil, fmt.Errorf("simulated redis failure")
}

func (f *failingRouter) ResolveUserNodesWithNodes(ctx context.Context, userIDs []int64, aliveNodes []*msgworker.GatewayNodeInfo) map[string][]int64 {
	return map[string][]int64{}
}

func (f *failingRouter) GetAliveNodes(ctx context.Context) ([]*msgworker.GatewayNodeInfo, error) {
	return nil, fmt.Errorf("simulated redis failure")
}

func (f *failingRouter) GetAliveNodesMap(ctx context.Context) (map[string]*msgworker.GatewayNodeInfo, error) {
	return nil, fmt.Errorf("simulated redis failure")
}

func (f *failingRouter) GetNodeGRPCAddr(ctx context.Context, nodeID string) string {
	return ""
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
		produceMessageWithRetry(ctx, t, producer, upstream)
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

// TestMsgWorker_DuplicateMessage_DoesNotAdvanceMaxSeq verifies that handling a
// duplicate message does not raise topic_seqs.max_seq, avoiding seq gaps.
func TestMsgWorker_DuplicateMessage_DoesNotAdvanceMaxSeq(t *testing.T) {
	worker, db, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	topic := "p2p_1_2"
	clientMsgID := "dup-seq-test-001"

	// First message: seq = 1.
	upstream1 := &v1.UpstreamMessage{
		SenderId:    1,
		Topic:       topic,
		MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
		Content:     []byte("first"),
		ClientMsgId: clientMsgID,
		Timestamp:   time.Now().UnixMilli(),
	}
	data1, _ := proto.Marshal(upstream1)
	if err := worker.HandleMessage(ctx, []byte(topic), data1, nil); err != nil {
		t.Fatalf("handle first message failed: %v", err)
	}

	seqColl := db.Collection(msgworker.CollectionTopicSeqs)
	var backup msgworker.TopicSeqBackup
	if err := seqColl.FindOne(ctx, bson.M{"topic": topic}).Decode(&backup); err != nil {
		t.Fatalf("find topic seq backup failed: %v", err)
	}
	if backup.MaxSeq != 1 {
		t.Fatalf("expected max_seq=1 after first message, got %d", backup.MaxSeq)
	}

	// Duplicate message: should be ignored and NOT advance max_seq.
	upstream2 := &v1.UpstreamMessage{
		SenderId:    1,
		Topic:       topic,
		MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
		Content:     []byte("duplicate"),
		ClientMsgId: clientMsgID,
		Timestamp:   time.Now().UnixMilli(),
	}
	data2, _ := proto.Marshal(upstream2)
	if err := worker.HandleMessage(ctx, []byte(topic), data2, nil); err != nil {
		t.Fatalf("handle duplicate message failed: %v", err)
	}

	if err := seqColl.FindOne(ctx, bson.M{"topic": topic}).Decode(&backup); err != nil {
		t.Fatalf("find topic seq backup after duplicate failed: %v", err)
	}
	if backup.MaxSeq != 1 {
		t.Fatalf("expected max_seq=1 after duplicate (not advanced), got %d", backup.MaxSeq)
	}

	t.Log("duplicate message did not advance topic_seqs.max_seq")
}

// TestStorage_GetOfflineMessages_GroupEnforcesLimit verifies that group offline
// messages are truncated to the requested limit after merging group messages and
// @mention messages.
func TestStorage_GetOfflineMessages_GroupEnforcesLimit(t *testing.T) {
	worker, _, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	userID := int64(200)
	mentionedUserID := int64(200)
	topic := "grp_limit_42"

	// Store 3 regular group messages.
	for i := 0; i < 3; i++ {
		upstream := &v1.UpstreamMessage{
			SenderId:    int64(100 + i),
			Topic:       topic,
			MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
			Content:     []byte(fmt.Sprintf("group msg %d", i+1)),
			ClientMsgId: fmt.Sprintf("limit-grp-%d", i),
			Timestamp:   time.Now().UnixMilli(),
		}
		data, _ := proto.Marshal(upstream)
		if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
			t.Fatalf("handle group message %d failed: %v", i, err)
		}
	}

	// Store 2 @mention messages for the same user.
	for i := 0; i < 2; i++ {
		upstream := &v1.UpstreamMessage{
			SenderId:         int64(100 + i),
			Topic:            topic,
			MsgType:          int32(v1.MsgType_MSG_TYPE_TEXT),
			Content:          []byte(fmt.Sprintf("mention msg %d", i+1)),
			ClientMsgId:      fmt.Sprintf("limit-mention-%d", i),
			Timestamp:        time.Now().UnixMilli(),
			MentionedUserIds: []int64{mentionedUserID},
		}
		data, _ := proto.Marshal(upstream)
		if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
			t.Fatalf("handle mention message %d failed: %v", i, err)
		}
	}

	// Request limit=3; merged result should be truncated to 3.
	msgs, err := worker.Storage().GetOfflineMessages(ctx, userID, topic, 0, 3)
	if err != nil {
		t.Fatalf("get offline messages failed: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages (limit enforced), got %d", len(msgs))
	}

	// Verify ascending order.
	for i := 1; i < len(msgs); i++ {
		if msgs[i].TopicSeq < msgs[i-1].TopicSeq {
			t.Fatalf("messages not sorted by seq: %v", msgs)
		}
	}

	t.Logf("group offline messages limit enforced: requested 3, got %d", len(msgs))
}

// TestGatewayPusher_RouterFallbackToStatic verifies that when the router fails
// (e.g., Redis unavailable), the pusher falls back to static gateway broadcast.
func TestGatewayPusher_RouterFallbackToStatic(t *testing.T) {
	// Static gateway: a recording pusher is not enough because we need real
	// gRPC connections. Use a real local gRPC push server.
	mgr := gateway.NewManager(testLogger)
	grpcAddr, grpcCleanup := setupGRPCPushServer(t, mgr)
	defer grpcCleanup()

	pusher, err := msgworker.NewGatewayPusher([]string{grpcAddr}, testLogger)
	if err != nil {
		t.Fatalf("failed to create gateway pusher: %v", err)
	}
	defer pusher.Close()

	// Attach a failing router: dynamic resolve should error and fallback.
	pusher.SetRouter(&failingRouter{})

	ctx := context.Background()
	msg := &pb.MessagePush{
		MsgId:    1,
		Topic:    "p2p_1_2",
		SenderId: 1,
		MsgType:  int32(v1.MsgType_MSG_TYPE_TEXT),
		Content:  []byte("fallback test"),
	}

	// Since the static gateway has no connections, the user is offline.
	// The important thing is that fallback was attempted and did not error.
	_, failedIDs, err := pusher.BatchPushToUsers(ctx, []int64{100}, msg)
	if err != nil {
		t.Fatalf("batch push with router fallback failed: %v", err)
	}
	if len(failedIDs) != 1 || failedIDs[0] != 100 {
		t.Fatalf("expected user 100 to be reported as failed (offline), got %v", failedIDs)
	}

	t.Log("gateway pusher correctly fell back to static gateways when router failed")
}
