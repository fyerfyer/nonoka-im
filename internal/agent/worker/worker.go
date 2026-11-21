package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"nonoka-im/internal/agent/executor"
	"nonoka-im/internal/agent/metrics"
	"nonoka-im/internal/agent/sweeper"
	kafkax "nonoka-im/internal/kafka"
	"nonoka-im/internal/messaging"

	"github.com/go-kratos/kratos/v2/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Worker owns the agent trigger consumer, the queued-run replay loop and the
// fixed-size execution pool. It implements Kratos transport.Server so the
// process lifecycle is managed by kratos.App.
type Worker struct {
	Store      RunQueue
	Dispatcher *Dispatcher
	Executor   *executor.Executor
	Sweeper    *sweeper.Sweeper
	Consumer   *kafkax.Consumer
	Metrics    *metrics.Metrics
	Log        *log.Helper

	RunWorkers int
	QueuedPoll time.Duration
	QueueSize  int
	Background []func(context.Context)

	work     chan uint64
	cancel   context.CancelFunc
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// Start implements transport.Server. It starts the pool and background loops,
// then blocks on the Kafka consumer until the context is canceled.
func (w *Worker) Start(ctx context.Context) error {
	runCtx, cancel := w.withCancel(ctx)
	defer cancel()
	queueSize := w.QueueSize
	if queueSize <= 0 {
		queueSize = 256
	}
	w.work = make(chan uint64, queueSize)
	workers := w.RunWorkers
	if workers < 1 {
		workers = 1
	}
	for i := 0; i < workers; i++ {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			w.runLoop(runCtx)
		}()
	}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.replayLoop(runCtx)
	}()
	if w.Sweeper != nil {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			w.Sweeper.Run(runCtx)
		}()
	}
	for _, run := range w.Background {
		w.wg.Add(1)
		go func(fn func(context.Context)) {
			defer w.wg.Done()
			fn(runCtx)
		}(run)
	}
	if w.Consumer == nil {
		<-runCtx.Done()
		return nil
	}
	err := w.Consumer.Start(runCtx)
	cancel()
	w.wg.Wait()
	return err
}

// Stop implements transport.Server.
func (w *Worker) Stop(context.Context) error {
	w.stopOnce.Do(func() {
		if w.cancel != nil {
			w.cancel()
		}
		if w.Consumer != nil {
			_ = w.Consumer.Stop()
		}
	})
	return nil
}

// Ready reports whether the Kafka consumer has received its first message.
func (w *Worker) Ready() bool {
	return w.Consumer != nil && w.Consumer.Started()
}

func (w *Worker) withCancel(ctx context.Context) (context.Context, context.CancelFunc) {
	runCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	return runCtx, cancel
}

func (w *Worker) runLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case id, ok := <-w.work:
			if !ok {
				return
			}
			w.Executor.ExecuteRun(ctx, id)
		}
	}
}

// replayLoop polls claimable runs and hands their IDs to the pool. Claiming
// happens in the pool, so a full queue never holds a lease.
func (w *Worker) replayLoop(ctx context.Context) {
	interval := w.QueuedPoll
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runs, err := w.Store.QueuedRuns(ctx, 64)
			if err != nil {
				w.log().Errorf("load queued runs: %v", err)
				continue
			}
			if w.Metrics != nil {
				w.Metrics.QueueDepth.Set(float64(len(runs)))
			}
			for i := range runs {
				select {
				case w.work <- runs[i].ID:
				case <-ctx.Done():
					return
				default:
					// Queue is full; the run stays queued for the next poll.
				}
			}
		}
	}
}

// HandleEvent is the Kafka consumer callback.
func (w *Worker) HandleEvent(ctx context.Context, key, value []byte, headers map[string]string) error {
	var ev messaging.MessageEvent
	if err := json.Unmarshal(value, &ev); err != nil {
		return err
	}
	if ev.SchemaVersion != messaging.SchemaVersion || ev.EventID == "" || ev.MsgID <= 0 {
		return fmt.Errorf("invalid agent event")
	}
	traceParent := ev.TraceParent
	if traceParent == "" {
		traceParent = headers[kafkax.TraceParentHeader]
	}
	ctx, span := otel.Tracer("nonoka-im/agent/worker").Start(ctx, "agent.dispatch",
		trace.WithAttributes(
			attribute.String("agent.event_id", ev.EventID),
			attribute.Int64("messaging.msg_id", ev.MsgID),
			attribute.String("messaging.topic", ev.Topic),
		))
	if link, ok := kafkax.LinkFromTraceParent(traceParent); ok {
		span.AddLink(link)
	}
	defer span.End()

	if err := w.Dispatcher.Dispatch(ctx, ev); err != nil {
		if w.Metrics != nil {
			w.Metrics.DispatchTotal.WithLabelValues("failure").Inc()
		}
		span.RecordError(err)
		return err
	}
	if w.Metrics != nil {
		w.Metrics.DispatchTotal.WithLabelValues("success").Inc()
	}
	return nil
}

func (w *Worker) log() *log.Helper {
	if w.Log != nil {
		return w.Log
	}
	return log.NewHelper(log.DefaultLogger)
}
