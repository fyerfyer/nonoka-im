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

	pushPool     chan deliveryTask
	pushStopCh   chan struct{}
	pushStopOnce sync.Once
	pushWg       sync.WaitGroup
	started      atomic.Bool
	stopping     atomic.Bool
}

type deliveryTask struct {
	result app.HandleResult
}

func NewWorker(consumer *kafka.Consumer, messages *app.MessageService, push *app.PushService, outbox *app.OutboxService, logger log.Logger, m ...*metrics.Metrics) *Worker {
	w := &Worker{
		Consumer: consumer, Messages: messages, Push: push, Outbox: outbox,
		pushPool: make(chan deliveryTask, 1024), pushStopCh: make(chan struct{}),
		Log: log.NewHelper(logger),
	}
	w.PushWorkers = runtime.NumCPU() * 2
	if w.PushWorkers < 4 {
		w.PushWorkers = 4
	}
	if w.PushWorkers > 64 {
		w.PushWorkers = 64
	}
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

	if w.Push != nil {
		w.Push.Start(ctx)
	}
	if w.Outbox != nil {
		go w.Outbox.Run(ctx, 250*time.Millisecond)
	}
	for _, run := range w.Background {
		go run(ctx)
	}
	for i := 0; i < w.PushWorkers; i++ {
		w.pushWg.Add(1)
		go w.pushLoop(ctx)
	}
	if w.Consumer == nil {
		return nil
	}
	return w.Consumer.Start(ctx)
}

// Stop implements transport.Server. It drains in-flight work.
func (w *Worker) Stop(ctx context.Context) error {
	if !w.stopping.CompareAndSwap(false, true) {
		return nil
	}
	w.Log.Info("msgworker stopping")
	if w.Consumer != nil {
		if err := w.Consumer.Stop(); err != nil {
			w.Log.Warnf("stop consumer: %v", err)
		}
	}
	w.pushStopOnce.Do(func() {
		close(w.pushStopCh)
		close(w.pushPool)
	})
	w.pushWg.Wait()
	if w.Push != nil {
		w.Push.Stop()
	}
	return nil
}

// Ready reports whether the Kafka consumer has received its first message.
func (w *Worker) Ready() bool {
	return w.started.Load() && w.Consumer != nil && w.Consumer.Started()
}

func (w *Worker) pushLoop(ctx context.Context) {
	defer w.pushWg.Done()
	for {
		select {
		case <-w.pushStopCh:
			return
		case <-ctx.Done():
			return
		case task, ok := <-w.pushPool:
			if !ok {
				return
			}
			pushCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if w.Push != nil {
				w.Push.Deliver(pushCtx, task.result)
			}
			cancel()
		}
	}
}

// HandleMessage is the Kafka message callback.
func (w *Worker) HandleMessage(ctx context.Context, key, value []byte, headers map[string]string) (err error) {
	start := time.Now()
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
	select {
	case w.pushPool <- deliveryTask{result: result}:
	case <-w.pushStopCh:
		w.Log.Warnf("push pool closed, delivery dropped: msg_id=%d", result.Message.ID)
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
