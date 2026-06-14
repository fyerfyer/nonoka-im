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

	"github.com/segmentio/kafka-go"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
)

func TestMsgWorker_KafkaConsumer_ParallelWorkers(t *testing.T) {
	if err := waitForKafka(testKafkaBroker); err != nil {
		t.Fatalf("kafka not ready: %v", err)
	}

	topic := fmt.Sprintf("msgworker-parallel-test-%d", time.Now().UnixNano())
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
		GroupID:     fmt.Sprintf("msgworker-parallel-group-%d", time.Now().UnixNano()),
		StartOffset: kafka.FirstOffset,
		WorkerCount: 4,
	}

	consumer := msgworker.NewKafkaConsumer(kafkaCfg, handler, testLogger)

	var wg sync.WaitGroup
	wg.Go(func() {
		_ = consumer.Start(ctx)
	})

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

	time.Sleep(3 * time.Second)

	cancel()
	wg.Wait()
	_ = consumer.Stop()

	if processed != messageCount {
		t.Fatalf("expected %d messages processed, got %d", messageCount, processed)
	}
}

func TestMsgWorker_DuplicateMessage_DoesNotAdvanceMaxSeq(t *testing.T) {
	worker, db, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	topic := "p2p_1_2"
	clientMsgID := "dup-seq-test-001"

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
}

func TestStorage_GetOfflineMessages_GroupEnforcesLimit(t *testing.T) {
	worker, _, cleanup := setupMsgWorker(t)
	defer cleanup()

	ctx := context.Background()
	userID := int64(200)
	mentionedUserID := int64(200)
	topic := "grp_limit_42"

	for i := range 3 {
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

	for i := range 2 {
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

	msgs, err := worker.Storage().GetOfflineMessages(ctx, userID, topic, 0, 3)
	if err != nil {
		t.Fatalf("get offline messages failed: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages (limit enforced), got %d", len(msgs))
	}

	for i := 1; i < len(msgs); i++ {
		if msgs[i].TopicSeq < msgs[i-1].TopicSeq {
			t.Fatalf("messages not sorted by seq: %v", msgs)
		}
	}
}
