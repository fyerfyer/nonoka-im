// Package consumer is the Kafka adapter for the IM write pipeline. It maps
// wire messages to domain objects, invokes the application services and owns
// the process lifecycle exposed to Kratos.
package consumer

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/im/app"
	"nonoka-im/internal/im/domain"
	imgroup "nonoka-im/internal/im/group"
	mongostore "nonoka-im/internal/im/storage/mongo"
	"nonoka-im/internal/kafka"
	"nonoka-im/internal/metrics"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/proto"
)

// Worker consumes upstream messages and drives persistence + delivery.
type Worker struct {
	Consumer *kafka.Consumer
	Store    *mongostore.Store
	Messages *app.MessageService
	Push     *app.PushService
	Outbox   *app.OutboxService
	Metrics  *metrics.Metrics
	Log      *log.Helper

	PushWorkers int
	// Background loops (bot index refresh, push retry queue) are started with
	// the worker context and stop when it is canceled.
	Background []func(context.Context)

	// Each shard owns a subset of topics. Routing by topic keeps messages from
	// one conversation in order while preserving cross-topic concurrency.
	pushShards   []chan deliveryTask
	pushStopCh   chan struct{}
	pushStopOnce sync.Once
	pushWg       sync.WaitGroup
	handlerMu    sync.Mutex
	inflight     sync.WaitGroup
	started      atomic.Bool
	stopping     atomic.Bool
}

type deliveryTask struct {
	result app.HandleResult
}

// resetPushShards creates one ordered queue per push worker. It is called
// before any consumer handler can run.
func (w *Worker) resetPushShards() {
	shards := make([]chan deliveryTask, w.PushWorkers)
	for i := range shards {
		shards[i] = make(chan deliveryTask, 1024)
	}
	w.pushShards = shards
}

// pushShard routes a topic to a stable queue. A topic always lands on the same
// worker, so delivery order matches the persisted topic_seq order.
func (w *Worker) pushShard(topic string) chan deliveryTask {
	if len(w.pushShards) == 0 {
		return nil
	}
	const offset32 = 2166136261
	h := uint32(offset32)
	for i := 0; i < len(topic); i++ {
		h ^= uint32(topic[i])
		h *= 16777619
	}
	return w.pushShards[h%uint32(len(w.pushShards))]
}

func NewWorker(consumer *kafka.Consumer, messages *app.MessageService, push *app.PushService, outbox *app.OutboxService, logger log.Logger, m ...*metrics.Metrics) *Worker {
	w := &Worker{
		Consumer: consumer, Messages: messages, Push: push, Outbox: outbox,
		pushStopCh: make(chan struct{}), Log: log.NewHelper(logger),
	}
	w.PushWorkers = runtime.NumCPU() * 2
	if w.PushWorkers < 4 {
		w.PushWorkers = 4
	}
	if w.PushWorkers > 64 {
		w.PushWorkers = 64
	}
	w.resetPushShards()
	if len(m) > 0 {
		w.Metrics = m[0]
	}
	if consumer != nil {
		consumer.SetHandler(w.HandleMessage)
	}
	return w
}

// Start implements transport.Server. It starts background loops and blocks on
// the Kafka consumer until the context is canceled.
func (w *Worker) Start(ctx context.Context) error {
	if w.stopping.Load() {
		return nil
	}
	w.started.Store(true)
	w.Log.Info("msgworker started")

	if len(w.pushShards) != w.PushWorkers {
		w.resetPushShards()
	}
	if w.Push != nil {
		w.Push.Start(ctx)
	}
	if w.Outbox != nil {
		go w.Outbox.Run(ctx, 250*time.Millisecond)
	}
	for _, run := range w.Background {
		go run(ctx)
	}
	for i := range w.pushShards {
		w.pushWg.Add(1)
		go w.pushLoop(ctx, w.pushShards[i])
	}
	if w.Consumer == nil {
		return nil
	}
	return w.Consumer.Start(ctx)
}

// Stop implements transport.Server. It drains in-flight work.
func (w *Worker) Stop(ctx context.Context) error {
	w.handlerMu.Lock()
	if w.stopping.Load() {
		w.handlerMu.Unlock()
		return nil
	}
	// Set stopping while holding the same gate used by HandleMessage, so no
	// handler can Add to inflight after Stop starts waiting.
	w.stopping.Store(true)
	w.handlerMu.Unlock()
	w.Log.Info("msgworker stopping")
	if w.Consumer != nil {
		if err := w.Consumer.Stop(); err != nil {
			w.Log.Warnf("stop consumer: %v", err)
		}
	}
	// The Kafka reader may have handed a message to HandleMessage just before
	// Stop. Wait for those handlers before waking push workers; otherwise a
	// handler can enqueue after the queue owners have exited.
	done := make(chan struct{})
	go func() {
		w.inflight.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		w.Log.Warn("msgworker stop grace elapsed while handlers are still running")
	}
	w.pushStopOnce.Do(func() { close(w.pushStopCh) })
	w.pushWg.Wait()
	if w.Push != nil {
		w.Push.Stop()
	}
	return nil
}

// Ready reports whether the worker has started and its Kafka consumer has
// connected (or already delivered a message).
func (w *Worker) Ready() bool {
	return w.started.Load() && w.Consumer != nil && w.Consumer.Ready()
}

func (w *Worker) pushLoop(ctx context.Context, shard chan deliveryTask) {
	defer w.pushWg.Done()
	deliver := func(task deliveryTask) {
		defer func() {
			if r := recover(); r != nil {
				w.Log.Errorf("push panic recovered: %v", r)
			}
		}()
		pushCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if w.Push != nil {
			w.Push.Deliver(pushCtx, task.result)
		}
		cancel()
	}
	for {
		select {
		case <-w.pushStopCh:
			// Drain this shard for a bounded grace period. Persisted messages
			// remain recoverable through pull if delivery cannot finish.
			deadline := time.NewTimer(5 * time.Second)
			defer deadline.Stop()
			for {
				select {
				case task := <-shard:
					deliver(task)
				case <-deadline.C:
					w.Log.Warn("push shard drain grace elapsed; remaining deliveries will be pulled")
					return
				default:
					return
				}
			}
		case <-ctx.Done():
			return
		case task := <-shard:
			deliver(task)
		}
	}
}

// HandleMessage is the Kafka message callback.
func (w *Worker) HandleMessage(ctx context.Context, key, value []byte, headers map[string]string) (err error) {
	start := time.Now()
	w.handlerMu.Lock()
	if w.stopping.Load() {
		w.handlerMu.Unlock()
		// Do not acknowledge a message that was read during shutdown. The
		// consumer has not committed it yet, so it will be redelivered.
		return context.Canceled
	}
	w.inflight.Add(1)
	w.handlerMu.Unlock()
	defer w.inflight.Done()
	defer func() {
		if r := recover(); r != nil {
			w.Log.Errorf("HandleMessage panic recovered: %v", r)
			err = fmt.Errorf("panic recovered: %v", r)
		}
	}()
	var upstream v1.UpstreamMessage
	if err = proto.Unmarshal(value, &upstream); err != nil {
		return fmt.Errorf("unmarshal upstream message: %w", err)
	}
	msg := domain.Message{
		Topic: upstream.GetTopic(), SenderID: upstream.GetSenderId(), Type: domain.MessageType(upstream.GetMsgType()),
		Content: upstream.GetContent(), Timestamp: upstream.GetTimestamp(), ClientMsgID: upstream.GetClientMsgId(),
		MentionedUserIDs: upstream.GetMentionedUserIds(),
	}
	result, err := w.Messages.HandleIncoming(ctx, msg)
	if err != nil {
		return err
	}
	if result.Duplicate {
		return nil
	}
	shard := w.pushShard(result.Message.Topic)
	if shard == nil {
		w.Log.Warnf("push shard unavailable, delivery dropped: msg_id=%d", result.Message.ID)
		return nil
	}
	select {
	case shard <- deliveryTask{result: result}:
	case <-w.pushStopCh:
		// The message is already durable. Pull/offline sync is the recovery
		// path when a shutdown races the push handoff.
		w.Log.Debugf("push queue stopped; delivery remains available via pull: msg_id=%d", result.Message.ID)
	}
	w.Log.Debugf("message persisted: msg_id=%d topic=%s seq=%d elapsed=%s",
		result.Message.ID, result.Message.Topic, result.Message.TopicSeq, time.Since(start))
	return nil
}

// MessagesService exposes the application message service for integration tests.
func (w *Worker) MessagesService() *app.MessageService { return w.Messages }

// Storage exposes the MongoDB adapter for integration tests.
func (w *Worker) Storage() *mongostore.Store { return w.Store }

// SetBotIndex wires the bot directory into persistence and push usecases.
func (w *Worker) SetBotIndex(index *imgroup.BotIndex) {
	if w.Messages != nil {
		w.Messages.Bots = index
	}
	if w.Push != nil {
		w.Push.Bots = index
	}
}

// SetGroupMemberService wires group membership into persistence and push.
func (w *Worker) SetGroupMemberService(svc app.GroupMemberReader) {
	if w.Messages != nil {
		w.Messages.GroupMembers = svc
	}
	if w.Push != nil {
		w.Push.GroupMembers = svc
	}
}

// SetConversationRepo wires conversation summaries into persistence.
func (w *Worker) SetConversationRepo(repo app.ConversationRepository) {
	if w.Messages != nil {
		w.Messages.Conversations = repo
	}
}
