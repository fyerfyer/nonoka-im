package realtime

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-kratos/kratos/v2/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// RunEventTransport pushes a run event to all live connections of one user.
type RunEventTransport interface {
	PushRunEventToUser(ctx context.Context, userID int64, event RunEvent) (int32, error)
}

// RunBroadcaster owns run-event sequencing, replay storage, durable timeline
// recording and live delivery. It is deliberately independent from the agent
// application layer so both the worker and tests can use it.
type RunBroadcaster struct {
	Store     RunEventStore
	Transport RunEventTransport
	Steps     func(context.Context, RunEvent) error
	OnEvent   func(RunEvent)
	Log       *log.Helper

	mu    sync.Mutex
	local map[uint64]uint64
}

func NewRunBroadcaster(store RunEventStore, transport RunEventTransport, steps func(context.Context, RunEvent) error, logger log.Logger) *RunBroadcaster {
	b := &RunBroadcaster{Store: store, Transport: transport, Steps: steps, local: map[uint64]uint64{}}
	if logger != nil {
		b.Log = log.NewHelper(logger)
	}
	return b
}

// Publish assigns a sequence, records the durable step (if any) and pushes the
// event to every recipient. Delivery failures are intentionally non-fatal:
// the final IM message remains the source of truth.
func (b *RunBroadcaster) Publish(ctx context.Context, event RunEvent, recipients []int64) error {
	if b == nil {
		return nil
	}
	event = b.assignSeq(ctx, event)
	if b.OnEvent != nil {
		b.OnEvent(event)
	}
	if event.Type.IsDurable() {
		var span trace.Span
		ctx, span = otel.Tracer("nonoka-im/realtime").Start(ctx, "agent.stream.publish",
			trace.WithAttributes(
				attribute.String("agent.run_id", fmt.Sprintf("%d", event.RunID)),
				attribute.String("agent.event.type", string(event.Type)),
			))
		defer span.End()
	}
	if event.Type.IsDurable() && b.Steps != nil {
		if err := b.Steps(ctx, event); err != nil {
			b.log().Warnf("record run step: run_id=%d type=%s err=%v", event.RunID, event.Type, err)
		}
	}
	if b.Transport == nil || len(recipients) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(recipients))
	var firstErr error
	for _, userID := range recipients {
		if userID <= 0 {
			continue
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		if _, err := b.Transport.PushRunEventToUser(ctx, userID, event); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("push run event to user %d: %w", userID, err)
		}
	}
	return firstErr
}

func (b *RunBroadcaster) assignSeq(ctx context.Context, event RunEvent) RunEvent {
	if b.Store != nil {
		stored, err := b.Store.Append(ctx, event)
		if err == nil {
			return stored
		}
		b.log().Warnf("append run event: run_id=%d type=%s err=%v", event.RunID, event.Type, err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.local[event.RunID]++
	event.Seq = b.local[event.RunID]
	return event
}

func (b *RunBroadcaster) log() *log.Helper {
	if b.Log != nil {
		return b.Log
	}
	return log.NewHelper(log.DefaultLogger)
}
