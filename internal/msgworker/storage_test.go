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

// TestAgentOutboxDueIndexCoversLock verifies the due index sorts by
// locked_until as well, so expired-lease scans stay index-only.
func TestAgentOutboxDueIndexCoversLock(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping mongodb-dependent test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testMongoURI).SetConnectTimeout(2 * time.Second))
	if err != nil {
		t.Skipf("mongodb not available, skipping storage test: %v", err)
	}
	defer func() { _ = client.Disconnect(ctx) }()
	if err := client.Ping(ctx, nil); err != nil {
		t.Skipf("mongodb not reachable, skipping storage test: %v", err)
	}
	db := client.Database(testMongoDB)
	storage := NewMessageStorage(db, log.NewStdLogger(os.Stdout))
	if err := storage.EnsureIndexes(ctx); err != nil {
		t.Skipf("mongodb index setup unavailable: %v", err)
	}
	cursor, err := db.Collection(CollectionAgentOutbox).Indexes().List(ctx)
	if err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	defer cursor.Close(ctx)
	found := false
	for cursor.Next(ctx) {
		var idx struct {
			Name string `bson:"name"`
			Key  bson.M `bson:"key"`
		}
		if err := cursor.Decode(&idx); err != nil {
			t.Fatalf("decode index: %v", err)
		}
		if idx.Name != "agent_outbox_due" {
			continue
		}
		found = true
		for _, k := range []string{"status", "next_attempt_at", "locked_until"} {
			if _, ok := idx.Key[k]; !ok {
				t.Fatalf("agent_outbox_due index missing key %q: %v", k, idx.Key)
			}
		}
	}
	if !found {
		t.Fatal("agent_outbox_due index not found")
	}
}

// insertOutboxRow inserts an outbox row directly with a valid event payload.
func insertOutboxRow(t *testing.T, ctx context.Context, db *mongo.Database, ev agevent.MessageEvent, status string, attempts int, nextAttemptAt time.Time) {
	t.Helper()
	payload, err := ev.Marshal()
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	_, err = db.Collection(CollectionAgentOutbox).InsertOne(ctx, AgentEventOutbox{
		EventID: ev.EventID, Topic: ev.Topic, TopicSeq: ev.TopicSeq, Payload: payload,
		Status: status, Attempts: attempts, NextAttemptAt: nextAttemptAt, CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("insert outbox row: %v", err)
	}
}

// TestRelayAgentEventsAttemptsCap verifies that a row whose attempts exceed
// agentEventMaxAttempts is parked as failed without another publish attempt.
func TestRelayAgentEventsAttemptsCap(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping mongodb-dependent test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testMongoURI).SetConnectTimeout(2 * time.Second))
	if err != nil {
		t.Skipf("mongodb not available, skipping storage test: %v", err)
	}
	defer func() { _ = client.Disconnect(ctx) }()
	if err := client.Ping(ctx, nil); err != nil {
		t.Skipf("mongodb not reachable, skipping storage test: %v", err)
	}
	db := client.Database(testMongoDB)
	_, _ = db.Collection(CollectionAgentOutbox).DeleteMany(ctx, bson.M{"topic": "grp_attempts_cap"})
	storage := NewMessageStorage(db, log.NewStdLogger(os.Stdout))

	ev := agevent.NewMessageEvent(9101, "grp_attempts_cap", 7, int32(v1.MsgType_MSG_TYPE_TEXT), time.Now().Unix(), nil)
	insertOutboxRow(t, ctx, db, ev, "pending", agentEventMaxAttempts, time.Now().Add(-time.Second))

	publisher := &recordingAgentPublisher{}
	n, err := storage.RelayAgentEvents(ctx, publisher, 1, time.Nanosecond)
	if err != nil {
		t.Fatalf("relay returned error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected exhausted row to be skipped, processed=%d", n)
	}
	if len(publisher.calls) != 0 {
		t.Fatalf("exhausted row must not be published, got %d calls", len(publisher.calls))
	}
	var row AgentEventOutbox
	if err := db.Collection(CollectionAgentOutbox).FindOne(ctx, bson.M{"event_id": ev.EventID}).Decode(&row); err != nil {
		t.Fatalf("outbox row vanished: %v", err)
	}
	if row.Status != "failed" {
		t.Fatalf("expected row parked as failed, got status=%q", row.Status)
	}
	if row.Attempts != agentEventMaxAttempts+1 {
		t.Fatalf("expected attempts=%d, got %d", agentEventMaxAttempts+1, row.Attempts)
	}
	// A second relay pass must not pick the failed row up again.
	if n, err := storage.RelayAgentEvents(ctx, publisher, 1, time.Nanosecond); err != nil || n != 0 {
		t.Fatalf("failed row must not be retried: n=%d err=%v", n, err)
	}
	if len(publisher.calls) != 0 {
		t.Fatalf("failed row must stay unpublished, got %d calls", len(publisher.calls))
	}
}

// TestRelayAgentEventsPrunesPublished verifies the relay cleans up published
// rows older than the retention window after a successful publish while
// leaving recent published rows untouched.
func TestRelayAgentEventsPrunesPublished(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping mongodb-dependent test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testMongoURI).SetConnectTimeout(2 * time.Second))
	if err != nil {
		t.Skipf("mongodb not available, skipping storage test: %v", err)
	}
	defer func() { _ = client.Disconnect(ctx) }()
	if err := client.Ping(ctx, nil); err != nil {
		t.Skipf("mongodb not reachable, skipping storage test: %v", err)
	}
	db := client.Database(testMongoDB)
	_, _ = db.Collection(CollectionAgentOutbox).DeleteMany(ctx, bson.M{"topic": bson.M{"$in": []string{"grp_prune", "grp_prune_recent"}}})
	storage := NewMessageStorage(db, log.NewStdLogger(os.Stdout))

	staleEv := agevent.NewMessageEvent(9201, "grp_prune", 7, int32(v1.MsgType_MSG_TYPE_TEXT), time.Now().Unix(), nil)
	insertOutboxRow(t, ctx, db, staleEv, "published", 1, time.Now().Add(-time.Hour))
	if _, err := db.Collection(CollectionAgentOutbox).UpdateOne(ctx, bson.M{"event_id": staleEv.EventID}, bson.M{"$set": bson.M{"published_at": time.Now().Add(-agentEventPublishedRetention - time.Hour)}}); err != nil {
		t.Fatalf("backdate published_at: %v", err)
	}
	recentEv := agevent.NewMessageEvent(9202, "grp_prune_recent", 7, int32(v1.MsgType_MSG_TYPE_TEXT), time.Now().Unix(), nil)
	insertOutboxRow(t, ctx, db, recentEv, "published", 1, time.Now().Add(-time.Hour))
	if _, err := db.Collection(CollectionAgentOutbox).UpdateOne(ctx, bson.M{"event_id": recentEv.EventID}, bson.M{"$set": bson.M{"published_at": time.Now().Add(-time.Hour)}}); err != nil {
		t.Fatalf("set published_at: %v", err)
	}
	freshEv := agevent.NewMessageEvent(9203, "grp_prune", 7, int32(v1.MsgType_MSG_TYPE_TEXT), time.Now().Unix(), nil)
	insertOutboxRow(t, ctx, db, freshEv, "pending", 0, time.Now().Add(-time.Second))

	publisher := &recordingAgentPublisher{}
	if n, err := storage.RelayAgentEvents(ctx, publisher, 1, time.Nanosecond); err != nil || n != 1 {
		t.Fatalf("relay fresh event failed: n=%d err=%v", n, err)
	}
	if count, err := db.Collection(CollectionAgentOutbox).CountDocuments(ctx, bson.M{"event_id": staleEv.EventID}); err != nil || count != 0 {
		t.Fatalf("stale published row was not pruned: count=%d err=%v", count, err)
	}
	if count, err := db.Collection(CollectionAgentOutbox).CountDocuments(ctx, bson.M{"event_id": recentEv.EventID}); err != nil || count != 1 {
		t.Fatalf("recent published row must be kept: count=%d err=%v", count, err)
	}
}

// TestRelayAgentEventsTopicOrder verifies the claim picks the oldest due row
// per topic and blocks a row while an earlier-seq sibling is still unpublished.
func TestRelayAgentEventsTopicOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping mongodb-dependent test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testMongoURI).SetConnectTimeout(2 * time.Second))
	if err != nil {
		t.Skipf("mongodb not available, skipping storage test: %v", err)
	}
	defer func() { _ = client.Disconnect(ctx) }()
	if err := client.Ping(ctx, nil); err != nil {
		t.Skipf("mongodb not reachable, skipping storage test: %v", err)
	}
	db := client.Database(testMongoDB)
	_, _ = db.Collection(CollectionAgentOutbox).DeleteMany(ctx, bson.M{"topic": bson.M{"$in": []string{"grp_relay_order", "grp_relay_block"}}})
	storage := NewMessageStorage(db, log.NewStdLogger(os.Stdout))

	// Two due rows in the same topic: the lower topic_seq must be claimed first.
	ev1 := agevent.NewMessageEvent(9301, "grp_relay_order", 7, int32(v1.MsgType_MSG_TYPE_TEXT), time.Now().Unix(), nil)
	ev1.TopicSeq = 1
	insertOutboxRow(t, ctx, db, ev1, "pending", 0, time.Now().Add(-time.Second))
	ev2 := agevent.NewMessageEvent(9302, "grp_relay_order", 7, int32(v1.MsgType_MSG_TYPE_TEXT), time.Now().Unix(), nil)
	ev2.TopicSeq = 2
	insertOutboxRow(t, ctx, db, ev2, "pending", 0, time.Now().Add(-time.Second))

	publisher := &recordingAgentPublisher{}
	if n, err := storage.RelayAgentEvents(ctx, publisher, 1, time.Nanosecond); err != nil || n != 1 {
		t.Fatalf("first relay failed: n=%d err=%v", n, err)
	}
	if len(publisher.calls) != 1 || publisher.calls[0].TopicSeq != 1 {
		t.Fatalf("expected topic_seq 1 first, got %+v", publisher.calls)
	}
	if n, err := storage.RelayAgentEvents(ctx, publisher, 1, time.Nanosecond); err != nil || n != 1 {
		t.Fatalf("second relay failed: n=%d err=%v", n, err)
	}
	if len(publisher.calls) != 2 || publisher.calls[1].TopicSeq != 2 {
		t.Fatalf("expected topic_seq 2 second, got %+v", publisher.calls)
	}

	// seq 1 not yet due (backoff) blocks seq 2 even though seq 2 is due.
	evB1 := agevent.NewMessageEvent(9303, "grp_relay_block", 7, int32(v1.MsgType_MSG_TYPE_TEXT), time.Now().Unix(), nil)
	evB1.TopicSeq = 1
	insertOutboxRow(t, ctx, db, evB1, "pending", 0, time.Now().Add(time.Hour))
	evB2 := agevent.NewMessageEvent(9304, "grp_relay_block", 7, int32(v1.MsgType_MSG_TYPE_TEXT), time.Now().Unix(), nil)
	evB2.TopicSeq = 2
	insertOutboxRow(t, ctx, db, evB2, "pending", 0, time.Now().Add(-time.Second))
	callsBefore := len(publisher.calls)
	if n, err := storage.RelayAgentEvents(ctx, publisher, 4, time.Nanosecond); err != nil || n != 0 {
		t.Fatalf("blocked row must not be relayed: n=%d err=%v", n, err)
	}
	if len(publisher.calls) != callsBefore {
		t.Fatalf("blocked row was published: calls=%d", len(publisher.calls))
	}
	var blocked AgentEventOutbox
	if err := db.Collection(CollectionAgentOutbox).FindOne(ctx, bson.M{"event_id": evB2.EventID}).Decode(&blocked); err != nil || blocked.Status != "pending" {
		t.Fatalf("blocked row must stay pending: row=%+v err=%v", blocked, err)
	}
}
