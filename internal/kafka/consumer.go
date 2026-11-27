package kafka

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
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
	retryHeaderKey = RetryCountHeader
	// dlqTopicSuffix is appended to the original topic name to form the DLQ topic.
	dlqTopicSuffix = DLQSuffix
	// defaultHandlerTimeout is the per-attempt timeout for message handlers.
	defaultHandlerTimeout = 10 * time.Second
	// defaultRetryBackoff is the base backoff between local retry attempts.
	defaultRetryBackoff = 100 * time.Millisecond
	// defaultCommitBatchSize is the number of messages to batch before committing offsets.
	defaultCommitBatchSize = 100
	// defaultCommitInterval is the maximum time between offset commits.
	defaultCommitInterval = 1 * time.Second
)

// Consumer consumes messages from Kafka.
type Consumer struct {
	reader    *kafka.Reader
	dlqWriter *kafka.Writer
	handler   MessageHandler
	log       *log.Helper
	stopCh    chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	started   atomic.Bool
	stopped   atomic.Bool
	ready     atomic.Bool

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

	// commitBatchSize is the number of messages to batch before committing offsets.
	commitBatchSize int

	// commitFlushInterval is the maximum time between offset commits.
	commitFlushInterval time.Duration
}

// ConsumerConfig holds consumer configuration.
type ConsumerConfig struct {
	Brokers             []string
	Topic               string
	GroupID             string
	MinBytes            int
	MaxBytes            int
	MaxWait             time.Duration
	CommitInterval      time.Duration // 0 means manual commit
	StartOffset         int64         // kafka.FirstOffset or kafka.LastOffset
	WorkerCount         int           // number of concurrent workers; <=1 means sequential
	MaxRetries          int           // local retry attempts before DLQ; 0 means DLQ on first failure
	HandlerTimeout      time.Duration // per-attempt handler timeout
	RetryBackoff        time.Duration // base backoff between local retries
	CommitBatchSize     int           // number of messages to batch before committing offsets
	CommitFlushInterval time.Duration // maximum time between offset commits
	DLQTopic            string        // explicit dead-letter topic; defaults to <topic>-dlq
}

// NewConsumer creates a new Kafka consumer.
func NewConsumer(cfg ConsumerConfig, handler MessageHandler, logger log.Logger) *Consumer {
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
		log.NewHelper(logger).Infof("kafka.Consumer ignores CommitInterval; manual commit is enforced")
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
	if cfg.CommitBatchSize <= 0 {
		cfg.CommitBatchSize = defaultCommitBatchSize
	}
	if cfg.CommitFlushInterval <= 0 {
		cfg.CommitFlushInterval = defaultCommitInterval
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
	dlqTopic := cfg.DLQTopic
	if dlqTopic == "" {
		dlqTopic = cfg.Topic + dlqTopicSuffix
	}
	dlqWriter := &kafka.Writer{
		Addr:         kafka.TCP(cfg.Brokers...),
		Topic:        dlqTopic,
		Async:        false,
		Compression:  kafka.Lz4,
		WriteTimeout: 5 * time.Second,
		MaxAttempts:  3,
	}

	return &Consumer{
		reader:              reader,
		dlqWriter:           dlqWriter,
		handler:             handler,
		log:                 log.NewHelper(logger),
		stopCh:              make(chan struct{}),
		workerCount:         cfg.WorkerCount,
		maxRetries:          cfg.MaxRetries,
		handlerTimeout:      cfg.HandlerTimeout,
		retryBackoff:        cfg.RetryBackoff,
		commitBatchSize:     cfg.CommitBatchSize,
		commitFlushInterval: cfg.CommitFlushInterval,
	}
}

// Start begins consuming messages.
// It spawns multiple worker goroutines and dispatches messages by partition so
// that each partition is processed sequentially by a single worker, while
// different partitions are processed concurrently.
// Start returns an error if called more than once.
func (c *Consumer) Start(ctx context.Context) error {
	if c.started.Load() {
		return fmt.Errorf("kafka consumer already started")
	}
	c.startOnce.Do(func() {
		c.started.Store(true)
	})

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

		// FetchMessage never commits. The worker owns the commit after the
		// handler succeeds (or the message is safely parked in the DLQ).
		// ReadMessage would commit here, before the handler runs, which turns
		// every handler failure or process crash into silent message loss.
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil || c.isStopped() || isShutdownError(err) {
				return nil
			}
			c.log.Errorf("read message error: %v", err)
			continue
		}

		c.ready.Store(true)
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
// Offsets are committed in batches to reduce Kafka round-trips.
// Started reports whether Start has been called.
func (c *Consumer) Started() bool { return c.started.Load() }

// Ready reports whether the consumer has connected to Kafka. It must not wait
// for the first message: an idle topic would otherwise keep the worker
// permanently unready. Fetches becomes non-zero after the reader reaches the
// broker, even when the topic is empty.
func (c *Consumer) Ready() bool {
	if !c.started.Load() {
		return false
	}
	if c.ready.Load() {
		return true
	}
	return c.reader != nil && c.reader.Stats().Fetches > 0
}

func (c *Consumer) workerLoop(ctx context.Context, ch chan kafka.Message) {
	defer func() {
		if r := recover(); r != nil {
			c.log.Errorf("worker panic recovered: %v", r)
		}
	}()

	pending := make([]kafka.Message, 0, c.commitBatchSize)
	commit := func(commitCtx context.Context) error {
		if len(pending) == 0 {
			return nil
		}
		if err := c.reader.CommitMessages(commitCtx, pending...); err != nil {
			if c.isStopped() || isShutdownError(err) {
				c.log.Debugf("commit offsets skipped during shutdown: count=%d err=%v", len(pending), err)
			} else {
				c.log.Errorf("commit offsets failed; keeping offsets pending: count=%d err=%v", len(pending), err)
			}
			// Never advance past an uncommitted message. Committing a later
			// batch would implicitly skip this range and silently lose it.
			return err
		}
		c.log.Debugf("committed offsets: count=%d", len(pending))
		pending = pending[:0]
		return nil
	}
	commitOnShutdown := func() {
		commitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = commit(commitCtx)
	}

	ticker := time.NewTicker(c.commitFlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			commitOnShutdown()
			return
		case <-c.stopCh:
			commitOnShutdown()
			return
		case <-ticker.C:
			commit(ctx)
		case msg, ok := <-ch:
			if !ok {
				commitOnShutdown()
				return
			}
			// Only append to pending commits if the message was fully processed
			// (successfully or moved to DLQ). If processing was interrupted by
			// shutdown/context cancellation, do not commit so it will be
			// redelivered. Stop the loop: continuing could process a later
			// offset and commit past the skipped message.
			if !c.handleMessage(ctx, msg) {
				commitOnShutdown()
				return
			}
			pending = append(pending, msg)
			if len(pending) >= c.commitBatchSize {
				commit(ctx)
			}
		}
	}
}

// isStopped reports whether Stop has been called.
func (c *Consumer) isStopped() bool { return c.stopped.Load() }

// isShutdownError reports errors that are expected when the reader is closed
// during a normal shutdown, so they do not pollute error logs/alerts.
func isShutdownError(err error) bool {
	return err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, io.ErrClosedPipe) || errors.Is(err, io.EOF)
}

// isTransientHandlerError classifies infrastructure timeouts separately from
// poison messages. Handlers wrap database/network calls with the per-attempt
// context, so a failover longer than HandlerTimeout appears here as a
// DeadlineExceeded error.
func isTransientHandlerError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary())
}

// handleMessage invokes the handler with local retries and routes persistent
// failures to DLQ. Local retries keep the message in the same partition order
// and do not re-produce it to the original topic, avoiding sequence gaps.
// It returns true if the message was fully processed (success or DLQ) and
// false if processing was interrupted by shutdown/context cancellation.
func (c *Consumer) handleMessage(ctx context.Context, msg kafka.Message) bool {
	headers := make(map[string]string)
	for _, h := range msg.Headers {
		headers[h.Key] = string(h.Value)
	}

	var lastErr error
	attempts := c.maxRetries + 1
	for attempt := 0; attempt < attempts; attempt++ {
		handlerCtx, cancel := context.WithTimeout(ctx, c.handlerTimeout)
		lastErr = c.invokeHandler(handlerCtx, msg.Key, msg.Value, headers)
		cancel()
		if lastErr == nil {
			return true
		}
		if c.isStopped() || (errors.Is(lastErr, context.Canceled) && ctx.Err() != nil) {
			// The handler refused the message because shutdown started. Leave
			// the offset uncommitted so Kafka redelivers it after restart.
			return false
		}
		if isTransientHandlerError(lastErr) {
			// Mongo/Redis failovers often surface as deadline/network errors.
			// Treating those as poison after three sub-second retries moves
			// valid traffic to the DLQ, which no worker consumes.
			c.log.Warnf("transient handler error; retrying without consuming poison budget: key=%s err=%v", string(msg.Key), lastErr)
			backoff := c.retryBackoff
			if backoff < time.Second {
				backoff = time.Second
			}
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			select {
			case <-time.After(backoff):
				continue
			case <-ctx.Done():
				return false
			case <-c.stopCh:
				return false
			}
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
				return false
			case <-c.stopCh:
				c.log.Warnf("consumer stopped during retry backoff: key=%s", string(msg.Key))
				return false
			}
		}
	}

	if err := c.handleProcessingError(ctx, msg, headers, lastErr); err != nil {
		return false
	}
	return true
}

// invokeHandler contains a panic from one message so the worker remains
// alive. Letting it escape the goroutine would silently kill that partition's
// consumer and stall the reader when its queue fills.
func (c *Consumer) invokeHandler(ctx context.Context, key, value []byte, headers map[string]string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("handler panic: %v", r)
		}
	}()
	return c.handler(ctx, key, value, headers)
}

// handleProcessingError routes failed messages to DLQ.
// It returns the DLQ send error so the caller can decide whether to commit offset.
func (c *Consumer) handleProcessingError(ctx context.Context, msg kafka.Message, headers map[string]string, procErr error) error {
	retryCount := getRetryCount(headers)
	c.log.Errorf("handle message failed after retries: key=%s retry_count=%d err=%v",
		string(msg.Key), retryCount, procErr)
	return c.sendToDLQ(ctx, msg, retryCount, procErr)
}

// sendToDLQ sends a poison message to the DLQ topic with error metadata.
// It returns any error encountered while writing to DLQ.
func (c *Consumer) sendToDLQ(ctx context.Context, msg kafka.Message, retryCount int, procErr error) error {
	dlqHeaders := make([]kafka.Header, 0, len(msg.Headers)+3)
	dlqHeaders = append(dlqHeaders, msg.Headers...)
	dlqHeaders = append(dlqHeaders,
		kafka.Header{Key: RetryCountHeader, Value: []byte(strconv.Itoa(retryCount))},
		kafka.Header{Key: DLQReasonHeader, Value: []byte(procErr.Error())},
		kafka.Header{Key: DLQTimeHeader, Value: []byte(time.Now().UTC().Format(time.RFC3339))},
	)

	dlqMsg := kafka.Message{
		Key:     msg.Key,
		Value:   msg.Value,
		Headers: dlqHeaders,
	}

	for {
		err := c.dlqWriter.WriteMessages(ctx, dlqMsg)
		if err == nil {
			c.log.Warnf("message sent to DLQ: key=%s retry_count=%d", string(msg.Key), retryCount)
			return nil
		}
		c.log.Errorf("send to DLQ failed; will retry: key=%s err=%v", string(msg.Key), err)
		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
			return ctx.Err()
		case <-c.stopCh:
			return fmt.Errorf("consumer stopped while sending to DLQ")
		}
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
func (c *Consumer) SetHandler(handler MessageHandler) {
	c.handler = handler
}

// Stop stops the consumer.
func (c *Consumer) Stop() error {
	c.stopped.Store(true)
	c.stopOnce.Do(func() {
		close(c.stopCh)
	})
	if err := c.reader.Close(); err != nil {
		_ = c.dlqWriter.Close()
		return err
	}
	return c.dlqWriter.Close()
}
