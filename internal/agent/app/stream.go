package app

import (
	"context"
	"time"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/port"
	"nonoka-im/internal/realtime"
	"nonoka-im/internal/topic"
)

// RunAudience resolves group members for typing/started fan-out. P2P runs do
// not need an audience and pass nil.
type RunAudience interface {
	GroupMemberIDs(ctx context.Context, groupID string) ([]int64, error)
}

// emitRunEvent publishes one transient run event. Delivery is best-effort and
// uses a detached context so cancellation/terminal events still reach clients.
func (h *Harness) emitRunEvent(ctx context.Context, run *domain.AgentRun, agent ResolvedAgent, event realtime.RunEvent, groupAudience bool) {
	if h.Stream == nil || run == nil {
		return
	}
	event.RunID = run.ID
	event.Topic = run.Topic
	event.AgentID = agent.ID
	event.BotUserID = agent.BotID
	event.ActorUserID = run.ActorUserID
	event.TriggerMsgID = run.MsgID
	event.Test = agent.Test || run.Kind == domain.RunKindTest
	if event.TimestampMs == 0 {
		event.TimestampMs = time.Now().UnixMilli()
	}
	recipients := []int64{run.ActorUserID}
	if groupAudience && !agent.Test {
		recipients = h.groupRecipients(ctx, run.Topic, run.ActorUserID)
	}
	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if err := h.Stream.Publish(sendCtx, event, recipients); err != nil && h.Log != nil {
		h.Log.Warnf("publish run event: run_id=%d type=%s err=%v", run.ID, event.Type, err)
	}
}

func (h *Harness) groupRecipients(ctx context.Context, topicName string, actorID int64) []int64 {
	recipients := []int64{actorID}
	if h.Audience == nil || topic.ParseType(topicName) != topic.TypeGroup {
		return recipients
	}
	groupID, err := topic.ExtractGroupID(topicName)
	if err != nil {
		return recipients
	}
	members, err := h.Audience.GroupMemberIDs(ctx, groupID)
	if err != nil {
		return recipients
	}
	for _, member := range members {
		if member > 0 && member != actorID {
			recipients = append(recipients, member)
		}
	}
	return recipients
}

// startHeartbeat emits a heartbeat every interval until the returned stop
// function is called. Heartbeats update the "Agent is working" indicator and
// keep long tool/model turns visibly alive.
func (h *Harness) startHeartbeat(ctx context.Context, run *domain.AgentRun, agent ResolvedAgent, interval time.Duration) func() {
	if h.Stream == nil {
		return func() {}
	}
	if interval <= 0 {
		interval = 10 * time.Second
	}
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				h.emitRunEvent(ctx, run, agent, realtime.RunEvent{Type: realtime.RunHeartbeat, Status: domain.RunRunning}, true)
			}
		}
	}()
	return func() {
		select {
		case <-done:
		default:
			close(done)
		}
	}
}

// eventSink adapts runtime progress to the run event contract.
func (h *Harness) eventSink(ctx context.Context, run *domain.AgentRun, agent ResolvedAgent) port.EventSink {
	return func(event port.StreamEvent) {
		out := mapRuntimeEvent(event)
		h.emitRunEvent(ctx, run, agent, out, false)
	}
}

// mapRuntimeEvent is kept explicit so the port stays free of realtime types.
func mapRuntimeEvent(event port.StreamEvent) realtime.RunEvent {
	switch event.Type {
	case port.EventDelta:
		return realtime.RunEvent{Type: realtime.RunDelta, Delta: event.Delta}
	case port.EventToolStarted:
		return realtime.RunEvent{Type: realtime.RunToolStarted, Tool: event.Tool, ToolCallID: event.ToolCallID, Summary: event.Summary}
	case port.EventToolFinished:
		return realtime.RunEvent{Type: realtime.RunToolFinished, Tool: event.Tool, ToolCallID: event.ToolCallID, Summary: event.Summary, Status: event.Status}
	default:
		return realtime.RunEvent{Type: realtime.RunEventType(event.Type)}
	}
}
