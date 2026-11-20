package event

import (
	"context"

	kafkax "nonoka-im/internal/kafka"

	"github.com/segmentio/kafka-go"
)

const Topic = "agent-events"

type KafkaPublisher struct{ publisher *kafkax.Publisher }

func NewKafkaPublisher(brokers []string, topic string) *KafkaPublisher {
	if topic == "" {
		topic = Topic
	}
	return &KafkaPublisher{publisher: kafkax.NewPublisher(brokers, topic)}
}

func (p *KafkaPublisher) Publish(ctx context.Context, e MessageEvent) error {
	b, err := e.Marshal()
	if err != nil {
		return err
	}
	headers := []kafka.Header{{Key: "content-type", Value: []byte("application/json")}}
	if traceparent := kafkax.TraceParent(ctx); traceparent != "" {
		headers = append(headers, kafka.Header{Key: kafkax.TraceParentHeader, Value: []byte(traceparent)})
	}
	return p.publisher.Publish(ctx, []byte(e.Topic), b, headers...)
}

func (p *KafkaPublisher) Close() error { return p.publisher.Close() }

var _ Publisher = (*KafkaPublisher)(nil)
