package integration

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/gateway"
	kafkax "nonoka-im/internal/kafka"

	"github.com/segmentio/kafka-go"
)

// TestKafkaConsumer_PoisonMessageGoesToDLQ verifies that a permanent handler
// failure is parked in the dead-letter topic instead of blocking the partition
// forever.
func TestKafkaConsumer_PoisonMessageGoesToDLQ(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires kafka")
	}
	if err := waitForKafka(testKafkaBroker); err != nil {
		t.Fatalf("kafka not ready: %v", err)
	}

	suffix := time.Now().UnixNano()
	topic := fmt.Sprintf("kafka-dlq-source-%d", suffix)
	dlqTopic := topic + "-dlq"
	if err := cleanupAndCreateTopic(testKafkaBroker, topic, 1); err != nil {
		t.Fatalf("create source topic: %v", err)
	}
	if err := cleanupAndCreateTopic(testKafkaBroker, dlqTopic, 1); err != nil {
		t.Fatalf("create dlq topic: %v", err)
	}
	t.Cleanup(func() {
		conn, err := kafka.Dial("tcp", testKafkaBroker)
		if err != nil {
			return
		}
		_ = conn.DeleteTopics(topic, dlqTopic)
		_ = conn.Close()
	})
	if err := waitForTopicReady(testKafkaBroker, topic); err != nil {
		t.Fatalf("source topic not ready: %v", err)
	}
	if err := waitForTopicReady(testKafkaBroker, dlqTopic); err != nil {
		t.Fatalf("dlq topic not ready: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	consumer := kafkax.NewConsumer(kafkax.ConsumerConfig{
		Brokers:         []string{testKafkaBroker},
		Topic:           topic,
		GroupID:         fmt.Sprintf("kafka-dlq-group-%d", suffix),
		StartOffset:     kafka.FirstOffset,
		WorkerCount:     1,
		MaxRetries:      0,
		HandlerTimeout:  2 * time.Second,
		RetryBackoff:    20 * time.Millisecond,
		CommitBatchSize: 1,
		DLQTopic:        dlqTopic,
	}, func(ctx context.Context, key, value []byte, headers map[string]string) error {
		return errors.New("permanent validation failure")
	}, testLogger)
	go func() { _ = consumer.Start(ctx) }()

	producer := gateway.NewKafkaProducer(gateway.KafkaConfig{
		Brokers: []string{testKafkaBroker},
		Topic:   topic,
	}, testLogger)
	defer producer.Close()
	produceMessageWithRetry(ctx, t, producer, &v1.UpstreamMessage{
		SenderId: 1, Topic: "p2p_1_2", MsgType: int32(v1.MsgType_MSG_TYPE_TEXT),
		Content: []byte("poison"), ClientMsgId: fmt.Sprintf("poison-%d", suffix),
		Timestamp: time.Now().UnixMilli(),
	})

	readCtx, readCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer readCancel()
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{testKafkaBroker},
		Topic:    dlqTopic,
		GroupID:  fmt.Sprintf("kafka-dlq-reader-%d", suffix),
		MinBytes: 1,
		MaxBytes: 10e6,
		MaxWait:  500 * time.Millisecond,
	})
	defer reader.Close()
	msg, err := reader.ReadMessage(readCtx)
	if err != nil {
		t.Fatalf("read DLQ message: %v", err)
	}
	if len(msg.Value) == 0 {
		t.Fatal("DLQ message has empty value")
	}

	cancel()
	_ = consumer.Stop()
}

// TestKafkaConsumer_CommitsOnlyAfterHandlerSuccess verifies that a consumer
// restarted with the same group does not replay a message whose handler
// already succeeded.
func TestKafkaConsumer_CommitsOnlyAfterHandlerSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires kafka")
	}
	if err := waitForKafka(testKafkaBroker); err != nil {
		t.Fatalf("kafka not ready: %v", err)
	}

	suffix := time.Now().UnixNano()
	topic := fmt.Sprintf("kafka-commit-source-%d", suffix)
	if err := cleanupAndCreateTopic(testKafkaBroker, topic, 1); err != nil {
		t.Fatalf("create source topic: %v", err)
	}
	t.Cleanup(func() {
		conn, err := kafka.Dial("tcp", testKafkaBroker)
		if err != nil {
			return
		}
		_ = conn.DeleteTopics(topic)
		_ = conn.Close()
	})
	if err := waitForTopicReady(testKafkaBroker, topic); err != nil {
		t.Fatalf("topic not ready: %v", err)
	}

	groupID := fmt.Sprintf("kafka-commit-group-%d", suffix)
	var firstCalls atomic.Int64
	firstDone := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	consumer := kafkax.NewConsumer(kafkax.ConsumerConfig{
		Brokers:         []string{testKafkaBroker},
		Topic:           topic,
		GroupID:         groupID,
		StartOffset:     kafka.FirstOffset,
		WorkerCount:     1,
		CommitBatchSize: 1,
	}, func(ctx context.Context, key, value []byte, headers map[string]string) error {
		if firstCalls.Add(1) == 1 {
			close(firstDone)
		}
		return nil
	}, testLogger)
	go func() { _ = consumer.Start(ctx) }()

	producer := gateway.NewKafkaProducer(gateway.KafkaConfig{
		Brokers: []string{testKafkaBroker},
		Topic:   topic,
	}, testLogger)
	defer producer.Close()
	produceMessageWithRetry(ctx, t, producer, &v1.UpstreamMessage{
		SenderId: 1, Topic: "p2p_1_2", MsgType: int32(v1.MsgType_MSG_TYPE_TEXT),
		Content: []byte("committed once"), ClientMsgId: fmt.Sprintf("commit-%d", suffix),
		Timestamp: time.Now().UnixMilli(),
	})

	select {
	case <-firstDone:
	case <-time.After(15 * time.Second):
		t.Fatal("first consumer did not process the message")
	}
	// Give the commit interval time to flush the pending offset.
	time.Sleep(2500 * time.Millisecond)
	cancel()
	_ = consumer.Stop()

	var replayCalls atomic.Int64
	replayCtx, replayCancel := context.WithCancel(context.Background())
	defer replayCancel()
	replay := kafkax.NewConsumer(kafkax.ConsumerConfig{
		Brokers:     []string{testKafkaBroker},
		Topic:       topic,
		GroupID:     groupID,
		StartOffset: kafka.FirstOffset,
		WorkerCount: 1,
	}, func(ctx context.Context, key, value []byte, headers map[string]string) error {
		replayCalls.Add(1)
		return nil
	}, testLogger)
	go func() { _ = replay.Start(replayCtx) }()
	time.Sleep(3 * time.Second)
	replayCancel()
	_ = replay.Stop()

	if got := replayCalls.Load(); got != 0 {
		t.Fatalf("restarted consumer replayed %d already-committed messages", got)
	}
}

// TestKafkaConsumer_TransientErrorsDoNotConsumePoisonBudget verifies that a
// short infrastructure outage retries independently of MaxRetries. Before the
// fix, a handler returning context.DeadlineExceeded was sent to the DLQ after
// MaxRetries+1 attempts, losing valid messages during any DB/Redis failover.
func TestKafkaConsumer_TransientErrorsDoNotConsumePoisonBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires kafka")
	}
	if err := waitForKafka(testKafkaBroker); err != nil {
		t.Fatalf("kafka not ready: %v", err)
	}

	suffix := time.Now().UnixNano()
	topic := fmt.Sprintf("kafka-transient-source-%d", suffix)
	dlqTopic := topic + "-dlq"
	if err := cleanupAndCreateTopic(testKafkaBroker, topic, 1); err != nil {
		t.Fatalf("create source topic: %v", err)
	}
	if err := cleanupAndCreateTopic(testKafkaBroker, dlqTopic, 1); err != nil {
		t.Fatalf("create dlq topic: %v", err)
	}
	t.Cleanup(func() {
		conn, err := kafka.Dial("tcp", testKafkaBroker)
		if err != nil {
			return
		}
		_ = conn.DeleteTopics(topic, dlqTopic)
		_ = conn.Close()
	})
	if err := waitForTopicReady(testKafkaBroker, topic); err != nil {
		t.Fatalf("source topic not ready: %v", err)
	}
	if err := waitForTopicReady(testKafkaBroker, dlqTopic); err != nil {
		t.Fatalf("dlq topic not ready: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int64
	consumer := kafkax.NewConsumer(kafkax.ConsumerConfig{
		Brokers:             []string{testKafkaBroker},
		Topic:               topic,
		GroupID:             fmt.Sprintf("kafka-transient-group-%d", suffix),
		StartOffset:         kafka.FirstOffset,
		WorkerCount:         1,
		MaxRetries:          0, // poison budget is one attempt
		TransientMaxRetries: 2,
		TransientBackoff:    10 * time.Millisecond,
		HandlerTimeout:      2 * time.Second,
		CommitBatchSize:     1,
		DLQTopic:            dlqTopic,
	}, func(ctx context.Context, key, value []byte, headers map[string]string) error {
		calls.Add(1)
		// Mongo/OpenFGA outages surface as deadline/network errors.
		return context.DeadlineExceeded
	}, testLogger)
	go func() { _ = consumer.Start(ctx) }()

	producer := gateway.NewKafkaProducer(gateway.KafkaConfig{
		Brokers: []string{testKafkaBroker}, Topic: topic,
	}, testLogger)
	defer producer.Close()
	produceMessageWithRetry(ctx, t, producer, &v1.UpstreamMessage{
		SenderId: 1, Topic: "p2p_1_2", MsgType: int32(v1.MsgType_MSG_TYPE_TEXT),
		Content: []byte("transient"), ClientMsgId: fmt.Sprintf("transient-%d", suffix), Timestamp: time.Now().UnixMilli(),
	})

	// Wait until the consumer has exhausted the transient budget and parked
	// the message in the DLQ.
	readCtx, readCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer readCancel()
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{testKafkaBroker},
		Topic:    dlqTopic,
		GroupID:  fmt.Sprintf("kafka-transient-reader-%d", suffix),
		MinBytes: 1,
		MaxBytes: 10e6,
		MaxWait:  200 * time.Millisecond,
	})
	defer reader.Close()
	if _, err := reader.ReadMessage(readCtx); err != nil {
		t.Fatalf("read DLQ message: %v", err)
	}
	cancel()
	_ = consumer.Stop()

	// 2 transient retries + the final attempt that exceeds the budget.
	if got := calls.Load(); got != 3 {
		t.Fatalf("handler calls = %d, want 3 (transient retries must not stop at MaxRetries=0)", got)
	}
}
