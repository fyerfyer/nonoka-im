package event

import (
	"context"

	"github.com/segmentio/kafka-go"
)

const Topic = "agent-events"

type KafkaPublisher struct{ writer *kafka.Writer }

func NewKafkaPublisher(brokers []string, topic string) *KafkaPublisher {
	if topic == "" {
		topic = Topic
	}
	return &KafkaPublisher{writer: &kafka.Writer{Addr: kafka.TCP(brokers...), Topic: topic, Balancer: &kafka.Hash{}, RequiredAcks: kafka.RequireOne, Async: false}}
}
func (p *KafkaPublisher) Publish(ctx context.Context, e MessageEvent) error {
	b, err := e.Marshal()
	if err != nil {
		return err
	}
	return p.writer.WriteMessages(ctx, kafka.Message{Key: []byte(e.Topic), Value: b, Headers: []kafka.Header{{Key: "content-type", Value: []byte("application/json")}}})
}
func (p *KafkaPublisher) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}

var _ Publisher = (*KafkaPublisher)(nil)
