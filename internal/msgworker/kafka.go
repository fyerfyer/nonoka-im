package msgworker

import (
	"nonoka-im/internal/kafka"
)

// Kafka consumer names are kept as aliases for the msgworker assembly and
// integration fixtures. The implementation lives in internal/kafka so the
// agent worker and future services share the same DLQ/trace behavior.
type (
	KafkaConsumer       = kafka.Consumer
	KafkaConsumerConfig = kafka.ConsumerConfig
	MessageHandler      = kafka.MessageHandler
)

var NewKafkaConsumer = kafka.NewConsumer
