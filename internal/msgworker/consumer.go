package msgworker

import (
	"context"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/segmentio/kafka-go"
)

// MessageHandler is the callback for processing consumed messages.
type MessageHandler func(ctx context.Context, key, value []byte, headers map[string]string) error

const (
	// retryHeaderKey tracks how many times a message has been retried.
	retryHeaderKey = "x-retry-count"
	// maxRetries is the maximum number of delivery attempts before sending to DLQ.
	maxRetries = 3
	// dlqTopicSuffix is appended to the original topic name to form the DLQ topic.

dlqTopicSuffix = "-dlq"
)

// KafkaConsumer consumes messages from Kafka.
type KafkaConsumer struct {
	reader    *kafka.Reader
	dlqWriter *kafka.Writer
	handler   MessageHandler
	log       *log.Helper
	stopCh    chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once

	// workerCount controls the number of concurrent message processors.
	// Messages from the same partition are always handled by the same worker,
	// preserving per-partition ordering.
	workerCount int
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
	WorkerCount    int   // number of concurrent workers; <=1 means sequential
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
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = runtime.NumCPU()
		if cfg.WorkerCount < 2 {
			cfg.WorkerCount = 2
		}
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

	// DLQ writer for poison messages.
	dlqWriter := &kafka.Writer{
		Addr:         kafka.TCP(cfg.Brokers...),
		Topic:        cfg.Topic + dlqTopicSuffix,
		Async:        false,
		WriteTimeout: 5 * time.Second,
		MaxAttempts:  3,
	}

	return &KafkaConsumer{
		reader:      reader,
		dlqWriter:   dlqWriter,
		handler:     handler,
		log:         log.NewHelper(logger),
		stopCh:      make(chan struct{}),
		workerCount: cfg.WorkerCount,
	}
}

// Start begins consuming messages.
// It spawns multiple worker goroutines and dispatches messages by partition so
// that each partition is processed sequentially by a single worker, while
// different partitions are processed concurrently.
func (c *KafkaConsumer) Start(ctx context.Context) error {
	c.startOnce.Do(func() {}) // signal that Start has been called

	workChs := make([]chan kafka.Message, c.workerCount)
	var wg sync.WaitGroup
	for i := 0; i < c.workerCount; i++ {
		workChs[i] = make(chan kafka.Message, 64)
		wg.Add(1)
		go func(ch chan kafka.Message) {
			defer wg.Done()
			c.workerLoop(ctx, ch)
		}(workChs[i])
	}

	// Close all worker channels when the read loop exits so workers shut down.
	defer func() {
		for _, ch := range workChs {
			close(ch)
		}
		wg.Wait()
	}()

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

		c.log.Debugf("consumed message: topic=%s partition=%d offset=%d key=%s",
			msg.Topic, msg.Partition, msg.Offset, string(msg.Key))

		workerID := msg.Partition % c.workerCount
		select {
		case workChs[workerID] <- msg:
		case <-ctx.Done():
			return ctx.Err()
		case <-c.stopCh:
			return nil
		}
	}
}

// workerLoop processes messages from a single work channel sequentially.
// Assigning the same partition to the same worker preserves ordering.
func (c *KafkaConsumer) workerLoop(ctx context.Context, ch chan kafka.Message) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			c.handleMessage(ctx, msg)
		}
	}
}

// handleMessage invokes the handler and routes failures to retry/DLQ.
func (c *KafkaConsumer) handleMessage(ctx context.Context, msg kafka.Message) {
	headers := make(map[string]string)
	for _, h := range msg.Headers {
		headers[h.Key] = string(h.Value)
	}

	handlerCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err := c.handler(handlerCtx, msg.Key, msg.Value, headers)
	cancel()

	if err != nil {
		c.handleProcessingError(ctx, msg, headers, err)
	}
}

// handleProcessingError routes failed messages to retry or DLQ based on retry count.
func (c *KafkaConsumer) handleProcessingError(ctx context.Context, msg kafka.Message, headers map[string]string, procErr error) {
	retryCount := getRetryCount(headers)

	c.log.Errorf("handle message failed: key=%s retry_count=%d err=%v",
		string(msg.Key), retryCount, procErr)

	if retryCount < maxRetries {
		// Re-produce to the same topic with incremented retry header.
		// Kafka will redeliver to the consumer group for retry.
		if err := c.produceRetry(ctx, msg, retryCount+1); err != nil {
			c.log.Errorf("retry produce failed: key=%s err=%v", string(msg.Key), err)
			// Fallback to DLQ if retry produce itself fails.
			c.sendToDLQ(ctx, msg, retryCount, procErr)
		}
		return
	}

	// Max retries exceeded — send to DLQ.
	c.sendToDLQ(ctx, msg, retryCount, procErr)
}

// produceRetry re-publishes a failed message to the original topic with an incremented retry header.
func (c *KafkaConsumer) produceRetry(ctx context.Context, msg kafka.Message, retryCount int) error {
	newHeaders := make([]kafka.Header, 0, len(msg.Headers)+1)
	for _, h := range msg.Headers {
		if h.Key != retryHeaderKey {
			newHeaders = append(newHeaders, h)
		}
	}
	newHeaders = append(newHeaders, kafka.Header{
		Key:   retryHeaderKey,
		Value: []byte(strconv.Itoa(retryCount)),
	})

	retryMsg := kafka.Message{
		Key:     msg.Key,
		Value:   msg.Value,
		Headers: newHeaders,
	}

	// Use the same topic for retries; consumer group will pick it up again.
	writer := &kafka.Writer{
		Addr:         c.dlqWriter.Addr,
		Topic:        c.reader.Config().Topic,
		Async:        false,
		WriteTimeout: 5 * time.Second,
		MaxAttempts:  3,
	}
	defer writer.Close()

	return writer.WriteMessages(ctx, retryMsg)
}

// sendToDLQ sends a poison message to the DLQ topic with error metadata.
func (c *KafkaConsumer) sendToDLQ(ctx context.Context, msg kafka.Message, retryCount int, procErr error) {
	dlqHeaders := make([]kafka.Header, 0, len(msg.Headers)+3)
	for _, h := range msg.Headers {
		dlqHeaders = append(dlqHeaders, h)
	}
	dlqHeaders = append(dlqHeaders,
		kafka.Header{Key: retryHeaderKey, Value: []byte(strconv.Itoa(retryCount))},
		kafka.Header{Key: "x-dlq-reason", Value: []byte(procErr.Error())},
		kafka.Header{Key: "x-dlq-time", Value: []byte(time.Now().UTC().Format(time.RFC3339))},
	)

	dlqMsg := kafka.Message{
		Key:     msg.Key,
		Value:   msg.Value,
		Headers: dlqHeaders,
	}

	if err := c.dlqWriter.WriteMessages(ctx, dlqMsg); err != nil {
		c.log.Errorf("send to DLQ failed: key=%s err=%v", string(msg.Key), err)
	} else {
		c.log.Warnf("message sent to DLQ: key=%s retry_count=%d", string(msg.Key), retryCount)
	}
}

// getRetryCount extracts the retry count from message headers.
func getRetryCount(headers map[string]string) int {
	if v, ok := headers[retryHeaderKey]; ok {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return 0
}

// SetHandler sets the message handler (useful when handler needs reference to the consumer's owner).
func (c *KafkaConsumer) SetHandler(handler MessageHandler) {
	c.handler = handler
}

// Stop stops the consumer.
func (c *KafkaConsumer) Stop() error {
	c.stopOnce.Do(func() {
		close(c.stopCh)
	})
	if err := c.reader.Close(); err != nil {
		_ = c.dlqWriter.Close()
		return err
	}
	return c.dlqWriter.Close()
}
