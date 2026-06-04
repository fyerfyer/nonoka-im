package msgworker

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/segmentio/kafka-go"
)

// MessageHandler is the callback for processing consumed messages.
type MessageHandler func(ctx context.Context, key, value []byte, headers map[string]string) error

// KafkaConsumer consumes messages from Kafka.
type KafkaConsumer struct {
	reader   *kafka.Reader
	handler  MessageHandler
	log      *log.Helper
	stopCh   chan struct{}
	stopOnce chan struct{}
}

// KafkaConsumerConfig holds consumer configuration.
type KafkaConsumerConfig struct {
	Brokers        []string
	Topic          string
	GroupID        string
	MinBytes       int
	MaxBytes       int
	MaxWait        time.Duration
	CommitInterval time.Duration
	StartOffset    int64 // kafka.FirstOffset or kafka.LastOffset
}

// NewKafkaConsumer creates a new Kafka consumer.
func NewKafkaConsumer(cfg KafkaConsumerConfig, handler MessageHandler, logger log.Logger) *KafkaConsumer {
	if cfg.MinBytes == 0 {
		cfg.MinBytes = 1
	}
	if cfg.MaxBytes == 0 {
		cfg.MaxBytes = 10e6 // 10MB
	}
	if cfg.MaxWait == 0 {
		cfg.MaxWait = 500 * time.Millisecond
	}
	if cfg.CommitInterval == 0 {
		cfg.CommitInterval = 1 * time.Second
	}

	readerCfg := kafka.ReaderConfig{
		Brokers:        cfg.Brokers,
		Topic:          cfg.Topic,
		GroupID:        cfg.GroupID,
		MinBytes:       cfg.MinBytes,
		MaxBytes:       cfg.MaxBytes,
		MaxWait:        cfg.MaxWait,
		CommitInterval: cfg.CommitInterval,
	}
	if cfg.StartOffset != 0 {
		readerCfg.StartOffset = cfg.StartOffset
	}

	reader := kafka.NewReader(readerCfg)

	return &KafkaConsumer{
		reader:   reader,
		handler:  handler,
		log:      log.NewHelper(logger),
		stopCh:   make(chan struct{}),
		stopOnce: make(chan struct{}),
	}
}

// Start begins consuming messages in a blocking loop.
func (c *KafkaConsumer) Start(ctx context.Context) error {
	close(c.stopOnce) // signal that Start has been called
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.stopCh:
			return nil
		default:
		}

		msg, err := c.reader.ReadMessage(ctx)
		if err != nil {
			if err == context.Canceled || err == context.DeadlineExceeded {
				return nil
			}
			c.log.Errorf("read message error: %v", err)
			continue
		}

		headers := make(map[string]string)
		for _, h := range msg.Headers {
			headers[h.Key] = string(h.Value)
		}

		c.log.Debugf("consumed message: topic=%s partition=%d offset=%d key=%s",
			msg.Topic, msg.Partition, msg.Offset, string(msg.Key))

		handlerCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err := c.handler(handlerCtx, msg.Key, msg.Value, headers); err != nil {
			c.log.Errorf("handle message failed: key=%s err=%v", string(msg.Key), err)
		}
		cancel()
	}
}

// SetHandler sets the message handler (useful when handler needs reference to the consumer's owner).
func (c *KafkaConsumer) SetHandler(handler MessageHandler) {
	c.handler = handler
}

// Stop stops the consumer.
func (c *KafkaConsumer) Stop() error {
	close(c.stopCh)
	return c.reader.Close()
}
