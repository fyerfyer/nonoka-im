package kafka

import (
	"context"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

// Publisher is a small synchronous producer used by services that publish
// low-volume control events (agent triggers, outbox relays). High-volume IM
// ingestion keeps its batching producer in the gateway.
type Publisher struct {
	writer *kafka.Writer
}

// NewPublisher creates a synchronous publisher for one topic.
func NewPublisher(brokers []string, topic string) *Publisher {
	return &Publisher{writer: &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.Murmur2Balancer{},
		RequiredAcks: kafka.RequireOne,
		Async:        false,
		BatchTimeout: 10 * time.Millisecond,
	}}
}

// Publish writes one message with the given key and headers.
func (p *Publisher) Publish(ctx context.Context, key, value []byte, headers ...kafka.Header) error {
	if p == nil || p.writer == nil {
		return fmt.Errorf("kafka publisher unavailable")
	}
	return p.writer.WriteMessages(ctx, kafka.Message{Key: key, Value: value, Headers: headers})
}

func (p *Publisher) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}
