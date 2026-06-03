package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/segmentio/kafka-go"
)

// UpstreamMessage represents a message sent from client to Kafka.
// MsgWorker consumes these messages to persist and dispatch them.
type UpstreamMessage struct {
	SenderID    int64  `json:"sender_id"`
	Topic       string `json:"topic"`
	MsgType     int32  `json:"msg_type"`
	Content     []byte `json:"content"`
	ClientMsgID string `json:"client_msg_id"`
	Timestamp   int64  `json:"timestamp"`
}

// MessageProducer produces upstream messages to Kafka.
type MessageProducer interface {
	Produce(ctx context.Context, msg *UpstreamMessage) error
	Close() error
}

// KafkaProducer implements MessageProducer using kafka-go.
type KafkaProducer struct {
	writer *kafka.Writer
	log    *log.Helper
}

// KafkaConfig holds Kafka producer configuration.
type KafkaConfig struct {
	Brokers   []string
	Topic     string
	BatchSize int
}

// NewKafkaProducer creates a new Kafka producer.
func NewKafkaProducer(cfg KafkaConfig, logger log.Logger) *KafkaProducer {
	if cfg.Topic == "" {
		cfg.Topic = "im-messages"
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 100
	}

	writer := &kafka.Writer{
		Addr:         kafka.TCP(cfg.Brokers...),
		Topic:        cfg.Topic,
		BatchSize:    cfg.BatchSize,
		BatchTimeout: 10 * time.Millisecond,
		RequiredAcks: kafka.RequireOne,
		Async:        false, // sync for guaranteed delivery in gateway
	}

	return &KafkaProducer{
		writer: writer,
		log:    log.NewHelper(logger),
	}
}

// Produce sends an upstream message to Kafka.
// Uses the message's Topic field as the Kafka message key for partition affinity.
func (p *KafkaProducer) Produce(ctx context.Context, msg *UpstreamMessage) error {
	value, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal upstream message: %w", err)
	}

	kmsg := kafka.Message{
		Key:   []byte(msg.Topic),
		Value: value,
		Headers: []kafka.Header{
			{Key: "sender_id", Value: []byte(fmt.Sprintf("%d", msg.SenderID))},
			{Key: "client_msg_id", Value: []byte(msg.ClientMsgID)},
		},
	}

	if err := p.writer.WriteMessages(ctx, kmsg); err != nil {
		return fmt.Errorf("write to kafka: %w", err)
	}

	p.log.Infof("produced message to kafka: topic=%s sender=%d client_msg_id=%s",
		msg.Topic, msg.SenderID, msg.ClientMsgID)
	return nil
}

// Close closes the Kafka writer.
func (p *KafkaProducer) Close() error {
	return p.writer.Close()
}

// NoopProducer is a no-op producer for testing or when Kafka is disabled.
type NoopProducer struct{}

// NewNoopProducer creates a no-op producer.
func NewNoopProducer() *NoopProducer {
	return &NoopProducer{}
}

// Produce does nothing.
func (p *NoopProducer) Produce(ctx context.Context, msg *UpstreamMessage) error {
	return nil
}

// Close does nothing.
func (p *NoopProducer) Close() error {
	return nil
}
