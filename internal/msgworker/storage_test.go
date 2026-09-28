package msgworker

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"
	agevent "nonoka-im/internal/agent/event"

	"github.com/go-kratos/kratos/v2/log"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type recordingAgentPublisher struct {
	mu    sync.Mutex
	calls []agevent.MessageEvent
	fail  bool
}

func (p *recordingAgentPublisher) Publish(_ context.Context, ev agevent.MessageEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, ev)
	if p.fail {
		return fmt.Errorf("publisher unavailable")
	}
	return nil
}

const (
	testMongoURI = "mongodb://127.0.0.1:27018/?replicaSet=rs0&directConnection=true"
	testMongoDB  = "nonoka_im_test"
)

func TestUpdateDeliveryStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(testMongoURI).SetConnectTimeout(2 * time.Second))
	if err != nil {
		t.Skipf("mongodb not available, skipping storage test: %v", err)
	}
	defer func() {
		_ = client.Disconnect(ctx)
	}()

	if err := client.Ping(ctx, nil); err != nil {
		t.Skipf("mongodb not reachable, skipping storage test: %v", err)
	}

	db := client.Database(testMongoDB)
	storage := NewMessageStorage(db, log.NewStdLogger(os.Stdout))

	userID := int64(100)
	topic := "grp_42"
	topicSeq := uint64(5)
	now := time.Now()

	inboxColl := db.Collection(CollectionInboxes)
	if _, err := inboxColl.InsertOne(ctx, InboxMessage{
		UserID:    userID,
		MsgID:     1,
		Topic:     topic,
		TopicSeq:  topicSeq,
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert inbox message failed: %v", err)
	}

	mentionColl := db.Collection(CollectionMentionInboxes)
	if _, err := mentionColl.InsertOne(ctx, MentionMessage{
		UserID:    userID,
		MsgID:     1,
		Topic:     topic,
		TopicSeq:  topicSeq,
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert mention message failed: %v", err)
	}

	if err := storage.UpdateDeliveryStatus(ctx, userID, topic, topicSeq); err != nil {
		t.Fatalf("update delivery status failed: %v", err)
	}

	var inbox InboxMessage
	if err := inboxColl.FindOne(ctx, bson.M{"user_id": userID, "topic": topic, "topic_seq": topicSeq}).Decode(&inbox); err != nil {
		t.Fatalf("find inbox message failed: %v", err)
	}
	if inbox.DeliveredAt.IsZero() {
		t.Fatal("expected inbox delivered_at to be set")
	}

	var mention MentionMessage
	if err := mentionColl.FindOne(ctx, bson.M{"user_id": userID, "topic": topic, "topic_seq": topicSeq}).Decode(&mention); err != nil {
		t.Fatalf("find mention message failed: %v", err)
	}
	if mention.DeliveredAt.IsZero() {
		t.Fatal("expected mention delivered_at to be set")
	}
}

// TestSaveGroupMessagesBatch verifies that a batch of group messages can be
// inserted and that duplicate batches are idempotent.
func TestSaveGroupMessagesBatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping mongodb-dependent test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(testMongoURI).SetConnectTimeout(2 * time.Second))
	if err != nil {
		t.Skipf("mongodb not available, skipping storage test: %v", err)
	}
	defer func() {
		_ = client.Disconnect(ctx)
	}()

	if err := client.Ping(ctx, nil); err != nil {
		t.Skipf("mongodb not reachable, skipping storage test: %v", err)
	}

	db := client.Database(testMongoDB)
	// Clean up the messages collection so this test is idempotent.
	_, _ = db.Collection(CollectionMessages).DeleteMany(ctx, bson.M{"topic": "grp_1"})

	storage := NewMessageStorage(db, log.NewStdLogger(os.Stdout))

	msgs := []*v1.UpstreamMessage{
		{Topic: "grp_1", SenderId: 1, MsgType: int32(v1.MsgType_MSG_TYPE_TEXT), Content: []byte("m1"), ClientMsgId: "cmid-1"},
		{Topic: "grp_1", SenderId: 2, MsgType: int32(v1.MsgType_MSG_TYPE_TEXT), Content: []byte("m2"), ClientMsgId: "cmid-2"},
	}
	msgIDs := []int64{1, 2}
	topicSeqs := []uint64{1, 2}

	inserted, isDup, err := storage.SaveGroupMessagesBatch(ctx, msgs, msgIDs, topicSeqs)
	if err != nil {
		t.Fatalf("batch insert failed: %v", err)
	}
	if isDup {
		t.Fatal("expected fresh batch, got duplicate")
	}
	if inserted != 2 {
		t.Fatalf("expected inserted=2, got %d", inserted)
	}

	// Re-inserting the same batch should be reported as duplicate.
	inserted, isDup, err = storage.SaveGroupMessagesBatch(ctx, msgs, msgIDs, topicSeqs)
	if err != nil {
		t.Fatalf("duplicate batch insert failed: %v", err)
	}
	if !isDup {
		t.Fatal("expected duplicate batch")
	}
}

// TestTransactionalAgentOutbox verifies the important consistency boundary:
// a newly persisted message and its Agent event become visible together, and
// the relay retries publication without creating a second event. MongoDB
// transactions require a replica set; environments without one skip this
// integration test rather than weakening the assertion.
func TestTransactionalAgentOutbox(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping mongodb-dependent transaction test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testMongoURI).SetConnectTimeout(2 * time.Second))
	if err != nil {
		t.Skipf("mongodb not available, skipping transaction test: %v", err)
	}
	defer func() { _ = client.Disconnect(ctx) }()
	if err := client.Ping(ctx, nil); err != nil {
		t.Skipf("mongodb not reachable, skipping transaction test: %v", err)
	}
	db := client.Database(testMongoDB)
	_, _ = db.Collection(CollectionMessages).DeleteMany(ctx, bson.M{"topic": "grp_outbox_test"})
	storage := NewMessageStorage(db, log.NewStdLogger(os.Stdout))
	if err := storage.EnsureIndexes(ctx); err != nil {
		t.Skipf("mongodb transaction/index setup unavailable: %v", err)
	}
	// Force a failure after the message insert but before the outbox insert.
	// The transaction must roll back both writes.
	invalidMsg := &v1.UpstreamMessage{Topic: "grp_outbox_test", SenderId: 7, MsgType: int32(v1.MsgType_MSG_TYPE_TEXT), Content: []byte("must rollback"), ClientMsgId: "outbox-rollback"}
	_, err = storage.SaveGroupMessageWithAgentEvent(ctx, invalidMsg, 9000, 0, agevent.MessageEvent{SchemaVersion: agevent.SchemaVersion})
	if err == nil {
		t.Fatal("expected invalid event to abort transaction")
	}
	if count, countErr := db.Collection(CollectionMessages).CountDocuments(ctx, bson.M{"msg_id": int64(9000)}); countErr != nil || count != 0 {
		t.Fatalf("message survived aborted transaction: count=%d err=%v", count, countErr)
	}
	ev := agevent.NewMessageEvent(9001, "grp_outbox_test", 7, int32(v1.MsgType_MSG_TYPE_TEXT), time.Now().Unix(), nil)
	_, _ = db.Collection(CollectionAgentOutbox).DeleteOne(ctx, bson.M{"event_id": ev.EventID})
	msg := &v1.UpstreamMessage{Topic: ev.Topic, SenderId: ev.SenderID, MsgType: ev.MsgType, Content: []byte("hello"), ClientMsgId: "outbox-cmid"}
	duplicate, err := storage.SaveGroupMessageWithAgentEvent(ctx, msg, ev.MsgID, 1, ev)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "transaction") || strings.Contains(strings.ToLower(err.Error()), "replica set") {
			t.Skipf("mongodb transactions unavailable: %v", err)
		}
		t.Fatalf("transactional message save failed: %v", err)
	}
	if duplicate {
		t.Fatal("expected first message save to be new")
	}
	var stored StoredMessage
	if err := db.Collection(CollectionMessages).FindOne(ctx, bson.M{"msg_id": ev.MsgID}).Decode(&stored); err != nil {
		t.Fatalf("message was not committed with outbox: %v", err)
	}
	var row AgentEventOutbox
	if err := db.Collection(CollectionAgentOutbox).FindOne(ctx, bson.M{"event_id": ev.EventID}).Decode(&row); err != nil {
		t.Fatalf("outbox row was not committed with message: %v", err)
	}

	publisher := &recordingAgentPublisher{fail: true}
	if _, err := storage.RelayAgentEvents(ctx, publisher, 1, time.Nanosecond); err == nil {
		t.Fatal("expected first relay attempt to fail")
	}
	publisher.fail = false
	_, _ = db.Collection(CollectionAgentOutbox).UpdateOne(ctx, bson.M{"event_id": ev.EventID}, bson.M{"$set": bson.M{"next_attempt_at": time.Now().Add(-time.Second)}})
	if n, err := storage.RelayAgentEvents(ctx, publisher, 1, time.Nanosecond); err != nil || n != 1 {
		t.Fatalf("retry relay failed: n=%d err=%v", n, err)
	}
	if len(publisher.calls) != 2 {
		t.Fatalf("expected two publish attempts, got %d", len(publisher.calls))
	}
	var published AgentEventOutbox
	if err := db.Collection(CollectionAgentOutbox).FindOne(ctx, bson.M{"event_id": ev.EventID}).Decode(&published); err != nil || published.Status != "published" {
		t.Fatalf("expected published outbox row, row=%+v err=%v", published, err)
	}
	duplicate, err = storage.SaveGroupMessageWithAgentEvent(ctx, msg, ev.MsgID+1, 2, ev)
	if err != nil || !duplicate {
		t.Fatalf("expected duplicate event/message to be idempotent: duplicate=%v err=%v", duplicate, err)
	}
	var count int64
	count, err = db.Collection(CollectionAgentOutbox).CountDocuments(ctx, bson.M{"event_id": ev.EventID})
	if err != nil || count != 1 {
		t.Fatalf("expected one outbox row, count=%d err=%v", count, err)
	}
}
