package gateway

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	v1 "nonoka-im/api/im/v1"
	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
)

// MessageProducer produces upstream messages to Kafka.
type MessageProducer interface {
	Produce(ctx context.Context, msg *v1.UpstreamMessage) error
	Close() error
}

// KafkaConfig holds Kafka producer configuration.
type KafkaConfig struct {
	Brokers       []string
	Topic         string
	BatchSize     int
	BatchTimeout  time.Duration
	RequiredAcks  kafka.RequiredAcks
	Compression   kafka.Compression
	ReadTimeout   time.Duration
	WriteTimeout  time.Duration
	MaxAttempts   int
}

// KafkaProducer implements MessageProducer using kafka-go.
type KafkaProducer struct {
	writer *kafka.Writer
	log    *log.Helper
}

// NewKafkaProducer creates a new Kafka producer.
func NewKafkaProducer(cfg KafkaConfig, logger log.Logger) *KafkaProducer {
	if cfg.Topic == "" {
		cfg.Topic = "im-messages"
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 100
	}
	if cfg.BatchTimeout == 0 {
		cfg.BatchTimeout = 100 * time.Millisecond
	}
	if cfg.RequiredAcks == 0 {
		// RequireAll ensures message is replicated to all ISR members before ack.
		// This prevents message loss when the leader fails immediately after ack.
		cfg.RequiredAcks = kafka.RequireAll
	}
	if cfg.Compression == 0 {
		// LZ4 offers a good balance between compression ratio and CPU usage.
		cfg.Compression = kafka.Lz4
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 10 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 10 * time.Second
	}
	if cfg.MaxAttempts == 0 {
		cfg.MaxAttempts = 3
	}

	writer := &kafka.Writer{
		Addr:         kafka.TCP(cfg.Brokers...),
		Topic:        cfg.Topic,
		BatchSize:    cfg.BatchSize,
		BatchTimeout: cfg.BatchTimeout,
		RequiredAcks: cfg.RequiredAcks,
		Compression:  cfg.Compression,
		Async:        false, // sync for guaranteed delivery in gateway
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		MaxAttempts:  cfg.MaxAttempts,
	}

	return &KafkaProducer{
		writer: writer,
		log:    log.NewHelper(logger),
	}
}

// Produce sends an upstream message to Kafka.
// Uses the message's Topic field as the Kafka message key for partition affinity.
func (p *KafkaProducer) Produce(ctx context.Context, msg *v1.UpstreamMessage) error {
	value, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal upstream message: %w", err)
	}

	kmsg := kafka.Message{
		Key:   []byte(msg.GetTopic()),
		Value: value,
		Headers: []kafka.Header{
			{Key: "sender_id", Value: []byte(fmt.Sprintf("%d", msg.GetSenderId()))},
			{Key: "client_msg_id", Value: []byte(msg.GetClientMsgId())},
		},
	}

	if err := p.writer.WriteMessages(ctx, kmsg); err != nil {
		return fmt.Errorf("write to kafka: %w", err)
	}

	p.log.Debugf("produced message to kafka: topic=%s sender=%d client_msg_id=%s",
		msg.GetTopic(), msg.GetSenderId(), msg.GetClientMsgId())
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
func (p *NoopProducer) Produce(ctx context.Context, msg *v1.UpstreamMessage) error {
	return nil
}

// Close does nothing.
func (p *NoopProducer) Close() error {
	return nil
}
