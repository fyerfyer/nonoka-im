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
			// Release the derived context once the stream is terminal.
			if s.cancel != nil {
				s.cancel()
			}
		})
	}
	go func() {
		defer finish()
		var lastSeq uint64
		var lastEventAt time.Time
		emit := func(event RunEvent) bool {
			if event.Seq <= lastSeq {
				return false
			}
			lastSeq = event.Seq
			lastEventAt = time.Now()
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
		terminalFromRun := func() bool {
			run, err := r.Get(streamCtx, runID)
			if err != nil {
				return false
			}
			var eventType RunEventType
			switch run.Status {
			case "completed":
				eventType = RunEventCompleted
			case "failed":
				eventType = RunEventFailed
			case "cancelled":
				eventType = RunEventCancelled
			default:
				return false
			}
			return emit(RunEvent{
				RunID: runID, Seq: lastSeq + 1, Type: eventType, Topic: run.Topic,
				AgentID: run.AgentID, ActorUserID: run.ActorUserID, TriggerMsgID: run.MsgID,
				Status: run.Status, ReplyMsgID: run.ReplyMsgID, ClientMessageID: run.ReplyClientMsgID,
				ErrorCode: run.ErrorCode, ErrorClass: run.ErrorClass,
				InputTokens: run.InputTokens, OutputTokens: run.OutputTokens,
				TimestampMs: time.Now().UnixMilli(),
			})
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
			// The replay buffer is bounded (15 minutes). If it expired while
			// the client was offline, recover the terminal state from the run.
			if len(events) == 0 && time.Since(lastEventAt) > 5*time.Second {
				return terminalFromRun()
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
				// While realtime is healthy and events are arriving, the HTTP
				// replay is an unnecessary extra request. Poll only when the
				// socket is down or the stream has gone quiet.
				if realtime != nil && r.client != nil && r.client.Realtime != nil &&
					r.client.Realtime.IsConnected() && time.Since(lastEventAt) < 5*time.Second {
					continue
				}
				if catchUp() {
					return
				}
			}
		}
	}()
	return s
}
