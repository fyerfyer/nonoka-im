package gateway

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/conf"
	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
)

// MessageProducer produces upstream messages to Kafka.
type MessageProducer interface {
	Produce(ctx context.Context, msg *v1.UpstreamMessage) error
	Close() error
	Healthy() bool
}

// KafkaConfig holds Kafka producer configuration.
type KafkaConfig struct {
	Brokers      []string
	Topic        string
	BatchSize    int
	BatchTimeout time.Duration
	RequiredAcks kafka.RequiredAcks
	Compression  kafka.Compression
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	MaxAttempts  int
	Async        bool // true for async mode, false for sync mode (default)
}

// KafkaProducer implements MessageProducer using kafka-go.
type KafkaProducer struct {
	writer      *kafka.Writer
	log         *log.Helper
	async       bool
	failedMu    sync.RWMutex
	failedCount int64
	lastErr     error
	lastErrTime time.Time
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
		cfg.WriteTimeout = 10 * time.Second
	}
	if cfg.MaxAttempts == 0 {
		cfg.MaxAttempts = 3
	}

	p := &KafkaProducer{
		log:   log.NewHelper(logger),
		async: cfg.Async,
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
		Completion:   p.onCompletion,
	}

	p.writer = writer
	p.log.Infof("kafka producer created: topic=%s async=%v acks=%d compression=%d",
		cfg.Topic, cfg.Async, cfg.RequiredAcks, cfg.Compression)
	return p
}

// onCompletion handles async delivery results from kafka-go.
func (p *KafkaProducer) onCompletion(messages []kafka.Message, err error) {
	if err != nil {
		p.failedMu.Lock()
		p.failedCount += int64(len(messages))
		p.lastErr = err
		p.lastErrTime = time.Now()
		p.failedMu.Unlock()

		// Log first message key for debugging
		key := ""
		if len(messages) > 0 {
			key = string(messages[0].Key)
		}
		p.log.Errorf("kafka async delivery failed: messages=%d key=%s err=%v",
			len(messages), key, err)
		return
	}
	p.failedMu.Lock()
	p.lastErr = nil
	p.failedMu.Unlock()
}

// FailedCount returns the number of failed async deliveries since startup.
func (p *KafkaProducer) FailedCount() int64 {
	p.failedMu.RLock()
	defer p.failedMu.RUnlock()
	return p.failedCount
}

// Healthy reports whether the producer is considered healthy.
// For sync producers this is best-effort based on the last async completion
// callback; callers should also treat Produce errors as unhealthy signals.
func (p *KafkaProducer) Healthy() bool {
	p.failedMu.RLock()
	defer p.failedMu.RUnlock()
	if p.lastErr == nil {
		return true
	}
	// Consider producer unhealthy if the last async delivery failed within the last minute.
	return time.Since(p.lastErrTime) > time.Minute
}

// Produce sends an upstream message to Kafka.
// Uses the message's Topic field as the Kafka message key for partition affinity.
// In sync mode (the default) this call blocks until the broker acknowledges the
// message or an error occurs. In async mode it returns once the message has been
// accepted into the local batch; delivery failures are tracked via Completion.
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

	// Newly created or recreated topics can briefly reject produces while
	// metadata propagates or partitions initialize. Retry those transient
	// errors; the write path is idempotent thanks to client_msg_id
	// deduplication downstream.
	for attempt := 1; ; attempt++ {
		err := p.writer.WriteMessages(ctx, kmsg)
		if err == nil {
			break
		}
		if attempt >= 5 || ctx.Err() != nil || !isTransientProduceError(err) {
			p.log.Errorf("write to kafka failed: topic=%s sender=%d client_msg_id=%s err=%v",
				msg.GetTopic(), msg.GetSenderId(), msg.GetClientMsgId(), err)
			return fmt.Errorf("write to kafka: %w", err)
		}
		time.Sleep(200 * time.Millisecond)
	}

	p.log.Debugf("produced message to kafka: topic=%s sender=%d client_msg_id=%s async=%v",
		msg.GetTopic(), msg.GetSenderId(), msg.GetClientMsgId(), p.async)
	return nil
}

// isTransientProduceError reports whether err is a transient broker-side
// produce failure worth retrying: topic metadata propagation and leader
// election windows surface these briefly after topic creation or recreation.
func isTransientProduceError(err error) bool {
	s := err.Error()
	return strings.Contains(s, "Unknown Topic Or Partition") ||
		strings.Contains(s, "Leader Not Available") ||
		strings.Contains(s, "Not Leader For Partition")
}

// Close closes the Kafka writer.
func (p *KafkaProducer) Close() error {
	return p.writer.Close()
}

// ParseCompression maps a compression string to kafka.Compression.
func ParseCompression(s string) kafka.Compression {
	switch strings.ToLower(s) {
	case "gzip":
		return kafka.Gzip
	case "snappy":
		return kafka.Snappy
	case "lz4":
		return kafka.Lz4
	case "zstd":
		return kafka.Zstd
	default:
		return kafka.Lz4
	}
}

// ParseRequiredAcks maps an acks string to kafka.RequiredAcks.
func ParseRequiredAcks(s string) kafka.RequiredAcks {
	switch strings.ToLower(s) {
	case "none":
		return kafka.RequireNone
	case "one":
		return kafka.RequireOne
	case "all":
		return kafka.RequireAll
	default:
		return kafka.RequireAll
	}
}

// KafkaConfigFromProto builds a gateway.KafkaConfig from the protobuf config.
func KafkaConfigFromProto(kc *conf.Data_Kafka) KafkaConfig {
	if kc == nil {
		return KafkaConfig{}
	}
	cfg := KafkaConfig{
		Brokers:      kc.GetBrokers(),
		Topic:        kc.GetTopic(),
		BatchSize:    int(kc.GetBatchSize()),
		Compression:  ParseCompression(kc.GetCompression()),
		RequiredAcks: ParseRequiredAcks(kc.GetRequiredAcks()),
		Async:        kc.GetAsync(),
		MaxAttempts:  int(kc.GetMaxAttempts()),
	}
	if kc.GetBatchTimeout() != nil {
		cfg.BatchTimeout = kc.GetBatchTimeout().AsDuration()
	}
	if kc.GetWriteTimeout() != nil {
		cfg.WriteTimeout = kc.GetWriteTimeout().AsDuration()
	}
	if kc.GetReadTimeout() != nil {
		cfg.ReadTimeout = kc.GetReadTimeout().AsDuration()
	}
	return cfg
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

// Healthy reports healthy for no-op producer.
func (p *NoopProducer) Healthy() bool {
	return true
}
