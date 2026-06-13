package msgworker

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/segmentio/kafka-go"
)

// MessageHandler is the callback for processing consumed messages.
type MessageHandler func(ctx context.Context, key, value []byte, headers map[string]string) error

const (
	// retryHeaderKey tracks how many times a message has been retried.
	retryHeaderKey = "x-retry-count"
	// dlqTopicSuffix is appended to the original topic name to form the DLQ topic.
	dlqTopicSuffix = "-dlq"
	// defaultHandlerTimeout is the per-attempt timeout for message handlers.
	defaultHandlerTimeout = 10 * time.Second
	// defaultRetryBackoff is the base backoff between local retry attempts.
	defaultRetryBackoff = 100 * time.Millisecond
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
	started   atomic.Bool

	// workerCount controls the number of concurrent message processors.
	// Messages from the same partition are always handled by the same worker,
	// preserving per-partition ordering.
	workerCount int

	// maxRetries is the number of local retry attempts before sending to DLQ.
	maxRetries int

	// handlerTimeout is the timeout for each handler invocation.
	handlerTimeout time.Duration

	// retryBackoff is the base backoff duration between local retries.
	retryBackoff time.Duration
}

// KafkaConsumerConfig holds consumer configuration.
type KafkaConsumerConfig struct {
	Brokers        []string
	Topic          string
	GroupID        string
	MinBytes       int
	MaxBytes       int
	MaxWait        time.Duration
	CommitInterval time.Duration // 0 means manual commit
	StartOffset    int64         // kafka.FirstOffset or kafka.LastOffset
	WorkerCount    int           // number of concurrent workers; <=1 means sequential
	MaxRetries     int           // local retry attempts before DLQ; 0 means DLQ on first failure
	HandlerTimeout time.Duration // per-attempt handler timeout
	RetryBackoff   time.Duration // base backoff between local retries
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
	if cfg.CommitInterval != 0 {
		// Manual commit is enforced to preserve ordering and exactly-once-ish
		// semantics: we only commit after the handler succeeds or the message
		// lands in DLQ. Inform the caller if a non-zero interval was supplied.
		log.NewHelper(logger).Infof("KafkaConsumer ignores CommitInterval; manual commit is enforced")
	}
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = runtime.NumCPU()
		if cfg.WorkerCount < 2 {
			cfg.WorkerCount = 2
		}
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 0
	}
	if cfg.HandlerTimeout <= 0 {
		cfg.HandlerTimeout = defaultHandlerTimeout
	}
	if cfg.RetryBackoff <= 0 {
		cfg.RetryBackoff = defaultRetryBackoff
	}

	readerCfg := kafka.ReaderConfig{
		Brokers:        cfg.Brokers,
		Topic:          cfg.Topic,
		GroupID:        cfg.GroupID,
		MinBytes:       cfg.MinBytes,
		MaxBytes:       cfg.MaxBytes,
		MaxWait:        cfg.MaxWait,
		CommitInterval: 0, // manual commit
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
		Compression:  kafka.Lz4,
		WriteTimeout: 5 * time.Second,
		MaxAttempts:  3,
	}

	return &KafkaConsumer{
		reader:         reader,
		dlqWriter:      dlqWriter,
		handler:        handler,
		log:            log.NewHelper(logger),
		stopCh:         make(chan struct{}),
		workerCount:    cfg.WorkerCount,
		maxRetries:     cfg.MaxRetries,
		handlerTimeout: cfg.HandlerTimeout,
		retryBackoff:   cfg.RetryBackoff,
	}
}

// Start begins consuming messages.
// It spawns multiple worker goroutines and dispatches messages by partition so
// that each partition is processed sequentially by a single worker, while
// different partitions are processed concurrently.
// Start returns an error if called more than once.
func (c *KafkaConsumer) Start(ctx context.Context) error {
	var alreadyStarted bool
	c.startOnce.Do(func() {
		alreadyStarted = c.started.Load()
		if alreadyStarted {
			return
		}
		c.started.Store(true)
	})
	if alreadyStarted {
		return fmt.Errorf("kafka consumer already started")
	}

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
	defer func() {
		if r := recover(); r != nil {
			c.log.Errorf("worker panic recovered: %v", r)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			c.handleMessage(ctx, msg)
			// Manual commit: only commit after the message has been processed
			// (successfully or moved to DLQ). This keeps per-partition ordering
			// and avoids losing messages on restart.
			if err := c.reader.CommitMessages(ctx, msg); err != nil {
				c.log.Errorf("commit offset failed: partition=%d offset=%d err=%v",
					msg.Partition, msg.Offset, err)
			}
		}
	}
}

// handleMessage invokes the handler with local retries and routes persistent
// failures to DLQ. Local retries keep the message in the same partition order
// and do not re-produce it to the original topic, avoiding sequence gaps.
func (c *KafkaConsumer) handleMessage(ctx context.Context, msg kafka.Message) {
	headers := make(map[string]string)
	for _, h := range msg.Headers {
		headers[h.Key] = string(h.Value)
	}

	var lastErr error
	attempts := c.maxRetries + 1
	for attempt := 0; attempt < attempts; attempt++ {
		handlerCtx, cancel := context.WithTimeout(ctx, c.handlerTimeout)
		lastErr = c.handler(handlerCtx, msg.Key, msg.Value, headers)
		cancel()
		if lastErr == nil {
			return
		}

		c.log.Warnf("handle message failed: attempt=%d/%d key=%s err=%v",
			attempt+1, attempts, string(msg.Key), lastErr)

		if attempt < c.maxRetries {
			// Exponential backoff: 100ms, 200ms, 400ms, ... capped at 5s.
			backoff := c.retryBackoff * time.Duration(1<<attempt)
			const maxBackoff = 5 * time.Second
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				c.log.Warnf("context canceled during retry backoff: key=%s", string(msg.Key))
				return
			case <-c.stopCh:
				c.log.Warnf("consumer stopped during retry backoff: key=%s", string(msg.Key))
				return
			}
		}
	}

	c.handleProcessingError(ctx, msg, headers, lastErr)
}

// handleProcessingError routes failed messages to DLQ.
func (c *KafkaConsumer) handleProcessingError(ctx context.Context, msg kafka.Message, headers map[string]string, procErr error) {
	retryCount := getRetryCount(headers)
	c.log.Errorf("handle message failed after retries: key=%s retry_count=%d err=%v",
		string(msg.Key), retryCount, procErr)
	c.sendToDLQ(ctx, msg, retryCount, procErr)
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
