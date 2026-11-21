// Package kafka adapts the shared Kafka publisher to the messaging event
// contract.
package kafka

import (
	"context"

	"nonoka-im/internal/kafka"
	"nonoka-im/internal/messaging"

	segmentio "github.com/segmentio/kafka-go"
)

const Topic = "agent-events"

type Publisher struct {
	publisher *kafka.Publisher
}

func NewPublisher(brokers []string, topic string) *Publisher {
	if topic == "" {
		topic = Topic
	}
	return &Publisher{publisher: kafka.NewPublisher(brokers, topic)}
}

func (p *Publisher) Publish(ctx context.Context, e messaging.MessageEvent) error {
	b, err := e.Marshal()
	if err != nil {
		return err
	}
	headers := []segmentio.Header{{Key: "content-type", Value: []byte("application/json")}}
	if traceparent := kafka.TraceParent(ctx); traceparent != "" {
		headers = append(headers, segmentio.Header{Key: kafka.TraceParentHeader, Value: []byte(traceparent)})
	}
	return p.publisher.Publish(ctx, []byte(e.Topic), b, headers...)
}

func (p *Publisher) Close() error { return p.publisher.Close() }

var _ messaging.EventPublisher = (*Publisher)(nil)
