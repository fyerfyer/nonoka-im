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

	"github.com/segmentio/kafka-go"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
)

// setupMsgWorker creates a MsgWorker with real dependencies for testing.
func setupMsgWorker(t *testing.T) (*msgworker.MsgWorker, *mongo.Database, func()) {
	db := setupMongoDB(t)
	redisClient := setupTestRedis(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := redisClient.Del(ctx, "im:seq:*").Err(); err != nil {
		t.Logf("warning: failed to clean redis keys: %v", err)
	}

	seqGen := msgworker.NewSeqGenerator(redisClient, testLogger)
	snowflake := msgworker.NewSnowflake(1)
	storage := msgworker.NewMessageStorage(db, testLogger)

	if err := storage.EnsureIndexes(ctx); err != nil {
		t.Fatalf("failed to ensure indexes: %v", err)
	}

	worker := msgworker.NewMsgWorker(nil, seqGen, snowflake, storage, nil, testLogger)

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, coll := range []string{"messages", "inboxes", "topic_seqs", "mention_inboxes"} {
			_ = db.Collection(coll).Drop(ctx)
		}
	}

	return worker, db, cleanup
}

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

	inboxColl := db.Collection(msgworker.CollectionInboxes)
	count := countMongoDocs(t, inboxColl, bson.M{"user_id": receiverID})
	if count != 1 {
		t.Fatalf("expected 1 inbox message for receiver %d, got %d", receiverID, count)
	}

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

	senderCount := countMongoDocs(t, inboxColl, bson.M{"user_id": senderID})
	if senderCount != 1 {
		t.Fatalf("expected 1 inbox message for sender %d, got %d", senderID, senderCount)
	}
}

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

	inboxColl := db.Collection(msgworker.CollectionInboxes)
	inboxCount := countMongoDocs(t, inboxColl, bson.M{})
	if inboxCount != 0 {
		t.Fatalf("expected 0 inbox messages for group message, got %d", inboxCount)
	}
}

func TestMsgWorker_SystemMessage(t *testing.T) {
	worker, db, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	targetUID := int64(999)
	topic := "sys_999"

	upstream := &v1.UpstreamMessage{
		SenderId:    0,
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
}

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
					ClientMsgId: fmt.Sprintf("concurrent-%d-%d", idx, j),
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

	expectedCount := concurrency * msgsPerGoroutine
	inboxColl := db.Collection(msgworker.CollectionInboxes)
	cursor, err := inboxColl.Find(ctx, bson.M{"topic": topic, "user_id": int64(2)})
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

	for i := uint64(1); i <= uint64(expectedCount); i++ {
		if !seqSet[i] {
			t.Fatalf("missing seq %d", i)
		}
	}

	seqColl := db.Collection(msgworker.CollectionTopicSeqs)
	var backup msgworker.TopicSeqBackup
	if err := seqColl.FindOne(ctx, bson.M{"topic": topic}).Decode(&backup); err != nil {
		t.Fatalf("failed to find topic seq backup: %v", err)
	}
	if backup.MaxSeq != uint64(expectedCount) {
		t.Fatalf("expected max_seq=%d, got %d", expectedCount, backup.MaxSeq)
	}
}

func TestMsgWorker_E2E_KafkaToMongoDB(t *testing.T) {
	if err := waitForKafka(testKafkaBroker); err != nil {
		t.Fatalf("kafka not ready: %v", err)
	}
	topic := fmt.Sprintf("msgworker-e2e-test-%d", time.Now().UnixNano())
	if err := cleanupAndCreateTopic(testKafkaBroker, topic, 3); err != nil {
		t.Fatalf("failed to create kafka topic: %v", err)
	}
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
		GroupID:     fmt.Sprintf("msgworker-e2e-group-%d", time.Now().UnixNano()),
		StartOffset: kafka.FirstOffset,
	}

	consumer := msgworker.NewKafkaConsumer(kafkaCfg, nil, testLogger)
	worker := msgworker.NewMsgWorker(consumer, seqGen, snowflake, storage, nil, testLogger)
	consumer.SetHandler(worker.HandleMessage)

	var workerErr error
	var workerDone sync.WaitGroup
	workerDone.Add(1)
	go func() {
		defer workerDone.Done()
		workerErr = worker.Start(ctx)
	}()

	time.Sleep(1 * time.Second)

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
			ClientMsgId: fmt.Sprintf("e2e-msg-%d", i),
			Timestamp:   time.Now().UnixMilli(),
		}
		produceMessageWithRetry(ctx, t, producer, upstream)
	}

	time.Sleep(3 * time.Second)

	cancel()
	workerDone.Wait()
	if workerErr != nil && workerErr != context.Canceled {
		t.Logf("worker exited with error (may be expected): %v", workerErr)
	}

	verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer verifyCancel()

	inboxColl := db.Collection(msgworker.CollectionInboxes)
	count := countMongoDocs(t, inboxColl, bson.M{"topic": "p2p_1_2", "user_id": int64(2)})
	if count != messageCount {
		t.Fatalf("expected %d inbox messages for receiver, got %d", messageCount, count)
	}

	cursor, err := inboxColl.Find(verifyCtx, bson.M{"topic": "p2p_1_2", "user_id": int64(2)})
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
}

func TestMsgWorker_KafkaConsumer_GracefulShutdown(t *testing.T) {
	if err := waitForKafka(testKafkaBroker); err != nil {
		t.Fatalf("kafka not ready: %v", err)
	}
	topic := fmt.Sprintf("msgworker-shutdown-test-%d", time.Now().UnixNano())
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
		GroupID: fmt.Sprintf("msgworker-shutdown-group-%d", time.Now().UnixNano()),
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

	time.Sleep(300 * time.Millisecond)

	cancel()
	wg.Wait()

	if err := worker.Stop(); err != nil {
		t.Fatalf("stop worker failed: %v", err)
	}
}

func TestMsgWorker_GroupMention_SameUser_Duplicate(t *testing.T) {
	worker, db, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	senderID := int64(100)
	topic := "grp_42"
	mentionedUser := int64(200)

	upstream := &v1.UpstreamMessage{
		SenderId:         senderID,
		Topic:            topic,
		MsgType:          int32(v1.MsgType_MSG_TYPE_TEXT),
		Content:          []byte("hello @user1"),
		ClientMsgId:      "mention-msg-dup-001",
		Timestamp:        time.Now().UnixMilli(),
		MentionedUserIds: []int64{mentionedUser},
	}
	data, _ := proto.Marshal(upstream)

	if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
		t.Fatalf("first handle failed: %v", err)
	}

	if err := worker.HandleMessage(ctx, []byte(topic), data, nil); err != nil {
		t.Fatalf("second handle failed: %v", err)
	}

	mentionColl := db.Collection(msgworker.CollectionMentionInboxes)
	count := countMongoDocs(t, mentionColl, bson.M{"user_id": mentionedUser, "topic": topic})
	if count != 1 {
		t.Fatalf("expected 1 mention inbox after duplicate, got %d", count)
	}
}
