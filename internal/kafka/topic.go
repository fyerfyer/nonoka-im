package kafka

import (
	"context"
	"errors"
	"fmt"

	"github.com/segmentio/kafka-go"
)

// TopicPartitionCount returns the broker-side partition count for a topic.
// It is a diagnostic used at startup so the configured `kafka.partitions` value
// cannot silently diverge from an auto-created topic with one partition.
func TopicPartitionCount(ctx context.Context, brokers []string, topic string) (int, error) {
	if len(brokers) == 0 || topic == "" {
		return 0, fmt.Errorf("brokers and topic are required")
	}
	client := &kafka.Client{Addr: kafka.TCP(brokers...)}
	res, err := client.Metadata(ctx, &kafka.MetadataRequest{
		Addr:   kafka.TCP(brokers[0]),
		Topics: []string{topic},
	})
	if err != nil {
		return 0, fmt.Errorf("read topic metadata: %w", err)
	}
	for _, t := range res.Topics {
		if t.Name != topic {
			continue
		}
		if t.Error != nil {
			return 0, fmt.Errorf("topic %s: %w", topic, t.Error)
		}
		return len(t.Partitions), nil
	}
	return 0, fmt.Errorf("topic %s not found", topic)
}

// IsUnknownTopic reports whether err means the broker has not auto-created the
// topic yet. Startup validation should warn rather than fail in that case.
func IsUnknownTopic(err error) bool {
	return errors.Is(err, kafka.UnknownTopicOrPartition)
}
