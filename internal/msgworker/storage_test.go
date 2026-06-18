package msgworker

import (
	"context"
	"os"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"

	"github.com/go-kratos/kratos/v2/log"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	testMongoURI = "mongodb://127.0.0.1:27018"
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
