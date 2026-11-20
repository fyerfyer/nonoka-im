package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	agevent "nonoka-im/internal/agent/event"
	"nonoka-im/internal/agent/executor"
	"nonoka-im/internal/agent/metrics"
	"nonoka-im/internal/agent/sweeper"
	kafkax "nonoka-im/internal/kafka"

	"github.com/go-kratos/kratos/v2/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Worker owns the agent trigger consumer, the queued-run replay loop and the
// fixed-size execution pool. All state lives in PostgreSQL; the process is
// stateless and can be restarted at any point.
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
	work       chan uint64
}

// Run starts the pool and background loops, then consumes until ctx is done
// or the consumer fails.
func (w *Worker) Run(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
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
		go w.runLoop(runCtx)
	}
	go w.replayLoop(runCtx)
	if w.Sweeper != nil {
		go w.Sweeper.Run(runCtx)
	}
	return w.Consumer.Start(runCtx)
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
	var ev agevent.MessageEvent
	if err := json.Unmarshal(value, &ev); err != nil {
		return err
	}
	if ev.SchemaVersion != agevent.SchemaVersion || ev.EventID == "" || ev.MsgID <= 0 {
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
		// A link keeps the consumer span independent while recording the
		// message-processing span that produced the trigger.
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
