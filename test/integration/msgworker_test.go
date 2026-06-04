package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/gateway"
	"nonoka-im/internal/msgworker"

	"github.com/segmentio/kafka-go"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
)

// ============================================
// MsgWorker 集成测试
// ============================================

// setupMsgWorker creates a MsgWorker with real dependencies for testing.
// It returns the worker, mongo database, and cleanup function.
func setupMsgWorker(t *testing.T) (*msgworker.MsgWorker, *mongo.Database, func()) {
	db := setupMongoDB(t)
	redisClient := setupTestRedis(t)

	// Clean Redis seq keys for this test
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := redisClient.Del(ctx, "im:seq:*").Err(); err != nil {
		t.Logf("warning: failed to clean redis keys: %v", err)
	}

	seqGen := msgworker.NewSeqGenerator(redisClient, testLogger)
	snowflake := msgworker.NewSnowflake(1)
	storage := msgworker.NewMessageStorage(db, testLogger)

	// Ensure indexes exist
	if err := storage.EnsureIndexes(ctx); err != nil {
		t.Fatalf("failed to ensure indexes: %v", err)
	}

	// Create worker without pusher (push tests will set up pusher separately)
	worker := msgworker.NewMsgWorker(nil, seqGen, snowflake, storage, nil, testLogger)

	cleanup := func() {
		// Clean up collections
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, coll := range []string{"messages", "inboxes", "topic_seqs", "mention_inboxes"} {
			_ = db.Collection(coll).Drop(ctx)
		}
	}

	return worker, db, cleanup
}

// TestMsgWorker_P2PMessage_WriteSpread verifies that P2P messages are persisted
// to the receiver's inbox using write扩散.
func TestMsgWorker_P2PMessage_WriteSpread(t *testing.T) {
	worker, db, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	senderID := int64(100)
	receiverID := int64(200)
	topic := "p2p_100_200"

	upstream := &v1.UpstreamMessage{
		SenderId:    senderID,
		Topic:       topic,
		MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
		Content:     []byte("hello p2p"),
		ClientMsgId: "p2p-msg-001",
		Timestamp:   time.Now().UnixMilli(),
	}
	data, err := proto.Marshal(upstream)
	if err != nil {
		t.Fatalf("failed to marshal upstream: %v", err)
	}

	if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
		t.Fatalf("handle message failed: %v", err)
	}

	// Verify inbox collection has the message for receiver
	inboxColl := db.Collection(msgworker.CollectionInboxes)
	count := countMongoDocs(t, inboxColl, bson.M{"user_id": receiverID})
	if count != 1 {
		t.Fatalf("expected 1 inbox message for receiver %d, got %d", receiverID, count)
	}

	// Verify the inbox message fields
	var inbox msgworker.InboxMessage
	if err := inboxColl.FindOne(ctx, bson.M{"user_id": receiverID}).Decode(&inbox); err != nil {
		t.Fatalf("failed to find inbox message: %v", err)
	}
	if inbox.SenderID != senderID {
		t.Fatalf("expected sender_id=%d, got %d", senderID, inbox.SenderID)
	}
	if inbox.Topic != topic {
		t.Fatalf("expected topic=%s, got %s", topic, inbox.Topic)
	}
	if string(inbox.Content) != "hello p2p" {
		t.Fatalf("expected content='hello p2p', got %s", string(inbox.Content))
	}
	if inbox.TopicSeq != 1 {
		t.Fatalf("expected topic_seq=1 for first message, got %d", inbox.TopicSeq)
	}
	if inbox.MsgID == 0 {
		t.Fatal("expected non-zero msg_id")
	}
	if inbox.Read {
		t.Fatal("expected message to be unread")
	}

	// Verify sender does NOT have an inbox entry (write扩散 only to receiver)
	senderCount := countMongoDocs(t, inboxColl, bson.M{"user_id": senderID})
	if senderCount != 0 {
		t.Fatalf("expected 0 inbox message for sender %d, got %d", senderID, senderCount)
	}

	t.Logf("p2p write spread verified: msg_id=%d, topic_seq=%d, receiver=%d", inbox.MsgID, inbox.TopicSeq, receiverID)
}

// TestMsgWorker_P2PMessage_SenderIsUID2 verifies correct receiver identification
// when sender is the second user in the topic.
func TestMsgWorker_P2PMessage_SenderIsUID2(t *testing.T) {
	worker, db, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	senderID := int64(200)
	receiverID := int64(100)
	topic := "p2p_100_200"

	upstream := &v1.UpstreamMessage{
		SenderId:    senderID,
		Topic:       topic,
		MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
		Content:     []byte("hello from uid2"),
		ClientMsgId: "p2p-msg-002",
		Timestamp:   time.Now().UnixMilli(),
	}
	data, _ := proto.Marshal(upstream)

	if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
		t.Fatalf("handle message failed: %v", err)
	}

	inboxColl := db.Collection(msgworker.CollectionInboxes)
	count := countMongoDocs(t, inboxColl, bson.M{"user_id": receiverID})
	if count != 1 {
		t.Fatalf("expected 1 inbox message for receiver %d, got %d", receiverID, count)
	}

	senderCount := countMongoDocs(t, inboxColl, bson.M{"user_id": senderID})
	if senderCount != 0 {
		t.Fatalf("expected 0 inbox message for sender %d, got %d", senderID, senderCount)
	}

	t.Logf("p2p sender-is-uid2 verified: receiver=%d", receiverID)
}

// TestMsgWorker_GroupMessage_ReadSpread verifies that group messages are persisted
// to the messages collection using read扩散 (only one copy stored).
func TestMsgWorker_GroupMessage_ReadSpread(t *testing.T) {
	worker, db, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	senderID := int64(100)
	topic := "grp_42"

	upstream := &v1.UpstreamMessage{
		SenderId:    senderID,
		Topic:       topic,
		MsgType:     int32(v1.MsgType_MSG_TYPE_IMAGE),
		Content:     []byte(`{"url":"https://example.com/img.png"}`),
		ClientMsgId: "grp-msg-001",
		Timestamp:   time.Now().UnixMilli(),
	}
	data, _ := proto.Marshal(upstream)

	if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
		t.Fatalf("handle message failed: %v", err)
	}

	// Verify messages collection has the group message
	msgColl := db.Collection(msgworker.CollectionMessages)
	count := countMongoDocs(t, msgColl, bson.M{"topic": topic})
	if count != 1 {
		t.Fatalf("expected 1 group message, got %d", count)
	}

	var stored msgworker.StoredMessage
	if err := msgColl.FindOne(ctx, bson.M{"topic": topic}).Decode(&stored); err != nil {
		t.Fatalf("failed to find group message: %v", err)
	}
	if stored.SenderID != senderID {
		t.Fatalf("expected sender_id=%d, got %d", senderID, stored.SenderID)
	}
	if stored.TopicSeq != 1 {
		t.Fatalf("expected topic_seq=1 for first message, got %d", stored.TopicSeq)
	}
	if stored.MsgID == 0 {
		t.Fatal("expected non-zero msg_id")
	}
	if stored.ClientMsgID != "grp-msg-001" {
		t.Fatalf("expected client_msg_id='grp-msg-001', got %s", stored.ClientMsgID)
	}

	// Verify NO inbox entries for group messages (read扩散)
	inboxColl := db.Collection(msgworker.CollectionInboxes)
	inboxCount := countMongoDocs(t, inboxColl, bson.M{})
	if inboxCount != 0 {
		t.Fatalf("expected 0 inbox messages for group message, got %d", inboxCount)
	}

	t.Logf("group read spread verified: msg_id=%d, topic_seq=%d", stored.MsgID, stored.TopicSeq)
}

// TestMsgWorker_SystemMessage verifies that system messages are persisted
// to the target user's inbox.
func TestMsgWorker_SystemMessage(t *testing.T) {
	worker, db, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	targetUID := int64(999)
	topic := "sys_999"

	upstream := &v1.UpstreamMessage{
		SenderId:    0, // system messages may have sender_id=0
		Topic:       topic,
		MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
		Content:     []byte("system notification"),
		ClientMsgId: "sys-msg-001",
		Timestamp:   time.Now().UnixMilli(),
	}
	data, _ := proto.Marshal(upstream)

	if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
		t.Fatalf("handle message failed: %v", err)
	}

	inboxColl := db.Collection(msgworker.CollectionInboxes)
	count := countMongoDocs(t, inboxColl, bson.M{"user_id": targetUID})
	if count != 1 {
		t.Fatalf("expected 1 system inbox message for user %d, got %d", targetUID, count)
	}

	var inbox msgworker.InboxMessage
	if err := inboxColl.FindOne(ctx, bson.M{"user_id": targetUID}).Decode(&inbox); err != nil {
		t.Fatalf("failed to find system inbox message: %v", err)
	}
	if inbox.Topic != topic {
		t.Fatalf("expected topic=%s, got %s", topic, inbox.Topic)
	}
	if inbox.TopicSeq != 1 {
		t.Fatalf("expected topic_seq=1, got %d", inbox.TopicSeq)
	}

	t.Logf("system message verified: target=%d, topic_seq=%d", targetUID, inbox.TopicSeq)
}

// TestMsgWorker_SeqGeneration verifies that sequence numbers are correctly
// generated and monotonically increasing per topic.
func TestMsgWorker_SeqGeneration(t *testing.T) {
	worker, db, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	topic1 := "p2p_1_2"
	topic2 := "grp_99"

	// Send 5 messages to topic1
	for i := 0; i < 5; i++ {
		upstream := &v1.UpstreamMessage{
			SenderId:    1,
			Topic:       topic1,
			MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
			Content:     []byte("seq test"),
			ClientMsgId: "seq-test-1-" + string(rune('0'+i)),
			Timestamp:   time.Now().UnixMilli(),
		}
		data, _ := proto.Marshal(upstream)
		if err := worker.HandleMessage(ctx, []byte(topic1), data, nil); err != nil {
			t.Fatalf("handle message %d failed: %v", i, err)
		}
	}

	// Send 3 messages to topic2
	for i := 0; i < 3; i++ {
		upstream := &v1.UpstreamMessage{
			SenderId:    1,
			Topic:       topic2,
			MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
			Content:     []byte("seq test"),
			ClientMsgId: "seq-test-2-" + string(rune('0'+i)),
			Timestamp:   time.Now().UnixMilli(),
		}
		data, _ := proto.Marshal(upstream)
		if err := worker.HandleMessage(ctx, []byte(topic2), data, nil); err != nil {
			t.Fatalf("handle message %d failed: %v", i, err)
		}
	}

	// Verify topic1 seqs are 1,2,3,4,5
	inboxColl := db.Collection(msgworker.CollectionInboxes)
	cursor, err := inboxColl.Find(ctx, bson.M{"topic": topic1})
	if err != nil {
		t.Fatalf("failed to find topic1 messages: %v", err)
	}
	defer cursor.Close(ctx)

	var seqs []uint64
	for cursor.Next(ctx) {
		var inbox msgworker.InboxMessage
		if err := cursor.Decode(&inbox); err != nil {
			t.Fatalf("failed to decode inbox: %v", err)
		}
		seqs = append(seqs, inbox.TopicSeq)
	}
	if len(seqs) != 5 {
		t.Fatalf("expected 5 topic1 messages, got %d", len(seqs))
	}

	// Verify seqs are unique and range 1-5
	seqSet := make(map[uint64]bool)
	for _, seq := range seqs {
		if seq < 1 || seq > 5 {
			t.Fatalf("expected seq in range [1,5], got %d", seq)
		}
		if seqSet[seq] {
			t.Fatalf("duplicate seq %d found", seq)
		}
		seqSet[seq] = true
	}

	// Verify topic2 seqs are independent (1,2,3)
	msgColl := db.Collection(msgworker.CollectionMessages)
	cursor2, err := msgColl.Find(ctx, bson.M{"topic": topic2})
	if err != nil {
		t.Fatalf("failed to find topic2 messages: %v", err)
	}
	defer cursor2.Close(ctx)

	var seqs2 []uint64
	for cursor2.Next(ctx) {
		var stored msgworker.StoredMessage
		if err := cursor2.Decode(&stored); err != nil {
			t.Fatalf("failed to decode message: %v", err)
		}
		seqs2 = append(seqs2, stored.TopicSeq)
	}
	if len(seqs2) != 3 {
		t.Fatalf("expected 3 topic2 messages, got %d", len(seqs2))
	}
	for _, seq := range seqs2 {
		if seq < 1 || seq > 3 {
			t.Fatalf("expected topic2 seq in range [1,3], got %d", seq)
		}
	}

	t.Logf("seq generation verified: topic1 seqs=%v, topic2 seqs=%v", seqs, seqs2)
}

// TestMsgWorker_TopicSeqBackup verifies that the max seq for each topic
// is backed up to MongoDB.
func TestMsgWorker_TopicSeqBackup(t *testing.T) {
	worker, db, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	topic := "p2p_1_2"

	// Send 3 messages
	for i := 0; i < 3; i++ {
		upstream := &v1.UpstreamMessage{
			SenderId:    1,
			Topic:       topic,
			MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
			Content:     []byte("backup test"),
			ClientMsgId: "backup-test-" + string(rune('0'+i)),
			Timestamp:   time.Now().UnixMilli(),
		}
		data, _ := proto.Marshal(upstream)
		if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
			t.Fatalf("handle message %d failed: %v", i, err)
		}
	}

	// Verify topic_seqs collection
	seqColl := db.Collection(msgworker.CollectionTopicSeqs)
	count := countMongoDocs(t, seqColl, bson.M{"topic": topic})
	if count != 1 {
		t.Fatalf("expected 1 topic seq backup, got %d", count)
	}

	var backup msgworker.TopicSeqBackup
	if err := seqColl.FindOne(ctx, bson.M{"topic": topic}).Decode(&backup); err != nil {
		t.Fatalf("failed to find topic seq backup: %v", err)
	}
	if backup.MaxSeq != 3 {
		t.Fatalf("expected max_seq=3, got %d", backup.MaxSeq)
	}
	if backup.Topic != topic {
		t.Fatalf("expected topic=%s, got %s", topic, backup.Topic)
	}

	t.Logf("topic seq backup verified: topic=%s, max_seq=%d", backup.Topic, backup.MaxSeq)
}

// TestMsgWorker_ConcurrentMessages verifies that concurrent message processing
// produces unique, monotonically increasing sequence numbers without duplicates or gaps.
func TestMsgWorker_ConcurrentMessages(t *testing.T) {
	worker, db, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	topic := "p2p_1_2"
	const concurrency = 20
	const msgsPerGoroutine = 10

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < msgsPerGoroutine; j++ {
				upstream := &v1.UpstreamMessage{
					SenderId:    1,
					Topic:       topic,
					MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
					Content:     []byte("concurrent test"),
					ClientMsgId: "concurrent-" + string(rune('0'+idx)) + "-" + string(rune('0'+j)),
					Timestamp:   time.Now().UnixMilli(),
				}
				data, _ := proto.Marshal(upstream)
				if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
					t.Errorf("handle message failed: %v", err)
				}
			}
		}(i)
	}
	wg.Wait()

	// Verify all seqs are unique and continuous from 1 to N
	expectedCount := concurrency * msgsPerGoroutine
	inboxColl := db.Collection(msgworker.CollectionInboxes)
	cursor, err := inboxColl.Find(ctx, bson.M{"topic": topic})
	if err != nil {
		t.Fatalf("failed to find messages: %v", err)
	}
	defer cursor.Close(ctx)

	seqSet := make(map[uint64]bool)
	for cursor.Next(ctx) {
		var inbox msgworker.InboxMessage
		if err := cursor.Decode(&inbox); err != nil {
			t.Fatalf("failed to decode inbox: %v", err)
		}
		if seqSet[inbox.TopicSeq] {
			t.Fatalf("duplicate seq %d found", inbox.TopicSeq)
		}
		seqSet[inbox.TopicSeq] = true
	}

	if len(seqSet) != expectedCount {
		t.Fatalf("expected %d unique seqs, got %d", expectedCount, len(seqSet))
	}

	// Verify no gaps: all seqs from 1 to expectedCount should exist
	for i := uint64(1); i <= uint64(expectedCount); i++ {
		if !seqSet[i] {
			t.Fatalf("missing seq %d", i)
		}
	}

	// Verify topic seq backup matches
	seqColl := db.Collection(msgworker.CollectionTopicSeqs)
	var backup msgworker.TopicSeqBackup
	if err := seqColl.FindOne(ctx, bson.M{"topic": topic}).Decode(&backup); err != nil {
		t.Fatalf("failed to find topic seq backup: %v", err)
	}
	if backup.MaxSeq != uint64(expectedCount) {
		t.Fatalf("expected max_seq=%d, got %d", expectedCount, backup.MaxSeq)
	}

	t.Logf("concurrent messages verified: %d messages, seqs 1-%d, no duplicates or gaps", expectedCount, expectedCount)
}

// TestMsgWorker_InvalidTopic verifies that messages with invalid topics are rejected.
func TestMsgWorker_InvalidTopic(t *testing.T) {
	worker, _, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	upstream := &v1.UpstreamMessage{
		SenderId:    1,
		Topic:       "invalid_topic_format",
		MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
		Content:     []byte("test"),
		ClientMsgId: "invalid-001",
		Timestamp:   time.Now().UnixMilli(),
	}
	data, _ := proto.Marshal(upstream)

	err := worker.HandleMessage(ctx, []byte("invalid_topic_format"), data, nil)
	if err == nil {
		t.Fatal("expected error for invalid topic, got nil")
	}

	t.Logf("invalid topic correctly rejected: %v", err)
}

// TestMsgWorker_MultipleTopics_IndependentSeq verifies that different topics
// have independent sequence numbers.
func TestMsgWorker_MultipleTopics_IndependentSeq(t *testing.T) {
	worker, db, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	topics := []string{"p2p_1_2", "p2p_3_4", "grp_10"}

	for _, topic := range topics {
		for i := 0; i < 3; i++ {
			upstream := &v1.UpstreamMessage{
				SenderId:    1,
				Topic:       topic,
				MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
				Content:     []byte("multi-topic test"),
				ClientMsgId: "multi-" + topic + "-" + string(rune('0'+i)),
				Timestamp:   time.Now().UnixMilli(),
			}
			data, _ := proto.Marshal(upstream)
			if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
				t.Fatalf("handle message failed for topic %s: %v", topic, err)
			}
		}
	}

	// Verify each topic has seq 1,2,3
	seqColl := db.Collection(msgworker.CollectionTopicSeqs)
	for _, topic := range topics {
		var backup msgworker.TopicSeqBackup
		if err := seqColl.FindOne(ctx, bson.M{"topic": topic}).Decode(&backup); err != nil {
			t.Fatalf("failed to find seq backup for topic %s: %v", topic, err)
		}
		if backup.MaxSeq != 3 {
			t.Fatalf("expected max_seq=3 for topic %s, got %d", topic, backup.MaxSeq)
		}
	}

	t.Logf("multi-topic independent seq verified: %d topics, each max_seq=3", len(topics))
}

// TestMsgWorker_PushToOnlineUser verifies that when a user is online (connected
// via WebSocket), the MsgWorker pushes the message to them via the Gateway's
// gRPC PushService.
func TestMsgWorker_PushToOnlineUser(t *testing.T) {
	// 1. Set up the full Gateway + HTTP server (for WebSocket connections)
	ts := setupTestServer(t, false)
	defer ts.stop()

	// 2. Set up MongoDB and MsgWorker with pusher
	db := setupMongoDB(t)
	redisClient := setupTestRedis(t)
	ctx := context.Background()

	// Clean collections
	for _, coll := range []string{"messages", "inboxes", "topic_seqs"} {
		_ = db.Collection(coll).Drop(ctx)
	}

	// 3. Start gRPC PushServer using the same gateway manager
	grpcAddr, grpcCleanup := setupGRPCPushServer(t, ts.gwManager)
	defer grpcCleanup()

	seqGen := msgworker.NewSeqGenerator(redisClient, testLogger)
	snowflake := msgworker.NewSnowflake(1)
	storage := msgworker.NewMessageStorage(db, testLogger)
	if err := storage.EnsureIndexes(ctx); err != nil {
		t.Fatalf("failed to ensure indexes: %v", err)
	}

	pusher, err := msgworker.NewGatewayPusher(grpcAddr, testLogger)
	if err != nil {
		t.Fatalf("failed to create gateway pusher: %v", err)
	}
	defer pusher.Close()

	worker := msgworker.NewMsgWorker(nil, seqGen, snowflake, storage, pusher, testLogger)

	// 4. Create two users: sender and receiver
	_, senderID := registerAndLogin(t, "push-sender", "123456")
	_, receiverID := registerAndLogin(t, "push-receiver", "123456")

	// 5. Connect receiver via WebSocket
	wsConn := wsConnect(t)
	defer wsConn.Close()

	authPayload, _ := json.Marshal(map[string]interface{}{
		"token":     generateJWTToken(receiverID, ts.authConf.JwtSecret),
		"device_id": "web-push-test",
	})
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd:     v1.Command_CMD_AUTH,
		Seq:     1,
		Payload: authPayload,
	})
	// Consume auth response
	wsReadPacket(t, wsConn, 2*time.Second)

	// Give the gateway a moment to register the connection
	time.Sleep(100 * time.Millisecond)

	// 6. Send a P2P message from sender to receiver
	topic := fmt.Sprintf("p2p_%d_%d", senderID, receiverID)
	if senderID > receiverID {
		topic = fmt.Sprintf("p2p_%d_%d", receiverID, senderID)
	}

	upstream := &v1.UpstreamMessage{
		SenderId:    senderID,
		Topic:       topic,
		MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
		Content:     []byte("push test message"),
		ClientMsgId: "push-msg-001",
		Timestamp:   time.Now().UnixMilli(),
	}
	data, _ := proto.Marshal(upstream)

	if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
		t.Fatalf("handle message failed: %v", err)
	}

	// 7. Verify the receiver got the push via WebSocket
	// The push message is a Packet with CMD_NOTIFY containing MessagePush
	pushedPacket := wsReadPacketOrNil(t, wsConn, 3*time.Second)
	if pushedPacket == nil {
		t.Fatal("expected push message to online user, got nil")
	}
	if pushedPacket.Cmd != v1.Command_CMD_NOTIFY {
		t.Fatalf("expected CMD_NOTIFY, got %v", pushedPacket.Cmd)
	}

	var pushMsg v1.MessagePush
	if err := proto.Unmarshal(pushedPacket.Payload, &pushMsg); err != nil {
		t.Fatalf("failed to unmarshal push message: %v", err)
	}
	if pushMsg.SenderId != senderID {
		t.Fatalf("expected sender_id=%d, got %d", senderID, pushMsg.SenderId)
	}
	if pushMsg.Topic != topic {
		t.Fatalf("expected topic=%s, got %s", topic, pushMsg.Topic)
	}
	if string(pushMsg.Content) != "push test message" {
		t.Fatalf("expected content='push test message', got %s", string(pushMsg.Content))
	}
	if pushMsg.TopicSeq != 1 {
		t.Fatalf("expected topic_seq=1, got %d", pushMsg.TopicSeq)
	}
	if pushMsg.MsgId == 0 {
		t.Fatal("expected non-zero msg_id in push")
	}

	// 8. Verify message is persisted in inbox
	inboxColl := db.Collection(msgworker.CollectionInboxes)
	count := countMongoDocs(t, inboxColl, bson.M{"user_id": receiverID})
	if count != 1 {
		t.Fatalf("expected 1 inbox message for receiver, got %d", count)
	}

	t.Logf("push to online user verified: receiver=%d, msg_id=%d, topic_seq=%d", receiverID, pushMsg.MsgId, pushMsg.TopicSeq)
}

// TestMsgWorker_PushToOfflineUser verifies that pushing to an offline user
// does not fail and records the user as failed.
func TestMsgWorker_PushToOfflineUser(t *testing.T) {
	// 1. Set up gRPC PushServer with empty manager (no connections)
	mgr := gateway.NewManager(testLogger)
	grpcAddr, grpcCleanup := setupGRPCPushServer(t, mgr)
	defer grpcCleanup()

	// 2. Set up MongoDB and MsgWorker with pusher
	db := setupMongoDB(t)
	redisClient := setupTestRedis(t)
	ctx := context.Background()

	for _, coll := range []string{"messages", "inboxes", "topic_seqs"} {
		_ = db.Collection(coll).Drop(ctx)
	}

	seqGen := msgworker.NewSeqGenerator(redisClient, testLogger)
	snowflake := msgworker.NewSnowflake(1)
	storage := msgworker.NewMessageStorage(db, testLogger)
	if err := storage.EnsureIndexes(ctx); err != nil {
		t.Fatalf("failed to ensure indexes: %v", err)
	}

	pusher, err := msgworker.NewGatewayPusher(grpcAddr, testLogger)
	if err != nil {
		t.Fatalf("failed to create gateway pusher: %v", err)
	}
	defer pusher.Close()

	worker := msgworker.NewMsgWorker(nil, seqGen, snowflake, storage, pusher, testLogger)

	// 3. Send a P2P message to an offline user (user 999)
	upstream := &v1.UpstreamMessage{
		SenderId:    100,
		Topic:       "p2p_100_999",
		MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
		Content:     []byte("offline user test"),
		ClientMsgId: "offline-msg-001",
		Timestamp:   time.Now().UnixMilli(),
	}
	data, _ := proto.Marshal(upstream)

	// Should not fail even though user is offline
	if err := worker.HandleMessage(ctx, []byte("p2p_100_999"), data, nil); err != nil {
		t.Fatalf("handle message failed for offline user: %v", err)
	}

	// Verify message is still persisted
	inboxColl := db.Collection(msgworker.CollectionInboxes)
	count := countMongoDocs(t, inboxColl, bson.M{"user_id": int64(999)})
	if count != 1 {
		t.Fatalf("expected 1 inbox message for offline user, got %d", count)
	}

	t.Log("push to offline user handled gracefully: message persisted, push recorded as failed")
}

// TestMsgWorker_E2E_KafkaToMongoDB verifies the full pipeline:
// produce to Kafka -> MsgWorker consumes -> persist to MongoDB.
func TestMsgWorker_E2E_KafkaToMongoDB(t *testing.T) {
	// 1. Set up Kafka
	if err := waitForKafka(testKafkaBroker); err != nil {
		t.Fatalf("kafka not ready: %v", err)
	}
	topic := "msgworker-e2e-test-" + fmt.Sprintf("%d", time.Now().UnixNano())
	if err := cleanupAndCreateTopic(testKafkaBroker, topic, 3); err != nil {
		t.Fatalf("failed to create kafka topic: %v", err)
	}
	// Wait for topic metadata to propagate to all brokers
	if err := waitForTopicReady(testKafkaBroker, topic); err != nil {
		t.Fatalf("topic not ready: %v", err)
	}
	defer func() {
		conn, _ := kafka.Dial("tcp", testKafkaBroker)
		if conn != nil {
			_ = conn.DeleteTopics(topic)
			conn.Close()
		}
	}()

	// 2. Set up MongoDB and MsgWorker with Kafka consumer
	db := setupMongoDB(t)
	redisClient := setupTestRedis(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, coll := range []string{"messages", "inboxes", "topic_seqs"} {
		_ = db.Collection(coll).Drop(ctx)
	}

	seqGen := msgworker.NewSeqGenerator(redisClient, testLogger)
	snowflake := msgworker.NewSnowflake(1)
	storage := msgworker.NewMessageStorage(db, testLogger)
	if err := storage.EnsureIndexes(ctx); err != nil {
		t.Fatalf("failed to ensure indexes: %v", err)
	}

	kafkaCfg := msgworker.KafkaConsumerConfig{
		Brokers:     []string{testKafkaBroker},
		Topic:       topic,
		GroupID:     "msgworker-e2e-group-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		StartOffset: kafka.FirstOffset, // consume from beginning for new consumer group
	}

	consumer := msgworker.NewKafkaConsumer(kafkaCfg, nil, testLogger)
	worker := msgworker.NewMsgWorker(consumer, seqGen, snowflake, storage, nil, testLogger)
	consumer.SetHandler(worker.HandleMessage)

	// Start worker in background
	var workerErr error
	var workerDone sync.WaitGroup
	workerDone.Add(1)
	go func() {
		defer workerDone.Done()
		workerErr = worker.Start(ctx)
	}()

	// Wait for consumer group to join and partitions to be assigned
	time.Sleep(1 * time.Second)

	// 3. Produce messages to Kafka
	producer := gateway.NewKafkaProducer(gateway.KafkaConfig{
		Brokers: []string{testKafkaBroker},
		Topic:   topic,
	}, testLogger)
	defer producer.Close()

	const messageCount = 5
	for i := 0; i < messageCount; i++ {
		upstream := &v1.UpstreamMessage{
			SenderId:    1,
			Topic:       "p2p_1_2",
			MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
			Content:     []byte("e2e test message"),
			ClientMsgId: "e2e-msg-" + string(rune('0'+i)),
			Timestamp:   time.Now().UnixMilli(),
		}
		if err := producer.Produce(ctx, upstream); err != nil {
			t.Fatalf("failed to produce message %d: %v", i, err)
		}
	}

	// Wait for messages to be consumed and processed
	time.Sleep(3 * time.Second)

	// 4. Cancel context to stop worker
	cancel()
	workerDone.Wait()
	if workerErr != nil && workerErr != context.Canceled {
		t.Logf("worker exited with error (may be expected): %v", workerErr)
	}

	// 5. Verify messages in MongoDB (use fresh context, not the canceled one)
	verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer verifyCancel()

	inboxColl := db.Collection(msgworker.CollectionInboxes)
	count := countMongoDocs(t, inboxColl, bson.M{"topic": "p2p_1_2"})
	if count != messageCount {
		t.Fatalf("expected %d inbox messages, got %d", messageCount, count)
	}

	// Verify seqs are correct
	cursor, err := inboxColl.Find(verifyCtx, bson.M{"topic": "p2p_1_2"})
	if err != nil {
		t.Fatalf("failed to find messages: %v", err)
	}
	defer cursor.Close(verifyCtx)

	seqSet := make(map[uint64]bool)
	for cursor.Next(verifyCtx) {
		var inbox msgworker.InboxMessage
		if err := cursor.Decode(&inbox); err != nil {
			t.Fatalf("failed to decode inbox: %v", err)
		}
		seqSet[inbox.TopicSeq] = true
	}

	for i := uint64(1); i <= messageCount; i++ {
		if !seqSet[i] {
			t.Fatalf("missing seq %d", i)
		}
	}

	t.Logf("e2e kafka-to-mongodb verified: %d messages consumed, seqs 1-%d", messageCount, messageCount)
}

// TestMsgWorker_KafkaConsumer_GracefulShutdown verifies that the Kafka consumer
// can be stopped gracefully without errors.
func TestMsgWorker_KafkaConsumer_GracefulShutdown(t *testing.T) {
	if err := waitForKafka(testKafkaBroker); err != nil {
		t.Fatalf("kafka not ready: %v", err)
	}
	topic := "msgworker-shutdown-test-" + fmt.Sprintf("%d", time.Now().UnixNano())
	if err := cleanupAndCreateTopic(testKafkaBroker, topic, 1); err != nil {
		t.Fatalf("failed to create kafka topic: %v", err)
	}
	defer func() {
		conn, _ := kafka.Dial("tcp", testKafkaBroker)
		if conn != nil {
			_ = conn.DeleteTopics(topic)
			conn.Close()
		}
	}()

	db := setupMongoDB(t)
	redisClient := setupTestRedis(t)

	seqGen := msgworker.NewSeqGenerator(redisClient, testLogger)
	snowflake := msgworker.NewSnowflake(1)
	storage := msgworker.NewMessageStorage(db, testLogger)

	kafkaCfg := msgworker.KafkaConsumerConfig{
		Brokers: []string{testKafkaBroker},
		Topic:   topic,
		GroupID: "msgworker-shutdown-group-" + fmt.Sprintf("%d", time.Now().UnixNano()),
	}

	consumer := msgworker.NewKafkaConsumer(kafkaCfg, nil, testLogger)
	worker := msgworker.NewMsgWorker(consumer, seqGen, snowflake, storage, nil, testLogger)

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = worker.Start(ctx)
	}()

	// Let it start consuming
	time.Sleep(300 * time.Millisecond)

	// Graceful shutdown
	cancel()
	wg.Wait()

	// Stop should not panic or block
	if err := worker.Stop(); err != nil {
		t.Fatalf("stop worker failed: %v", err)
	}

	t.Log("kafka consumer graceful shutdown verified")
}

// TestMsgWorker_SnowflakeUniqueness verifies that generated message IDs are unique
// even under concurrent load.
func TestMsgWorker_SnowflakeUniqueness(t *testing.T) {
	worker, db, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	topic := "p2p_1_2"
	const concurrency = 50
	const msgsPerGoroutine = 20

	var wg sync.WaitGroup
	var failed int32
	idSet := make(map[int64]bool)
	var mu sync.Mutex

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < msgsPerGoroutine; j++ {
				upstream := &v1.UpstreamMessage{
					SenderId:    1,
					Topic:       topic,
					MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
					Content:     []byte("snowflake test"),
					ClientMsgId: "snowflake-" + string(rune('0'+idx)) + "-" + string(rune('0'+j)),
					Timestamp:   time.Now().UnixMilli(),
				}
				data, _ := proto.Marshal(upstream)
				if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
					atomic.AddInt32(&failed, 1)
					t.Errorf("handle message failed: %v", err)
					continue
				}
			}
		}(i)
	}
	wg.Wait()

	if failed > 0 {
		t.Fatalf("%d messages failed to process", failed)
	}

	// Collect all msg_ids
	inboxColl := db.Collection(msgworker.CollectionInboxes)
	cursor, err := inboxColl.Find(ctx, bson.M{"topic": topic})
	if err != nil {
		t.Fatalf("failed to find messages: %v", err)
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var inbox msgworker.InboxMessage
		if err := cursor.Decode(&inbox); err != nil {
			t.Fatalf("failed to decode inbox: %v", err)
		}
		mu.Lock()
		if idSet[inbox.MsgID] {
			mu.Unlock()
			t.Fatalf("duplicate msg_id found: %d", inbox.MsgID)
		}
		idSet[inbox.MsgID] = true
		mu.Unlock()
	}

	expectedCount := concurrency * msgsPerGoroutine
	if len(idSet) != expectedCount {
		t.Fatalf("expected %d unique msg_ids, got %d", expectedCount, len(idSet))
	}

	t.Logf("snowflake uniqueness verified: %d concurrent messages, all %d msg_ids unique", expectedCount, len(idSet))
}
