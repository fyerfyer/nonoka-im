package kafka

// Dead-letter conventions shared by every Kafka consumer in the project.
// The original key and value are preserved; metadata travels in headers so a
// replay can restore the message without guessing its origin.
const (
	DLQSuffix        = "-dlq"
	RetryCountHeader = "x-retry-count"
	DLQReasonHeader  = "x-dlq-reason"
	DLQTimeHeader    = "x-dlq-time"
)
