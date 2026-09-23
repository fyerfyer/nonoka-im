package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/msgworker"
	"nonoka-im/pkg/sdk"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
)

// TestMsgWorker_SaveMentionInbox_PartialDuplicate verifies that when some
// @mention inbox entries already exist, the remaining entries are still inserted.
// This is a regression test for the unordered InsertMany bug.
func TestMsgWorker_SaveMentionInbox_PartialDuplicate(t *testing.T) {
	worker, db, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	senderID := int64(100)
	topic := "grp_partial_mention"

	upstream := &v1.UpstreamMessage{
		SenderId:         senderID,
		Topic:            topic,
		MsgType:          int32(v1.MsgType_MSG_TYPE_TEXT),
		Content:          []byte("hello @all"),
		ClientMsgId:      "partial-mention-001",
		Timestamp:        time.Now().UnixMilli(),
		MentionedUserIds: []int64{1, 2, 3},
	}
	data, _ := proto.Marshal(upstream)

	// First handle inserts all three mention inbox entries.
	if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
		t.Fatalf("first handle failed: %v", err)
	}

	mentionColl := db.Collection(msgworker.CollectionMentionInboxes)
	if count := countMongoDocs(t, mentionColl, bson.M{"topic": topic}); count != 3 {
		t.Fatalf("expected 3 mention inbox entries after first handle, got %d", count)
	}

	// Second handle with the same client_msg_id should be idempotent.
	// Before the fix, ordered InsertMany would stop at the first duplicate and
	// silently leave the remaining docs unverified.
	if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
		t.Fatalf("second handle failed: %v", err)
	}

	if count := countMongoDocs(t, mentionColl, bson.M{"topic": topic}); count != 3 {
		t.Fatalf("expected 3 mention inbox entries after duplicate handle, got %d", count)
	}
}

// TestMsgWorker_SaveP2PMessage_PartialDuplicate verifies that when the
// receiver's inbox entry already exists, the sender's inbox entry is still
// inserted on retry. This is a regression test for the SaveP2PMessage early
// return bug.
func TestMsgWorker_SaveP2PMessage_PartialDuplicate(t *testing.T) {
	worker, db, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	senderID := int64(100)
	receiverID := int64(200)
	topic := "p2p_100_200"
	clientMsgID := "partial-p2p-001"

	// Pre-insert the receiver's inbox entry to simulate a previous partial write.
	inboxColl := db.Collection(msgworker.CollectionInboxes)
	_, err := inboxColl.InsertOne(ctx, msgworker.InboxMessage{
		UserID:      receiverID,
		MsgID:       1,
		Topic:       topic,
		SenderID:    senderID,
		MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
		Content:     []byte("hello"),
		Timestamp:   time.Now().UnixMilli(),
		TopicSeq:    1,
		ClientMsgID: clientMsgID,
		Read:        false,
		CreatedAt:   time.Now(),
	})
	if err != nil {
		t.Fatalf("pre-insert receiver inbox failed: %v", err)
	}

	upstream := &v1.UpstreamMessage{
		SenderId:    senderID,
		Topic:       topic,
		MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
		Content:     []byte("hello"),
		ClientMsgId: clientMsgID,
		Timestamp:   time.Now().UnixMilli(),
	}
	data, _ := proto.Marshal(upstream)

	if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
		t.Fatalf("handle failed: %v", err)
	}

	// Sender's inbox entry should have been created even though receiver's was duplicate.
	if count := countMongoDocs(t, inboxColl, bson.M{"user_id": senderID, "topic": topic}); count != 1 {
		t.Fatalf("expected 1 sender inbox entry, got %d", count)
	}
	// Receiver's entry remains exactly one.
	if count := countMongoDocs(t, inboxColl, bson.M{"user_id": receiverID, "topic": topic}); count != 1 {
		t.Fatalf("expected 1 receiver inbox entry, got %d", count)
	}
}

// TestGateway_Kafka_Pull_NormalizesP2PTopic verifies that pulling with a non-normalized
// P2P topic (e.g., p2p_2_1) still finds messages stored under the canonical topic
// (e.g., p2p_1_2). This is a real end-to-end test: messages flow through the
// gateway, Kafka, and msgworker before being pulled via WebSocket.
func TestGateway_Kafka_Pull_NormalizesP2PTopic(t *testing.T) {
	ts := setupTestServer(t, true)
	defer ts.stop()

	ctx := context.Background()
	token1, user1 := registerAndLogin(t, "pull-norm-1", "123456")
	token2, user2 := registerAndLogin(t, "pull-norm-2", "123456")

	// Build canonical and non-canonical topics. The canonical form is
	// p2p_<min(user1,user2)>_<max(user1,user2)>.
	minID, maxID := user1, user2
	if user2 < user1 {
		minID, maxID = user2, user1
	}
	canonicalTopic := fmt.Sprintf("p2p_%d_%d", minID, maxID)
	nonCanonicalTopic := fmt.Sprintf("p2p_%d_%d", maxID, minID)

	// Connect sender and publish messages while the receiver is offline.
	sender := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token1,
		DeviceID:          "web-pull-norm-sender",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second,
	})
	defer sender.Close()

	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	if err := sender.Connect(connectCtx); err != nil {
		t.Fatalf("sender connect failed: %v", err)
	}
	cancel()

	content := []byte("real e2e normalized pull")
	for i := 0; i < 3; i++ {
		sendCtx, sendCancel := context.WithTimeout(ctx, 5*time.Second)
		_, err := sender.SendMessage(sendCtx, canonicalTopic, v1.MsgType_MSG_TYPE_TEXT, content)
		sendCancel()
		if err != nil {
			t.Fatalf("send message %d failed: %v", i, err)
		}
	}

	// Wait until all three messages have been persisted to the receiver's inbox.
	inboxColl := ts.mongoDB.Collection(msgworker.CollectionInboxes)
	deadline := time.Now().Add(10 * time.Second)
	for {
		count, err := inboxColl.CountDocuments(ctx, bson.M{"user_id": user2, "topic": canonicalTopic})
		if err != nil {
			t.Fatalf("count inbox failed: %v", err)
		}
		if count == 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for receiver inbox, got %d messages", count)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Now connect the receiver and pull using the non-canonical topic.
	receiver := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token2,
		DeviceID:          "web-pull-norm-receiver",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second,
	})
	defer receiver.Close()

	connectCtx, cancel = context.WithTimeout(ctx, 5*time.Second)
	if err := receiver.Connect(connectCtx); err != nil {
		t.Fatalf("receiver connect failed: %v", err)
	}
	cancel()

	pullCtx, pullCancel := context.WithTimeout(ctx, 5*time.Second)
	result, err := receiver.PullMessages(pullCtx, nonCanonicalTopic, 0, 50)
	pullCancel()
	if err != nil {
		t.Fatalf("pull messages failed: %v", err)
	}
	if len(result.Messages) != 3 {
		t.Fatalf("expected 3 messages after normalizing topic, got %d", len(result.Messages))
	}
	for i, m := range result.Messages {
		if string(m.Content) != string(content) {
			t.Fatalf("message %d content mismatch: got %q, want %q", i, string(m.Content), string(content))
		}
	}
}
