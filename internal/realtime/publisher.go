package realtime

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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

	// DeltaFlushInterval coalesces text deltas per run. Zero keeps the
	// synchronous behavior (useful for tests and simple embedders). The agent
	// worker sets a small interval so model streaming is not blocked by a
	// Redis INCR + gRPC round trip for every token chunk.
	DeltaFlushInterval time.Duration

	mu    sync.Mutex
	local map[uint64]uint64

	streamMu sync.Mutex
	streams  map[uint64]*runStream
	closed   bool
}

type runStream struct {
	// mu protects the delta buffer and timer only. It is never held across a
	// Redis/gRPC call, so model streaming is not blocked by a slow flush.
	mu         sync.Mutex
	pending    strings.Builder
	base       RunEvent
	recipients []int64
	timer      *time.Timer

	// closed is set by a terminal event. It is atomic so the broadcaster can
	// prune closed streams without acquiring every stream's mu.
	closed atomic.Bool

	// publishMu orders actual publishes for this run. A delta flush and a
	// durable/terminal event take it in turn; event buffering only needs mu.
	publishMu sync.Mutex
}

// maxRetainedStreams bounds the per-run state map. A terminal stream is kept
// closed for a while so duplicate/late events are dropped instead of creating
// a brand-new stream and emitting after the terminal event.
const maxRetainedStreams = 4096

func NewRunBroadcaster(store RunEventStore, transport RunEventTransport, steps func(context.Context, RunEvent) error, logger log.Logger) *RunBroadcaster {
	b := &RunBroadcaster{Store: store, Transport: transport, Steps: steps, local: map[uint64]uint64{}, streams: map[uint64]*runStream{}}
	if logger != nil {
		b.Log = log.NewHelper(logger)
	}
	return b
}

// Publish assigns a sequence, records the durable step (if any) and pushes the
// event to every recipient. Delivery failures are intentionally non-fatal:
// the final IM message remains the source of truth. When delta coalescing is
// enabled, delta calls return after buffering; non-delta and terminal events
// flush the buffer first so ordering is preserved.
func (b *RunBroadcaster) Publish(ctx context.Context, event RunEvent, recipients []int64) error {
	if b == nil {
		return nil
	}
	if b.isClosed() {
		return nil
	}
	if event.Type == RunDelta && b.DeltaFlushInterval > 0 {
		b.enqueueDelta(event, recipients)
		return nil
	}

	rs := b.stream(event.RunID)
	rs.publishMu.Lock()
	// A duplicate event after the terminal must not reopen the stream.
	rs.mu.Lock()
	closed := rs.closed.Load()
	rs.mu.Unlock()
	if closed {
		rs.publishMu.Unlock()
		return nil
	}
	b.flushPendingLocked(rs)
	err := b.publishLocked(ctx, event, recipients)
	if event.Type.IsTerminal() {
		// Mark under publishMu so a concurrent delta either precedes this
		// terminal (and is flushed above) or is rejected below.
		rs.mu.Lock()
		rs.closed.Store(true)
		rs.pending.Reset()
		rs.recipients = nil
		if rs.timer != nil {
			rs.timer.Stop()
			rs.timer = nil
		}
		rs.mu.Unlock()
	}
	rs.publishMu.Unlock()
	return err
}

func (b *RunBroadcaster) enqueueDelta(event RunEvent, recipients []int64) {
	rs := b.stream(event.RunID)
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.closed.Load() {
		return
	}
	if rs.pending.Len() == 0 {
		rs.base = event
	}
	rs.pending.WriteString(event.Delta)
	rs.recipients = recipients
	if rs.timer == nil {
		rs.timer = time.AfterFunc(b.DeltaFlushInterval, func() {
			b.flushRun(event.RunID)
		})
	}
}

func (b *RunBroadcaster) flushRun(runID uint64) {
	b.streamMu.Lock()
	rs := b.streams[runID]
	b.streamMu.Unlock()
	if rs == nil {
		return
	}
	b.flushPending(rs)
}

// flushPending publishes one coalesced delta, if any. It takes publishMu to
// reserve ordering, then stateMu only long enough to detach the buffer; the
// network call happens without blocking enqueueDelta.
func (b *RunBroadcaster) flushPending(rs *runStream) {
	rs.publishMu.Lock()
	defer rs.publishMu.Unlock()
	b.flushPendingLocked(rs)
}

// flushPendingLocked is flushPending for callers that already own publishMu.
func (b *RunBroadcaster) flushPendingLocked(rs *runStream) {
	rs.mu.Lock()
	if rs.closed.Load() {
		rs.mu.Unlock()
		return
	}
	if rs.timer != nil {
		rs.timer.Stop()
		rs.timer = nil
	}
	if rs.pending.Len() == 0 {
		rs.mu.Unlock()
		return
	}
	event := rs.base
	event.Type = RunDelta
	event.Delta = rs.pending.String()
	recipients := rs.recipients
	rs.pending.Reset()
	rs.recipients = nil
	rs.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := b.publishLocked(ctx, event, recipients); err != nil {
		b.log().Warnf("flush coalesced delta: run_id=%d err=%v", event.RunID, err)
	}
}

// publishLocked assigns a sequence and delivers one event. The caller holds
// the run's publishMu, which keeps delta flushes and durable events ordered.
func (b *RunBroadcaster) publishLocked(ctx context.Context, event RunEvent, recipients []int64) error {
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

// Close flushes buffered deltas and rejects new writes. It is safe to call
// more than once.
func (b *RunBroadcaster) Close() {
	if b == nil {
		return
	}
	b.streamMu.Lock()
	if b.closed {
		b.streamMu.Unlock()
		return
	}
	b.closed = true
	ids := make([]uint64, 0, len(b.streams))
	for id := range b.streams {
		ids = append(ids, id)
	}
	b.streamMu.Unlock()
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(runID uint64) {
			defer wg.Done()
			b.flushRun(runID)
		}(id)
	}
	wg.Wait()

	// Reject any late delta/terminal after Close by marking retained streams
	// closed. The map itself is bounded by maxRetainedStreams and pruned on the
	// next stream creation.
	b.streamMu.Lock()
	for _, rs := range b.streams {
		rs.closed.Store(true)
	}
	b.streamMu.Unlock()
}

func (b *RunBroadcaster) stream(runID uint64) *runStream {
	b.streamMu.Lock()
	defer b.streamMu.Unlock()
	if b.closed {
		// Close already flushed; keep a throwaway state so a late Publish does
		// not write into a closed broadcaster.
		rs := &runStream{}
		rs.closed.Store(true)
		return rs
	}
	if rs := b.streams[runID]; rs != nil {
		return rs
	}
	if len(b.streams) >= maxRetainedStreams {
		b.pruneStreamsLocked()
	}
	rs := &runStream{}
	b.streams[runID] = rs
	return rs
}

// pruneStreamsLocked removes closed streams. It is called with streamMu held.
func (b *RunBroadcaster) pruneStreamsLocked() {
	target := maxRetainedStreams * 3 / 4
	for id, rs := range b.streams {
		if len(b.streams) <= target {
			return
		}
		if rs.closed.Load() {
			delete(b.streams, id)
		}
	}
}

func (b *RunBroadcaster) isClosed() bool {
	b.streamMu.Lock()
	defer b.streamMu.Unlock()
	return b.closed
}

func (b *RunBroadcaster) assignSeq(ctx context.Context, event RunEvent) RunEvent {
	if b.Store != nil {
		stored, err := b.Store.Append(ctx, event)
		if err == nil {
			b.mu.Lock()
			b.local[event.RunID] = stored.Seq
			b.pruneLocalLocked(event.RunID)
			b.mu.Unlock()
			return stored
		}
		b.log().Warnf("append run event: run_id=%d type=%s err=%v", event.RunID, event.Type, err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	// Continue from the last sequence observed from the store. Starting at 1
	// after a Redis outage would make clients drop the events as duplicates.
	b.local[event.RunID]++
	event.Seq = b.local[event.RunID]
	b.pruneLocalLocked(event.RunID)
	return event
}

// pruneLocalLocked keeps the per-run fallback map bounded. Only used when the
// event store is unavailable; dropping old finished runs is safe.
func (b *RunBroadcaster) pruneLocalLocked(keep uint64) {
	if len(b.local) <= 4096 {
		return
	}
	removed := 0
	for runID := range b.local {
		if runID == keep {
			continue
		}
		delete(b.local, runID)
		removed++
		if removed >= 1024 {
			break
		}
	}
}

func (b *RunBroadcaster) log() *log.Helper {
	if b.Log != nil {
		return b.Log
	}
	return log.NewHelper(log.DefaultLogger)
}
