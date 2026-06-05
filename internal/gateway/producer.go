package gateway

import (
	"context"
	"fmt"
	"sync"
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
	Async         bool // true for async mode, false for sync mode
}

// KafkaProducer implements MessageProducer using kafka-go.
type KafkaProducer struct {
	writer      *kafka.Writer
	log         *log.Helper
	failedMu    sync.RWMutex
	failedCount int64
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
		cfg.WriteTimeout = 2 * time.Second // reduced from 10s for faster failure detection
	}
	if cfg.MaxAttempts == 0 {
		cfg.MaxAttempts = 3
	}

	p := &KafkaProducer{
		log: log.NewHelper(logger),
	}

	writer := &kafka.Writer{
		Addr:         kafka.TCP(cfg.Brokers...),
		Topic:        cfg.Topic,
		BatchSize:    cfg.BatchSize,
		BatchTimeout: cfg.BatchTimeout,
		RequiredAcks: cfg.RequiredAcks,
		Compression:  cfg.Compression,
		Async:        cfg.Async,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		MaxAttempts:  cfg.MaxAttempts,
		Completion:   p.onCompletion, // track async delivery results
	}

	p.writer = writer
	return p
}

// onCompletion handles async delivery results from kafka-go.
func (p *KafkaProducer) onCompletion(messages []kafka.Message, err error) {
	if err != nil {
		p.failedMu.Lock()
		p.failedCount += int64(len(messages))
		p.failedMu.Unlock()

		// Log first message key for debugging
		key := ""
		if len(messages) > 0 {
			key = string(messages[0].Key)
		}
		p.log.Warnf("kafka async delivery failed: messages=%d key=%s err=%v",
			len(messages), key, err)
	}
}

// FailedCount returns the number of failed async deliveries since startup.
func (p *KafkaProducer) FailedCount() int64 {
	p.failedMu.RLock()
	defer p.failedMu.RUnlock()
	return p.failedCount
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
