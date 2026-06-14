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

	// Verify: GetOfflineMessages returns the group message for group topic
	offlineMsgs, err := worker.Storage().GetOfflineMessages(ctx, mentionedUID, topic, 0, 50)
	if err != nil {
		t.Fatalf("get offline messages failed: %v", err)
	}
	if len(offlineMsgs) != 1 {
		t.Fatalf("expected 1 offline message for group topic (from messages collection), got %d", len(offlineMsgs))
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

	pusher, err := msgworker.NewGatewayPusher([]string{grpcAddr}, testLogger)
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

	pusher, err := msgworker.NewGatewayPusher([]string{grpcAddr}, testLogger)
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
