package sdk

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RunStream is a resumable stream of one agent run. It starts with an HTTP
// replay (events?afterSeq=0), then follows WebSocket events and periodically
// catches up over HTTP when the realtime connection is unavailable.
type RunStream struct {
	events chan RunEvent
	cancel context.CancelFunc

	mu     sync.Mutex
	err    error
	closed bool
}

func (s *RunStream) Events() <-chan RunEvent { return s.events }

func (s *RunStream) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *RunStream) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

func (s *RunStream) setErr(err error) {
	s.mu.Lock()
	if s.err == nil {
		s.err = err
	}
	s.mu.Unlock()
}

// Stream subscribes to a run. The caller must Close the stream when done.
func (r *RunsResource) Stream(ctx context.Context, runID uint64) *RunStream {
	streamCtx, cancel := context.WithCancel(ctx)
	s := &RunStream{events: make(chan RunEvent, 128), cancel: cancel}
	var closeOnce sync.Once
	finish := func() {
		closeOnce.Do(func() {
			s.mu.Lock()
			s.closed = true
			s.mu.Unlock()
			close(s.events)
		})
	}
	go func() {
		defer finish()
		var lastSeq uint64
		emit := func(event RunEvent) bool {
			if event.Seq <= lastSeq {
				return false
			}
			lastSeq = event.Seq
			if event.Type == RunEventFailed {
				s.setErr(&APIError{Code: event.ErrorCode, Message: fmt.Sprintf("agent run failed: %s", event.ErrorCode), Retryable: event.Retryable})
			}
			select {
			case s.events <- event:
			case <-streamCtx.Done():
				return true
			}
			return event.Type.Terminal()
		}
		catchUp := func() bool {
			events, _, err := r.Events(streamCtx, runID, lastSeq)
			if err != nil {
				if streamCtx.Err() != nil {
					return true
				}
				s.setErr(err)
				return false
			}
			for _, event := range events {
				if emit(event) {
					return true
				}
			}
			return false
		}
		if catchUp() {
			return
		}
		var realtime <-chan RealtimeEvent
		cancelRealtime := func() {}
		if r.client != nil && r.client.Realtime != nil {
			realtime, cancelRealtime = r.client.Realtime.Subscribe()
		}
		defer cancelRealtime()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-streamCtx.Done():
				return
			case event, ok := <-realtime:
				if !ok {
					realtime = nil
					continue
				}
				if event.Type == RealtimeAgentRunEvent && event.RunEvent != nil && event.RunEvent.RunID == runID {
					if emit(*event.RunEvent) {
						return
					}
				}
			case <-ticker.C:
				if catchUp() {
					return
				}
			}
		}
	}()
	return s
}
