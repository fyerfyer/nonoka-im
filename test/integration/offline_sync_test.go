package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/gateway"
	"nonoka-im/internal/msgworker"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
)

// ============================================
// 阶段五：离线拉取与状态同步 集成测试
// ============================================

// ---------- Storage 层直接测试 ----------

// TestStorage_GetOfflineMessages_P2P verifies that P2P messages can be
// retrieved via GetOfflineMessages with correct seq ordering.
func TestStorage_GetOfflineMessages_P2P(t *testing.T) {
	worker, _, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	senderID := int64(100)
	receiverID := int64(200)
	topic := "p2p_100_200"

	// Store 5 P2P messages directly through the worker
	for i := 0; i < 5; i++ {
		upstream := &v1.UpstreamMessage{
			SenderId:    senderID,
			Topic:       topic,
			MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
			Content:     []byte(fmt.Sprintf("message %d", i+1)),
			ClientMsgId: fmt.Sprintf("p2p-offline-%d", i),
			Timestamp:   time.Now().UnixMilli(),
		}
		data, _ := proto.Marshal(upstream)
		if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
			t.Fatalf("handle message %d failed: %v", i, err)
		}
	}

	// Pull offline messages for receiver with lastSeq=0
	msgs, err := worker.Storage().GetOfflineMessages(ctx, receiverID, topic, 0, 50)
	if err != nil {
		t.Fatalf("get offline messages failed: %v", err)
	}
	if len(msgs) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(msgs))
	}

	// Verify ascending seq order and content
	for i, m := range msgs {
		if m.TopicSeq != uint64(i+1) {
			t.Fatalf("expected seq=%d at index %d, got %d", i+1, i, m.TopicSeq)
		}
		if m.SenderID != senderID {
			t.Fatalf("expected sender_id=%d, got %d", senderID, m.SenderID)
		}
		expectedContent := fmt.Sprintf("message %d", i+1)
		if string(m.Content) != expectedContent {
			t.Fatalf("expected content='%s', got '%s'", expectedContent, string(m.Content))
		}
		if m.Read {
			t.Fatalf("expected message to be unread")
		}
	}

	// Pull with lastSeq=3 should return only messages 4 and 5
	msgsPartial, err := worker.Storage().GetOfflineMessages(ctx, receiverID, topic, 3, 50)
	if err != nil {
		t.Fatalf("get partial offline messages failed: %v", err)
	}
	if len(msgsPartial) != 2 {
		t.Fatalf("expected 2 messages with lastSeq=3, got %d", len(msgsPartial))
	}
	if msgsPartial[0].TopicSeq != 4 {
		t.Fatalf("expected first partial seq=4, got %d", msgsPartial[0].TopicSeq)
	}
	if msgsPartial[1].TopicSeq != 5 {
		t.Fatalf("expected second partial seq=5, got %d", msgsPartial[1].TopicSeq)
	}

	t.Logf("p2p offline messages verified: total=5, partial pull returned 2")
}

// TestStorage_GetOfflineMessages_System verifies system message offline retrieval.
func TestStorage_GetOfflineMessages_System(t *testing.T) {
	worker, _, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	targetUID := int64(999)
	topic := "sys_999"

	// Store 3 system messages
	for i := 0; i < 3; i++ {
		upstream := &v1.UpstreamMessage{
			SenderId:    0,
			Topic:       topic,
			MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
			Content:     []byte(fmt.Sprintf("system notification %d", i+1)),
			ClientMsgId: fmt.Sprintf("sys-offline-%d", i),
			Timestamp:   time.Now().UnixMilli(),
		}
		data, _ := proto.Marshal(upstream)
		if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
			t.Fatalf("handle message %d failed: %v", i, err)
		}
	}

	msgs, err := worker.Storage().GetOfflineMessages(ctx, targetUID, topic, 0, 50)
	if err != nil {
		t.Fatalf("get offline messages failed: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 system messages, got %d", len(msgs))
	}
	for i, m := range msgs {
		if m.TopicSeq != uint64(i+1) {
			t.Fatalf("expected seq=%d, got %d", i+1, m.TopicSeq)
		}
	}

	t.Logf("system offline messages verified: count=%d", len(msgs))
}

// TestStorage_GetGroupMessages verifies that group messages can be retrieved
// via GetGroupMessages (read扩散 model).
func TestStorage_GetGroupMessages(t *testing.T) {
	worker, _, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	topic := "grp_42"

	// Store 5 group messages
	for i := 0; i < 5; i++ {
		upstream := &v1.UpstreamMessage{
			SenderId:    int64(100 + i),
			Topic:       topic,
			MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
			Content:     []byte(fmt.Sprintf("group message %d", i+1)),
			ClientMsgId: fmt.Sprintf("grp-offline-%d", i),
			Timestamp:   time.Now().UnixMilli(),
		}
		data, _ := proto.Marshal(upstream)
		if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
			t.Fatalf("handle message %d failed: %v", i, err)
		}
	}

	// Pull group messages with lastSeq=0
	msgs, err := worker.Storage().GetGroupMessages(ctx, topic, 0, 50)
	if err != nil {
		t.Fatalf("get group messages failed: %v", err)
	}
	if len(msgs) != 5 {
		t.Fatalf("expected 5 group messages, got %d", len(msgs))
	}

	// Verify ascending seq order
	for i, m := range msgs {
		if m.TopicSeq != uint64(i+1) {
			t.Fatalf("expected seq=%d at index %d, got %d", i+1, i, m.TopicSeq)
		}
	}

	// Pull with lastSeq=2 should return messages 3,4,5
	msgsPartial, err := worker.Storage().GetGroupMessages(ctx, topic, 2, 50)
	if err != nil {
		t.Fatalf("get partial group messages failed: %v", err)
	}
	if len(msgsPartial) != 3 {
		t.Fatalf("expected 3 messages with lastSeq=2, got %d", len(msgsPartial))
	}

	t.Logf("group offline messages verified: total=5, partial pull returned 3")
}

// TestStorage_GetMentionMessages verifies that @mention messages are stored
// and retrievable separately from regular group messages.
func TestStorage_GetMentionMessages(t *testing.T) {
	worker, db, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	senderID := int64(100)
	mentionedUID := int64(200)
	unmentionedUID := int64(300)
	topic := "grp_99"

	// Store a group message with @mentions
	upstream := &v1.UpstreamMessage{
		SenderId:         senderID,
		Topic:            topic,
		MsgType:          int32(v1.MsgType_MSG_TYPE_TEXT),
		Content:          []byte("@user200 hello!"),
		ClientMsgId:      "mention-msg-001",
		Timestamp:        time.Now().UnixMilli(),
		MentionedUserIds: []int64{mentionedUID},
	}
	data, _ := proto.Marshal(upstream)
	if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
		t.Fatalf("handle message failed: %v", err)
	}

	// Verify the mentioned user can retrieve mention messages
	mentionMsgs, err := worker.Storage().GetMentionMessages(ctx, mentionedUID, topic, 0, 50)
	if err != nil {
		t.Fatalf("get mention messages failed: %v", err)
	}
	if len(mentionMsgs) != 1 {
		t.Fatalf("expected 1 mention message for user %d, got %d", mentionedUID, len(mentionMsgs))
	}
	if mentionMsgs[0].SenderID != senderID {
		t.Fatalf("expected sender_id=%d, got %d", senderID, mentionMsgs[0].SenderID)
	}
	if string(mentionMsgs[0].Content) != "@user200 hello!" {
		t.Fatalf("unexpected content: %s", string(mentionMsgs[0].Content))
	}
	if mentionMsgs[0].Read {
		t.Fatal("expected mention message to be unread")
	}

	// Verify unmentioned user gets no mention messages
	unmentionedMsgs, err := worker.Storage().GetMentionMessages(ctx, unmentionedUID, topic, 0, 50)
	if err != nil {
		t.Fatalf("get mention messages for unmentioned user failed: %v", err)
	}
	if len(unmentionedMsgs) != 0 {
		t.Fatalf("expected 0 mention messages for unmentioned user, got %d", len(unmentionedMsgs))
	}

	// Verify the group message itself is also in messages collection (read扩散)
	msgColl := db.Collection(msgworker.CollectionMessages)
	count := countMongoDocs(t, msgColl, bson.M{"topic": topic})
	if count != 1 {
		t.Fatalf("expected 1 group message in messages collection, got %d", count)
	}

	t.Logf("mention messages verified: mentioned user got 1, unmentioned got 0")
}

// TestStorage_GetTopicMaxSeq verifies that the topic max seq is correctly
// backed up and retrievable.
func TestStorage_GetTopicMaxSeq(t *testing.T) {
	worker, _, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	topic := "p2p_1_2"

	// Before any messages, max seq should be 0
	maxSeq, err := worker.Storage().GetTopicMaxSeq(ctx, topic)
	if err != nil {
		t.Fatalf("get topic max seq failed: %v", err)
	}
	if maxSeq != 0 {
		t.Fatalf("expected max_seq=0 for empty topic, got %d", maxSeq)
	}

	// Store 5 messages
	for i := 0; i < 5; i++ {
		upstream := &v1.UpstreamMessage{
			SenderId:    1,
			Topic:       topic,
			MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
			Content:     []byte("seq test"),
			ClientMsgId: fmt.Sprintf("seq-test-%d", i),
			Timestamp:   time.Now().UnixMilli(),
		}
		data, _ := proto.Marshal(upstream)
		if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
			t.Fatalf("handle message %d failed: %v", i, err)
		}
	}

	// Max seq should now be 5
	maxSeq, err = worker.Storage().GetTopicMaxSeq(ctx, topic)
	if err != nil {
		t.Fatalf("get topic max seq after messages failed: %v", err)
	}
	if maxSeq != 5 {
		t.Fatalf("expected max_seq=5, got %d", maxSeq)
	}

	t.Logf("topic max seq verified: initial=0, after 5 messages=%d", maxSeq)
}

// TestStorage_OfflineMessages_Pagination verifies pagination with limit.
func TestStorage_OfflineMessages_Pagination(t *testing.T) {
	worker, _, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	receiverID := int64(200)
	topic := "p2p_100_200"

	// Store 10 P2P messages
	for i := 0; i < 10; i++ {
		upstream := &v1.UpstreamMessage{
			SenderId:    100,
			Topic:       topic,
			MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
			Content:     []byte(fmt.Sprintf("page %d", i+1)),
			ClientMsgId: fmt.Sprintf("page-%d", i),
			Timestamp:   time.Now().UnixMilli(),
		}
		data, _ := proto.Marshal(upstream)
		if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
			t.Fatalf("handle message %d failed: %v", i, err)
		}
	}

	// Page 1: limit=3, lastSeq=0
	msgs1, err := worker.Storage().GetOfflineMessages(ctx, receiverID, topic, 0, 3)
	if err != nil {
		t.Fatalf("get page 1 failed: %v", err)
	}
	if len(msgs1) != 3 {
		t.Fatalf("expected 3 messages on page 1, got %d", len(msgs1))
	}
	if msgs1[0].TopicSeq != 1 || msgs1[2].TopicSeq != 3 {
		t.Fatalf("page 1 seq mismatch: got %v", []uint64{msgs1[0].TopicSeq, msgs1[2].TopicSeq})
	}

	// Page 2: limit=3, lastSeq=3
	msgs2, err := worker.Storage().GetOfflineMessages(ctx, receiverID, topic, 3, 3)
	if err != nil {
		t.Fatalf("get page 2 failed: %v", err)
	}
	if len(msgs2) != 3 {
		t.Fatalf("expected 3 messages on page 2, got %d", len(msgs2))
	}
	if msgs2[0].TopicSeq != 4 || msgs2[2].TopicSeq != 6 {
		t.Fatalf("page 2 seq mismatch: got %v", []uint64{msgs2[0].TopicSeq, msgs2[2].TopicSeq})
	}

	// Page 3: limit=3, lastSeq=6
	msgs3, err := worker.Storage().GetOfflineMessages(ctx, receiverID, topic, 6, 3)
	if err != nil {
		t.Fatalf("get page 3 failed: %v", err)
	}
	if len(msgs3) != 3 {
		t.Fatalf("expected 3 messages on page 3, got %d", len(msgs3))
	}
	if msgs3[0].TopicSeq != 7 || msgs3[2].TopicSeq != 9 {
		t.Fatalf("page 3 seq mismatch: got %v", []uint64{msgs3[0].TopicSeq, msgs3[2].TopicSeq})
	}

	// Page 4: limit=3, lastSeq=9 -> should get only 1 message
	msgs4, err := worker.Storage().GetOfflineMessages(ctx, receiverID, topic, 9, 3)
	if err != nil {
		t.Fatalf("get page 4 failed: %v", err)
	}
	if len(msgs4) != 1 {
		t.Fatalf("expected 1 message on page 4, got %d", len(msgs4))
	}
	if msgs4[0].TopicSeq != 10 {
		t.Fatalf("expected page 4 seq=10, got %d", msgs4[0].TopicSeq)
	}

	// Page 5: limit=3, lastSeq=10 -> empty
	msgs5, err := worker.Storage().GetOfflineMessages(ctx, receiverID, topic, 10, 3)
	if err != nil {
		t.Fatalf("get page 5 failed: %v", err)
	}
	if len(msgs5) != 0 {
		t.Fatalf("expected 0 messages on page 5, got %d", len(msgs5))
	}

	t.Logf("pagination verified: 10 messages across 5 pages")
}

// TestStorage_OfflineMessages_EmptyResult verifies that querying with lastSeq
// >= maxSeq returns an empty result without error.
func TestStorage_OfflineMessages_EmptyResult(t *testing.T) {
	worker, _, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	receiverID := int64(200)
	topic := "p2p_100_200"

	// Store 2 messages
	for i := 0; i < 2; i++ {
		upstream := &v1.UpstreamMessage{
			SenderId:    100,
			Topic:       topic,
			MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
			Content:     []byte("empty test"),
			ClientMsgId: fmt.Sprintf("empty-%d", i),
			Timestamp:   time.Now().UnixMilli(),
		}
		data, _ := proto.Marshal(upstream)
		if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
			t.Fatalf("handle message %d failed: %v", i, err)
		}
	}

	// Query with lastSeq=2 (equal to max) -> empty
	msgs, err := worker.Storage().GetOfflineMessages(ctx, receiverID, topic, 2, 50)
	if err != nil {
		t.Fatalf("get offline messages failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages when lastSeq==maxSeq, got %d", len(msgs))
	}

	// Query with lastSeq=5 (greater than max) -> empty
	msgs, err = worker.Storage().GetOfflineMessages(ctx, receiverID, topic, 5, 50)
	if err != nil {
		t.Fatalf("get offline messages failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages when lastSeq>maxSeq, got %d", len(msgs))
	}

	// Same for group messages
	grpMsgs, err := worker.Storage().GetGroupMessages(ctx, topic, 2, 50)
	if err != nil {
		t.Fatalf("get group messages failed: %v", err)
	}
	if len(grpMsgs) != 0 {
		t.Fatalf("expected 0 group messages when lastSeq==maxSeq, got %d", len(grpMsgs))
	}

	t.Log("empty result verified: no messages returned when lastSeq >= maxSeq")
}

// TestStorage_OfflineMessages_ConcurrentReadWrite verifies that concurrent
// writes and reads maintain consistency without race conditions.
func TestStorage_OfflineMessages_ConcurrentReadWrite(t *testing.T) {
	worker, _, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	receiverID := int64(200)
	topic := "p2p_100_200"
	const writeCount = 50
	const readerCount = 10

	var writeWg sync.WaitGroup
	// Concurrent writes
	for i := 0; i < writeCount; i++ {
		writeWg.Add(1)
		go func(idx int) {
			defer writeWg.Done()
			upstream := &v1.UpstreamMessage{
				SenderId:    100,
				Topic:       topic,
				MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
				Content:     []byte(fmt.Sprintf("concurrent %d", idx)),
				ClientMsgId: fmt.Sprintf("concurrent-%d", idx),
				Timestamp:   time.Now().UnixMilli(),
			}
			data, _ := proto.Marshal(upstream)
			if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
				t.Errorf("handle message %d failed: %v", idx, err)
			}
		}(i)
	}

	// Concurrent reads during writes
	var readWg sync.WaitGroup
	var readCount int64
	var readMu sync.Mutex
	for i := 0; i < readerCount; i++ {
		readWg.Add(1)
		go func() {
			defer readWg.Done()
			for j := 0; j < 10; j++ {
				msgs, err := worker.Storage().GetOfflineMessages(ctx, receiverID, topic, 0, 100)
				if err != nil {
					t.Errorf("concurrent read failed: %v", err)
					return
				}
				readMu.Lock()
				readCount += int64(len(msgs))
				readMu.Unlock()
				time.Sleep(10 * time.Millisecond)
			}
		}()
	}

	writeWg.Wait()
	readWg.Wait()

	// Final verification: all 50 messages should be present
	finalMsgs, err := worker.Storage().GetOfflineMessages(ctx, receiverID, topic, 0, 100)
	if err != nil {
		t.Fatalf("final read failed: %v", err)
	}
	if len(finalMsgs) != writeCount {
		t.Fatalf("expected %d messages after all writes, got %d", writeCount, len(finalMsgs))
	}

	// Verify all seqs are unique and continuous
	seqSet := make(map[uint64]bool)
	for _, m := range finalMsgs {
		if seqSet[m.TopicSeq] {
			t.Fatalf("duplicate seq %d found", m.TopicSeq)
		}
		seqSet[m.TopicSeq] = true
	}
	for i := uint64(1); i <= writeCount; i++ {
		if !seqSet[i] {
			t.Fatalf("missing seq %d", i)
		}
	}

	t.Logf("concurrent read/write verified: %d writes, seqs 1-%d all unique", writeCount, writeCount)
}

// TestStorage_MentionInbox_Isolation verifies that mention_inboxes is
// isolated from regular inboxes and messages collections.
func TestStorage_MentionInbox_Isolation(t *testing.T) {
	worker, db, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	senderID := int64(100)
	mentionedUID := int64(200)
	topic := "grp_77"

	// Send a group message with @mention
	upstream := &v1.UpstreamMessage{
		SenderId:         senderID,
		Topic:            topic,
		MsgType:          int32(v1.MsgType_MSG_TYPE_TEXT),
		Content:          []byte("@user200 check this"),
		ClientMsgId:      "isolation-001",
		Timestamp:        time.Now().UnixMilli(),
		MentionedUserIds: []int64{mentionedUID},
	}
	data, _ := proto.Marshal(upstream)
	if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
		t.Fatalf("handle message failed: %v", err)
	}

	// Verify: mention_inboxes has 1 record
	mentionColl := db.Collection(msgworker.CollectionMentionInboxes)
	mentionCount := countMongoDocs(t, mentionColl, bson.M{"user_id": mentionedUID})
	if mentionCount != 1 {
		t.Fatalf("expected 1 mention inbox record, got %d", mentionCount)
	}

	// Verify: regular inboxes has 0 records for this user/topic
	inboxColl := db.Collection(msgworker.CollectionInboxes)
	inboxCount := countMongoDocs(t, inboxColl, bson.M{"user_id": mentionedUID, "topic": topic})
	if inboxCount != 0 {
		t.Fatalf("expected 0 regular inbox record for mentioned user in group, got %d", inboxCount)
	}

	// Verify: messages collection has 1 record
	msgColl := db.Collection(msgworker.CollectionMessages)
	msgCount := countMongoDocs(t, msgColl, bson.M{"topic": topic})
	if msgCount != 1 {
		t.Fatalf("expected 1 message in messages collection, got %d", msgCount)
	}

	// Verify: GetOfflineMessages returns nothing for group topic
	offlineMsgs, err := worker.Storage().GetOfflineMessages(ctx, mentionedUID, topic, 0, 50)
	if err != nil {
		t.Fatalf("get offline messages failed: %v", err)
	}
	if len(offlineMsgs) != 0 {
		t.Fatalf("expected 0 offline messages for group topic, got %d", len(offlineMsgs))
	}

	// Verify: GetMentionMessages returns the mention
	mentionMsgs, err := worker.Storage().GetMentionMessages(ctx, mentionedUID, topic, 0, 50)
	if err != nil {
		t.Fatalf("get mention messages failed: %v", err)
	}
	if len(mentionMsgs) != 1 {
		t.Fatalf("expected 1 mention message, got %d", len(mentionMsgs))
	}

	// Verify: GetGroupMessages returns the group message
	groupMsgs, err := worker.Storage().GetGroupMessages(ctx, topic, 0, 50)
	if err != nil {
		t.Fatalf("get group messages failed: %v", err)
	}
	if len(groupMsgs) != 1 {
		t.Fatalf("expected 1 group message, got %d", len(groupMsgs))
	}

	t.Log("mention inbox isolation verified: 3 collections, correct separation")
}

// ---------- Gateway E2E 测试 ----------

// TestGateway_Pull_P2POfflineMessages verifies the full E2E flow:
// user A sends messages -> user B connects -> user B pulls offline messages.
func TestGateway_Pull_P2POfflineMessages(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	ctx := context.Background()

	// Create sender and receiver users
	_, senderID := registerAndLogin(t, "pull-p2p-sender", "123456")
	_, receiverID := registerAndLogin(t, "pull-p2p-receiver", "123456")

	// Build P2P topic
	topic := fmt.Sprintf("p2p_%d_%d", senderID, receiverID)
	if senderID > receiverID {
		topic = fmt.Sprintf("p2p_%d_%d", receiverID, senderID)
	}

	// Send 3 P2P messages directly via storage (simulate offline period)
	for i := 0; i < 3; i++ {
		msgID := int64(1000 + i)
		seq := uint64(i + 1)
		inbox := &msgworker.InboxMessage{
			UserID:    receiverID,
			MsgID:     msgID,
			Topic:     topic,
			SenderID:  senderID,
			MsgType:   int32(v1.MsgType_MSG_TYPE_TEXT),
			Content:   []byte(fmt.Sprintf("offline p2p %d", i+1)),
			Timestamp: time.Now().UnixMilli(),
			TopicSeq:  seq,
			Read:      false,
			CreatedAt: time.Now(),
		}
		_, err := ts.mongoDB.Collection(msgworker.CollectionInboxes).InsertOne(ctx, inbox)
		if err != nil {
			t.Fatalf("insert inbox message %d failed: %v", i, err)
		}
		// Also update topic seq backup
		_ = ts.storage.BackupTopicSeq(ctx, topic, seq)
	}

	// Receiver connects via WebSocket
	wsConn := wsConnect(t)
	defer wsConn.Close()

	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd: v1.Command_CMD_AUTH,
		Seq: 1,
		Payload: &v1.Packet_AuthReq{
			AuthReq: &v1.AuthRequest{
				Token:    generateJWTToken(receiverID, ts.authConf.JwtSecret),
				DeviceId: "web-pull-test",
			},
		},
	})
	wsReadPacket(t, wsConn, 2*time.Second) // consume auth response

	// Send CMD_PULL request
	pullReq := &v1.PullRequest{
		Topic:   topic,
		LastSeq: 0,
		Limit:   50,
	}
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd:     v1.Command_CMD_PULL,
		Seq:     2,
		Payload: &v1.Packet_PullReq{PullReq: pullReq},
	})

	// Read pull response
	resp := wsReadPacket(t, wsConn, 3*time.Second)
	if resp.Cmd != v1.Command_CMD_PULL {
		t.Fatalf("expected CMD_PULL response, got %v", resp.Cmd)
	}
	if resp.Seq != 2 {
		t.Fatalf("expected seq=2, got %d", resp.Seq)
	}

	pullReply := resp.GetPullReply()
	if pullReply == nil {
		t.Fatalf("expected PullReply payload, got nil")
	}

	if len(pullReply.Messages) != 3 {
		t.Fatalf("expected 3 messages in pull reply, got %d", len(pullReply.Messages))
	}
	if pullReply.HasMore {
		t.Fatal("expected has_more=false")
	}
	if pullReply.NextSeq != 4 {
		t.Fatalf("expected next_seq=6, got %d", pullReply.NextSeq)
	}

	// Verify message content and order
	for i, m := range pullReply.Messages {
		if m.TopicSeq != uint64(i+1) {
			t.Fatalf("expected seq=%d at index %d, got %d", i+1, i, m.TopicSeq)
		}
		if m.SenderId != senderID {
			t.Fatalf("expected sender_id=%d, got %d", senderID, m.SenderId)
		}
		expectedContent := fmt.Sprintf("offline p2p %d", i+1)
		if string(m.Content) != expectedContent {
			t.Fatalf("expected content='%s', got '%s'", expectedContent, string(m.Content))
		}
	}

	t.Logf("gateway pull p2p verified: %d messages pulled, next_seq=%d", len(pullReply.Messages), pullReply.NextSeq)
}

// TestGateway_Pull_GroupOfflineMessages verifies group message pull via WebSocket.
func TestGateway_Pull_GroupOfflineMessages(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	ctx := context.Background()
	_, userID := registerAndLogin(t, "pull-group-user", "123456")

	topic := "grp_test_42"

	// Store 3 group messages
	for i := 0; i < 3; i++ {
		msgID := int64(2000 + i)
		seq := uint64(i + 1)
		stored := &msgworker.StoredMessage{
			MsgID:       msgID,
			Topic:       topic,
			SenderID:    int64(100 + i),
			MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
			Content:     []byte(fmt.Sprintf("group msg %d", i+1)),
			Timestamp:   time.Now().UnixMilli(),
			TopicSeq:    seq,
			ClientMsgID: fmt.Sprintf("grp-pull-%d", i),
			CreatedAt:   time.Now(),
		}
		_, err := ts.mongoDB.Collection(msgworker.CollectionMessages).InsertOne(ctx, stored)
		if err != nil {
			t.Fatalf("insert group message %d failed: %v", i, err)
		}
		_ = ts.storage.BackupTopicSeq(ctx, topic, seq)
	}

	// User connects via WebSocket
	wsConn := wsConnect(t)
	defer wsConn.Close()

	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd: v1.Command_CMD_AUTH,
		Seq: 1,
		Payload: &v1.Packet_AuthReq{
			AuthReq: &v1.AuthRequest{
				Token:    generateJWTToken(userID, ts.authConf.JwtSecret),
				DeviceId: "web-grp-pull",
			},
		},
	})
	wsReadPacket(t, wsConn, 2*time.Second)

	// Pull group messages
	pullReq := &v1.PullRequest{
		Topic:   topic,
		LastSeq: 0,
		Limit:   50,
	}
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd:     v1.Command_CMD_PULL,
		Seq:     2,
		Payload: &v1.Packet_PullReq{PullReq: pullReq},
	})

	resp := wsReadPacket(t, wsConn, 3*time.Second)
	if resp.Cmd != v1.Command_CMD_PULL {
		t.Fatalf("expected CMD_PULL response, got %v", resp.Cmd)
	}

	pullReply := resp.GetPullReply()
	if pullReply == nil {
		t.Fatalf("expected PullReply payload, got nil")
	}

	if len(pullReply.Messages) != 3 {
		t.Fatalf("expected 3 group messages, got %d", len(pullReply.Messages))
	}

	for i, m := range pullReply.Messages {
		if m.TopicSeq != uint64(i+1) {
			t.Fatalf("expected seq=%d at index %d, got %d", i+1, i, m.TopicSeq)
		}
		expectedContent := fmt.Sprintf("group msg %d", i+1)
		if string(m.Content) != expectedContent {
			t.Fatalf("expected content='%s', got '%s'", expectedContent, string(m.Content))
		}
	}

	t.Logf("gateway pull group verified: %d messages", len(pullReply.Messages))
}

// TestGateway_Pull_MentionMessages verifies that @mention messages are included
// in the pull reply for group topics.
func TestGateway_Pull_MentionMessages(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	ctx := context.Background()
	_, mentionedUserID := registerAndLogin(t, "pull-mention-user", "123456")
	senderID := int64(555)

	topic := "grp_mention_99"

	// Store a regular group message
	stored := &msgworker.StoredMessage{
		MsgID:       3000,
		Topic:       topic,
		SenderID:    senderID,
		MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
		Content:     []byte("regular group message"),
		Timestamp:   time.Now().UnixMilli(),
		TopicSeq:    1,
		ClientMsgID: "mention-regular-001",
		CreatedAt:   time.Now(),
	}
	_, err := ts.mongoDB.Collection(msgworker.CollectionMessages).InsertOne(ctx, stored)
	if err != nil {
		t.Fatalf("insert regular group message failed: %v", err)
	}
	_ = ts.storage.BackupTopicSeq(ctx, topic, 1)

	// Store an @mention message for the user
	mention := &msgworker.MentionMessage{
		UserID:    mentionedUserID,
		MsgID:     3001,
		Topic:     topic,
		SenderID:  senderID,
		MsgType:   int32(v1.MsgType_MSG_TYPE_TEXT),
		Content:   []byte("@user200 you are mentioned!"),
		Timestamp: time.Now().UnixMilli(),
		TopicSeq:  2,
		Read:      false,
		CreatedAt: time.Now(),
	}
	_, err = ts.mongoDB.Collection(msgworker.CollectionMentionInboxes).InsertOne(ctx, mention)
	if err != nil {
		t.Fatalf("insert mention message failed: %v", err)
	}
	_ = ts.storage.BackupTopicSeq(ctx, topic, 2)

	// Also store the mention as a regular group message (seq 2)
	stored2 := &msgworker.StoredMessage{
		MsgID:       3001,
		Topic:       topic,
		SenderID:    senderID,
		MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
		Content:     []byte("@user200 you are mentioned!"),
		Timestamp:   time.Now().UnixMilli(),
		TopicSeq:    2,
		ClientMsgID: "mention-002",
		CreatedAt:   time.Now(),
	}
	_, err = ts.mongoDB.Collection(msgworker.CollectionMessages).InsertOne(ctx, stored2)
	if err != nil {
		t.Fatalf("insert group message seq 2 failed: %v", err)
	}

	// User connects and pulls
	wsConn := wsConnect(t)
	defer wsConn.Close()

	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd: v1.Command_CMD_AUTH,
		Seq: 1,
		Payload: &v1.Packet_AuthReq{
			AuthReq: &v1.AuthRequest{
				Token:    generateJWTToken(mentionedUserID, ts.authConf.JwtSecret),
				DeviceId: "web-mention-pull",
			},
		},
	})
	wsReadPacket(t, wsConn, 2*time.Second)

	// Pull group messages (should include both regular and mention)
	pullReq := &v1.PullRequest{
		Topic:   topic,
		LastSeq: 0,
		Limit:   50,
	}
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd:     v1.Command_CMD_PULL,
		Seq:     2,
		Payload: &v1.Packet_PullReq{PullReq: pullReq},
	})

	resp := wsReadPacket(t, wsConn, 3*time.Second)
	if resp.Cmd != v1.Command_CMD_PULL {
		t.Fatalf("expected CMD_PULL response, got %v", resp.Cmd)
	}

	pullReply := resp.GetPullReply()
	if pullReply == nil {
		t.Fatalf("expected PullReply payload, got nil")
	}

	// Should have 3 messages: regular (seq1), mention (seq2), group copy of mention (seq2)
	if len(pullReply.Messages) != 3 {
		t.Fatalf("expected 3 messages (2 group + 1 mention), got %d", len(pullReply.Messages))
	}

	// Verify seqs
	seqs := make(map[uint64]int)
	for _, m := range pullReply.Messages {
		seqs[m.TopicSeq]++
	}
	if seqs[1] != 1 {
		t.Fatalf("expected 1 message with seq=1, got %d", seqs[1])
	}
	if seqs[2] != 2 {
		t.Fatalf("expected 2 messages with seq=2 (group + mention), got %d", seqs[2])
	}

	t.Logf("gateway pull mention verified: %d messages including @mention", len(pullReply.Messages))
}

// TestGateway_Pull_Pagination verifies pagination via WebSocket pull.
func TestGateway_Pull_Pagination(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	ctx := context.Background()
	_, receiverID := registerAndLogin(t, "pull-page-user", "123456")

	topic := "p2p_1_2"

	// Store 5 P2P messages
	for i := 0; i < 5; i++ {
		inbox := &msgworker.InboxMessage{
			UserID:    receiverID,
			MsgID:     int64(4000 + i),
			Topic:     topic,
			SenderID:  1,
			MsgType:   int32(v1.MsgType_MSG_TYPE_TEXT),
			Content:   []byte(fmt.Sprintf("page msg %d", i+1)),
			Timestamp: time.Now().UnixMilli(),
			TopicSeq:  uint64(i + 1),
			Read:      false,
			CreatedAt: time.Now(),
		}
		_, err := ts.mongoDB.Collection(msgworker.CollectionInboxes).InsertOne(ctx, inbox)
		if err != nil {
			t.Fatalf("insert inbox message %d failed: %v", i, err)
		}
		_ = ts.storage.BackupTopicSeq(ctx, topic, uint64(i+1))
	}

	wsConn := wsConnect(t)
	defer wsConn.Close()

	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd: v1.Command_CMD_AUTH,
		Seq: 1,
		Payload: &v1.Packet_AuthReq{
			AuthReq: &v1.AuthRequest{
				Token:    generateJWTToken(receiverID, ts.authConf.JwtSecret),
				DeviceId: "web-page-pull",
			},
		},
	})
	wsReadPacket(t, wsConn, 2*time.Second)

	// Page 1: limit=2, lastSeq=0
	pullReq := &v1.PullRequest{
		Topic:   topic,
		LastSeq: 0,
		Limit:   2,
	}
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd:     v1.Command_CMD_PULL,
		Seq:     2,
		Payload: &v1.Packet_PullReq{PullReq: pullReq},
	})

	resp1 := wsReadPacket(t, wsConn, 3*time.Second)
	reply1 := resp1.GetPullReply()
	if reply1 == nil {
		t.Fatalf("expected PullReply payload, got nil")
	}

	if len(reply1.Messages) != 2 {
		t.Fatalf("expected 2 messages on page 1, got %d", len(reply1.Messages))
	}
	if !reply1.HasMore {
		t.Fatal("expected has_more=true on page 1")
	}
	if reply1.NextSeq != 3 {
		t.Fatalf("expected next_seq=3, got %d", reply1.NextSeq)
	}

	// Page 2: limit=2, lastSeq=2
	pullReq2 := &v1.PullRequest{
		Topic:   topic,
		LastSeq: 2,
		Limit:   2,
	}
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd:     v1.Command_CMD_PULL,
		Seq:     3,
		Payload: &v1.Packet_PullReq{PullReq: pullReq2},
	})

	resp2 := wsReadPacket(t, wsConn, 3*time.Second)
	reply2 := resp2.GetPullReply()
	if reply2 == nil {
		t.Fatalf("expected PullReply payload, got nil")
	}

	if len(reply2.Messages) != 2 {
		t.Fatalf("expected 2 messages on page 2, got %d", len(reply2.Messages))
	}
	if !reply2.HasMore {
		t.Fatal("expected has_more=true on page 2")
	}
	if reply2.NextSeq != 5 {
		t.Fatalf("expected next_seq=6, got %d", reply2.NextSeq)
	}

	// Page 3: limit=2, lastSeq=4
	pullReq3 := &v1.PullRequest{
		Topic:   topic,
		LastSeq: 4,
		Limit:   2,
	}
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd:     v1.Command_CMD_PULL,
		Seq:     4,
		Payload: &v1.Packet_PullReq{PullReq: pullReq3},
	})

	resp3 := wsReadPacket(t, wsConn, 3*time.Second)
	reply3 := resp3.GetPullReply()
	if reply3 == nil {
		t.Fatalf("expected PullReply payload, got nil")
	}

	if len(reply3.Messages) != 1 {
		t.Fatalf("expected 1 message on page 3, got %d", len(reply3.Messages))
	}
	if reply3.HasMore {
		t.Fatal("expected has_more=false on page 3")
	}
	if reply3.NextSeq != 6 {
		t.Fatalf("expected next_seq=6, got %d", reply3.NextSeq)
	}

	t.Logf("gateway pull pagination verified: 5 messages in 3 pages")
}

// TestGateway_Pull_EmptyResult verifies that pulling with no new messages
// returns an empty but valid reply.
func TestGateway_Pull_EmptyResult(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	_, userID := registerAndLogin(t, "pull-empty-user", "123456")

	wsConn := wsConnect(t)
	defer wsConn.Close()

	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd: v1.Command_CMD_AUTH,
		Seq: 1,
		Payload: &v1.Packet_AuthReq{
			AuthReq: &v1.AuthRequest{
				Token:    generateJWTToken(userID, ts.authConf.JwtSecret),
				DeviceId: "web-empty-pull",
			},
		},
	})
	wsReadPacket(t, wsConn, 2*time.Second)

	// Pull a topic with no messages
	pullReq := &v1.PullRequest{
		Topic:   "p2p_999_1000",
		LastSeq: 0,
		Limit:   50,
	}
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd:     v1.Command_CMD_PULL,
		Seq:     2,
		Payload: &v1.Packet_PullReq{PullReq: pullReq},
	})

	resp := wsReadPacket(t, wsConn, 3*time.Second)
	if resp.Cmd != v1.Command_CMD_PULL {
		t.Fatalf("expected CMD_PULL response, got %v", resp.Cmd)
	}

	pullReply := resp.GetPullReply()
	if pullReply == nil {
		t.Fatalf("expected PullReply payload, got nil")
	}

	if len(pullReply.Messages) != 0 {
		t.Fatalf("expected 0 messages for empty topic, got %d", len(pullReply.Messages))
	}
	if pullReply.HasMore {
		t.Fatal("expected has_more=false for empty result")
	}
	if pullReply.NextSeq != 0 {
		t.Fatalf("expected next_seq=0 for empty result, got %d", pullReply.NextSeq)
	}

	t.Log("gateway pull empty result verified: 0 messages, has_more=false")
}

// ---------- MsgWorker @提醒 测试 ----------

// TestMsgWorker_GroupMention_SaveToMentionInbox verifies that group messages
// with @mentions are saved to both messages collection and mention_inboxes.
func TestMsgWorker_GroupMention_SaveToMentionInbox(t *testing.T) {
	worker, db, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	senderID := int64(100)
	mentionedUID1 := int64(200)
	mentionedUID2 := int64(300)
	topic := "grp_mention_test"

	// Send a group message with two @mentions
	upstream := &v1.UpstreamMessage{
		SenderId:         senderID,
		Topic:            topic,
		MsgType:          int32(v1.MsgType_MSG_TYPE_TEXT),
		Content:          []byte("@user200 @user300 meeting now!"),
		ClientMsgId:      "mention-save-001",
		Timestamp:        time.Now().UnixMilli(),
		MentionedUserIds: []int64{mentionedUID1, mentionedUID2},
	}
	data, _ := proto.Marshal(upstream)
	if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
		t.Fatalf("handle message failed: %v", err)
	}

	// Verify messages collection has 1 record (read扩散)
	msgColl := db.Collection(msgworker.CollectionMessages)
	msgCount := countMongoDocs(t, msgColl, bson.M{"topic": topic})
	if msgCount != 1 {
		t.Fatalf("expected 1 group message, got %d", msgCount)
	}

	// Verify mention_inboxes has 2 records (write扩散 for @mentions)
	mentionColl := db.Collection(msgworker.CollectionMentionInboxes)
	mentionCount1 := countMongoDocs(t, mentionColl, bson.M{"user_id": mentionedUID1, "topic": topic})
	if mentionCount1 != 1 {
		t.Fatalf("expected 1 mention inbox for user %d, got %d", mentionedUID1, mentionCount1)
	}
	mentionCount2 := countMongoDocs(t, mentionColl, bson.M{"user_id": mentionedUID2, "topic": topic})
	if mentionCount2 != 1 {
		t.Fatalf("expected 1 mention inbox for user %d, got %d", mentionedUID2, mentionCount2)
	}

	// Verify mention records have correct fields
	var mention1 msgworker.MentionMessage
	if err := mentionColl.FindOne(ctx, bson.M{"user_id": mentionedUID1}).Decode(&mention1); err != nil {
		t.Fatalf("failed to find mention for user %d: %v", mentionedUID1, err)
	}
	if mention1.SenderID != senderID {
		t.Fatalf("expected sender_id=%d, got %d", senderID, mention1.SenderID)
	}
	if mention1.TopicSeq != 1 {
		t.Fatalf("expected topic_seq=1, got %d", mention1.TopicSeq)
	}
	if string(mention1.Content) != "@user200 @user300 meeting now!" {
		t.Fatalf("unexpected content: %s", string(mention1.Content))
	}

	t.Logf("mention save verified: 1 group msg, 2 mention inboxes")
}

// TestMsgWorker_GroupMention_PushToOnlineUser verifies that @mention
// notifications are pushed to online users via Gateway's gRPC PushService.
func TestMsgWorker_GroupMention_PushToOnlineUser(t *testing.T) {
	// 1. Set up full Gateway + HTTP server
	ts := setupTestServer(t, false)
	defer ts.stop()

	// 2. Set up MsgWorker with pusher
	db := ts.mongoDB
	redisClient := ts.redis
	ctx := context.Background()

	for _, coll := range []string{"messages", "inboxes", "topic_seqs", "mention_inboxes"} {
		_ = db.Collection(coll).Drop(ctx)
	}

	// 3. Start gRPC PushServer
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

	// 4. Create sender and mentioned user
	_, senderID := registerAndLogin(t, "mention-sender", "123456")
	_, mentionedID := registerAndLogin(t, "mention-receiver", "123456")

	// 5. Connect mentioned user via WebSocket
	wsConn := wsConnect(t)
	defer wsConn.Close()

	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd: v1.Command_CMD_AUTH,
		Seq: 1,
		Payload: &v1.Packet_AuthReq{
			AuthReq: &v1.AuthRequest{
				Token:    generateJWTToken(mentionedID, ts.authConf.JwtSecret),
				DeviceId: "web-mention-push",
			},
		},
	})
	wsReadPacket(t, wsConn, 2*time.Second)
	time.Sleep(100 * time.Millisecond)

	// 6. Send a group message with @mention
	topic := "grp_push_mention"
	upstream := &v1.UpstreamMessage{
		SenderId:         senderID,
		Topic:            topic,
		MsgType:          int32(v1.MsgType_MSG_TYPE_TEXT),
		Content:          []byte("@user200 urgent!"),
		ClientMsgId:      "mention-push-001",
		Timestamp:        time.Now().UnixMilli(),
		MentionedUserIds: []int64{mentionedID},
	}
	data, _ := proto.Marshal(upstream)

	if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
		t.Fatalf("handle message failed: %v", err)
	}

	// 7. Verify the mentioned user got the push via WebSocket
	pushedPacket := wsReadPacketOrNil(t, wsConn, 3*time.Second)
	if pushedPacket == nil {
		t.Fatal("expected push message for @mention to online user, got nil")
	}
	if pushedPacket.Cmd != v1.Command_CMD_NOTIFY {
		t.Fatalf("expected CMD_NOTIFY, got %v", pushedPacket.Cmd)
	}

	pushMsg := pushedPacket.GetNotify()
	if pushMsg == nil {
		t.Fatalf("expected Notify payload, got nil")
	}
	if pushMsg.SenderId != senderID {
		t.Fatalf("expected sender_id=%d, got %d", senderID, pushMsg.SenderId)
	}
	if pushMsg.Topic != topic {
		t.Fatalf("expected topic=%s, got %s", topic, pushMsg.Topic)
	}
	if string(pushMsg.Content) != "@user200 urgent!" {
		t.Fatalf("expected content='@user200 urgent!', got '%s'", string(pushMsg.Content))
	}
	if pushMsg.TopicSeq != 1 {
		t.Fatalf("expected topic_seq=1, got %d", pushMsg.TopicSeq)
	}

	// 8. Verify mention_inboxes has the record
	mentionColl := db.Collection(msgworker.CollectionMentionInboxes)
	mentionCount := countMongoDocs(t, mentionColl, bson.M{"user_id": mentionedID, "topic": topic})
	if mentionCount != 1 {
		t.Fatalf("expected 1 mention inbox record, got %d", mentionCount)
	}

	t.Logf("mention push to online user verified: receiver=%d, msg_id=%d", mentionedID, pushMsg.MsgId)
}

// TestMsgWorker_GroupMention_PushToOfflineUser verifies that @mention
// notifications for offline users are stored but push fails gracefully.
func TestMsgWorker_GroupMention_PushToOfflineUser(t *testing.T) {
	// 1. Set up gRPC PushServer with empty manager
	mgr := gateway.NewManager(testLogger)
	grpcAddr, grpcCleanup := setupGRPCPushServer(t, mgr)
	defer grpcCleanup()

	// 2. Set up MsgWorker with pusher
	db := setupMongoDB(t)
	redisClient := setupTestRedis(t)
	ctx := context.Background()

	for _, coll := range []string{"messages", "inboxes", "topic_seqs", "mention_inboxes"} {
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

	// 3. Send a group message with @mention to an offline user
	senderID := int64(100)
	offlineUID := int64(999)
	topic := "grp_offline_mention"

	upstream := &v1.UpstreamMessage{
		SenderId:         senderID,
		Topic:            topic,
		MsgType:          int32(v1.MsgType_MSG_TYPE_TEXT),
		Content:          []byte("@user999 hello offline!"),
		ClientMsgId:      "mention-offline-001",
		Timestamp:        time.Now().UnixMilli(),
		MentionedUserIds: []int64{offlineUID},
	}
	data, _ := proto.Marshal(upstream)

	// Should not fail even though user is offline
	if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
		t.Fatalf("handle message failed for offline mention: %v", err)
	}

	// Verify mention_inboxes has the record
	mentionColl := db.Collection(msgworker.CollectionMentionInboxes)
	mentionCount := countMongoDocs(t, mentionColl, bson.M{"user_id": offlineUID, "topic": topic})
	if mentionCount != 1 {
		t.Fatalf("expected 1 mention inbox for offline user, got %d", mentionCount)
	}

	// Verify messages collection also has the group message
	msgColl := db.Collection(msgworker.CollectionMessages)
	msgCount := countMongoDocs(t, msgColl, bson.M{"topic": topic})
	if msgCount != 1 {
		t.Fatalf("expected 1 group message, got %d", msgCount)
	}

	t.Log("mention to offline user verified: mention inbox saved, push handled gracefully")
}

// TestMsgWorker_GroupMention_MultipleUsers verifies @mention handling
// for multiple mentioned users in a single group message.
func TestMsgWorker_GroupMention_MultipleUsers(t *testing.T) {
	worker, db, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	senderID := int64(100)
	mentionedUsers := []int64{201, 202, 203, 204, 205}
	topic := "grp_multi_mention"

	upstream := &v1.UpstreamMessage{
		SenderId:         senderID,
		Topic:            topic,
		MsgType:          int32(v1.MsgType_MSG_TYPE_TEXT),
		Content:          []byte("@all meeting now!"),
		ClientMsgId:      "multi-mention-001",
		Timestamp:        time.Now().UnixMilli(),
		MentionedUserIds: mentionedUsers,
	}
	data, _ := proto.Marshal(upstream)
	if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
		t.Fatalf("handle message failed: %v", err)
	}

	// Verify each mentioned user has exactly 1 mention inbox record
	mentionColl := db.Collection(msgworker.CollectionMentionInboxes)
	for _, uid := range mentionedUsers {
		count := countMongoDocs(t, mentionColl, bson.M{"user_id": uid, "topic": topic})
		if count != 1 {
			t.Fatalf("expected 1 mention inbox for user %d, got %d", uid, count)
		}
	}

	// Verify a non-mentioned user has no records
	nonMentionedCount := countMongoDocs(t, mentionColl, bson.M{"user_id": int64(999), "topic": topic})
	if nonMentionedCount != 0 {
		t.Fatalf("expected 0 mention inbox for non-mentioned user, got %d", nonMentionedCount)
	}

	// Verify total mention records = 5
	totalCount := countMongoDocs(t, mentionColl, bson.M{"topic": topic})
	if totalCount != int64(len(mentionedUsers)) {
		t.Fatalf("expected %d total mention records, got %d", len(mentionedUsers), totalCount)
	}

	t.Logf("multi-user mention verified: %d users mentioned, %d records created", len(mentionedUsers), totalCount)
}

// ---------- 综合 E2E 测试 ----------

// TestOfflineSync_FullScenario simulates a realistic scenario:
// 1. User sends P2P messages while receiver is offline
// 2. Receiver reconnects and pulls offline messages
// 3. Receiver also pulls group messages and @mentions
// 4. Verifies seq continuity and state consistency
func TestOfflineSync_FullScenario(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	ctx := context.Background()
	_, senderID := registerAndLogin(t, "full-sender", "123456")
	_, receiverID := registerAndLogin(t, "full-receiver", "123456")

	p2pTopic := fmt.Sprintf("p2p_%d_%d", senderID, receiverID)
	if senderID > receiverID {
		p2pTopic = fmt.Sprintf("p2p_%d_%d", receiverID, senderID)
	}
	grpTopic := "grp_full_scenario"

	// Phase 1: Store offline messages while receiver is "offline"
	// 3 P2P messages
	for i := 0; i < 3; i++ {
		inbox := &msgworker.InboxMessage{
			UserID:    receiverID,
			MsgID:     int64(5000 + i),
			Topic:     p2pTopic,
			SenderID:  senderID,
			MsgType:   int32(v1.MsgType_MSG_TYPE_TEXT),
			Content:   []byte(fmt.Sprintf("offline p2p %d", i+1)),
			Timestamp: time.Now().UnixMilli(),
			TopicSeq:  uint64(i + 1),
			Read:      false,
			CreatedAt: time.Now(),
		}
		_, err := ts.mongoDB.Collection(msgworker.CollectionInboxes).InsertOne(ctx, inbox)
		if err != nil {
			t.Fatalf("insert p2p inbox %d failed: %v", i, err)
		}
		_ = ts.storage.BackupTopicSeq(ctx, p2pTopic, uint64(i+1))
	}

	// 2 group messages
	for i := 0; i < 2; i++ {
		stored := &msgworker.StoredMessage{
			MsgID:       int64(5100 + i),
			Topic:       grpTopic,
			SenderID:    senderID,
			MsgType:     int32(v1.MsgType_MSG_TYPE_TEXT),
			Content:     []byte(fmt.Sprintf("group msg %d", i+1)),
			Timestamp:   time.Now().UnixMilli(),
			TopicSeq:    uint64(i + 1),
			ClientMsgID: fmt.Sprintf("grp-full-%d", i),
			CreatedAt:   time.Now(),
		}
		_, err := ts.mongoDB.Collection(msgworker.CollectionMessages).InsertOne(ctx, stored)
		if err != nil {
			t.Fatalf("insert group message %d failed: %v", i, err)
		}
		_ = ts.storage.BackupTopicSeq(ctx, grpTopic, uint64(i+1))
	}

	// 1 @mention for receiver in the group
	mention := &msgworker.MentionMessage{
		UserID:    receiverID,
		MsgID:     5102,
		Topic:     grpTopic,
		SenderID:  senderID,
		MsgType:   int32(v1.MsgType_MSG_TYPE_TEXT),
		Content:   []byte("@receiver important!"),
		Timestamp: time.Now().UnixMilli(),
		TopicSeq:  3,
		Read:      false,
		CreatedAt: time.Now(),
	}
	_, err := ts.mongoDB.Collection(msgworker.CollectionMentionInboxes).InsertOne(ctx, mention)
	if err != nil {
		t.Fatalf("insert mention failed: %v", err)
	}
	_ = ts.storage.BackupTopicSeq(ctx, grpTopic, 3)

	// Phase 2: Receiver reconnects via WebSocket
	wsConn := wsConnect(t)
	defer wsConn.Close()

	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd: v1.Command_CMD_AUTH,
		Seq: 1,
		Payload: &v1.Packet_AuthReq{
			AuthReq: &v1.AuthRequest{
				Token:    generateJWTToken(receiverID, ts.authConf.JwtSecret),
				DeviceId: "web-full-sync",
			},
		},
	})
	wsReadPacket(t, wsConn, 2*time.Second)

	// Phase 3: Pull P2P messages
	pullReqP2P := &v1.PullRequest{
		Topic:   p2pTopic,
		LastSeq: 0,
		Limit:   50,
	}
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd:     v1.Command_CMD_PULL,
		Seq:     2,
		Payload: &v1.Packet_PullReq{PullReq: pullReqP2P},
	})

	respP2P := wsReadPacket(t, wsConn, 3*time.Second)
	replyP2P := respP2P.GetPullReply()
	if replyP2P == nil {
		t.Fatalf("expected PullReply payload, got nil")
	}

	if len(replyP2P.Messages) != 3 {
		t.Fatalf("expected 3 P2P messages, got %d", len(replyP2P.Messages))
	}
	if replyP2P.NextSeq != 4 {
		t.Fatalf("expected P2P next_seq=4, got %d", replyP2P.NextSeq)
	}

	// Phase 4: Pull group messages (should include regular + mention)
	pullReqGrp := &v1.PullRequest{
		Topic:   grpTopic,
		LastSeq: 0,
		Limit:   50,
	}
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd:     v1.Command_CMD_PULL,
		Seq:     3,
		Payload: &v1.Packet_PullReq{PullReq: pullReqGrp},
	})

	respGrp := wsReadPacket(t, wsConn, 3*time.Second)
	replyGrp := respGrp.GetPullReply()
	if replyGrp == nil {
		t.Fatalf("expected PullReply payload, got nil")
	}

	// Should have: 2 regular group + 1 mention = 3 messages
	if len(replyGrp.Messages) != 3 {
		t.Fatalf("expected 3 group messages (2 regular + 1 mention), got %d", len(replyGrp.Messages))
	}

	// Verify seqs: 1, 2, 3
	seqSet := make(map[uint64]bool)
	for _, m := range replyGrp.Messages {
		seqSet[m.TopicSeq] = true
	}
	for i := uint64(1); i <= 3; i++ {
		if !seqSet[i] {
			t.Fatalf("missing group message with seq=%d", i)
		}
	}

	// Phase 5: Verify topic seq backups
	p2pMaxSeq, _ := ts.storage.GetTopicMaxSeq(ctx, p2pTopic)
	if p2pMaxSeq != 3 {
		t.Fatalf("expected P2P max_seq=3, got %d", p2pMaxSeq)
	}
	grpMaxSeq, _ := ts.storage.GetTopicMaxSeq(ctx, grpTopic)
	if grpMaxSeq != 3 {
		t.Fatalf("expected group max_seq=3, got %d", grpMaxSeq)
	}

	// Phase 6: Pull again with updated lastSeq -> should be empty
	pullReqP2P2 := &v1.PullRequest{
		Topic:   p2pTopic,
		LastSeq: 3,
		Limit:   50,
	}
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd:     v1.Command_CMD_PULL,
		Seq:     4,
		Payload: &v1.Packet_PullReq{PullReq: pullReqP2P2},
	})

	respP2P2 := wsReadPacket(t, wsConn, 3*time.Second)
	replyP2P2 := respP2P2.GetPullReply()
	if replyP2P2 == nil {
		t.Fatalf("expected PullReply payload, got nil")
	}

	if len(replyP2P2.Messages) != 0 {
		t.Fatalf("expected 0 messages after sync, got %d", len(replyP2P2.Messages))
	}

	t.Logf("full offline sync scenario verified: P2P=%d, Group=%d, mentions included",
		len(replyP2P.Messages), len(replyGrp.Messages))
}
