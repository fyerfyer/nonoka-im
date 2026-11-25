package realtime

import "context"

// RunEventType is the closed set of agent run stream events. Deltas are
// transient and never enter Kafka/Mongo; the final answer still goes through
// the normal IM message pipeline.
type RunEventType string

const (
	RunStarted          RunEventType = "started"
	RunDelta            RunEventType = "delta"
	RunToolStarted      RunEventType = "tool_started"
	RunToolFinished     RunEventType = "tool_finished"
	RunApprovalNeeded   RunEventType = "approval_required"
	RunApprovalResolved RunEventType = "approval_resolved"
	RunCompleted        RunEventType = "completed"
	RunFailed           RunEventType = "failed"
	RunCancelled        RunEventType = "cancelled"
	RunHeartbeat        RunEventType = "heartbeat"
)

// IsTerminal reports whether the event closes a run stream.
func (t RunEventType) IsTerminal() bool {
	switch t {
	case RunCompleted, RunFailed, RunCancelled:
		return true
	default:
		return false
	}
}

// IsDurable reports whether the event should be persisted to the run timeline.
// Deltas and heartbeats are intentionally transient.
func (t RunEventType) IsDurable() bool {
	switch t {
	case RunStarted, RunToolStarted, RunToolFinished, RunApprovalNeeded,
		RunApprovalResolved, RunCompleted, RunFailed, RunCancelled:
		return true
	default:
		return false
	}
}

// RunCitation is one message source consulted by a run. It is intentionally a
// reference plus a short excerpt; the message body remains in IM storage.
type RunCitation struct {
	MsgID    int64  `json:"msgId"`
	Topic    string `json:"topic"`
	TopicSeq uint64 `json:"topicSeq"`
	SenderID int64  `json:"senderId"`
	Snippet  string `json:"snippet,omitempty"`
}

// RunEvent is the single event shape carried by the realtime run stream. The
// pair (RunID, Seq) is the ordering and deduplication key.
type RunEvent struct {
	RunID   uint64       `json:"runId"`
	Seq     uint64       `json:"seq"`
	Type    RunEventType `json:"type"`
	Topic   string       `json:"topic"`
	AgentID int64        `json:"agentId"`
	// BotUserID identifies the agent's sender in the chat UI.
	BotUserID    int64 `json:"botUserId"`
	ActorUserID  int64 `json:"actorUserId"`
	TriggerMsgID int64 `json:"triggerMsgId,omitempty"`

	// Delta is set for RunDelta only.
	Delta string `json:"delta,omitempty"`
	// Tool fields are set for tool events.
	Tool       string `json:"tool,omitempty"`
	ToolCallID string `json:"toolCallId,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Status     string `json:"status,omitempty"`
	// ApprovalID is set for approval events.
	ApprovalID uint64 `json:"approvalId,omitempty"`
	// ReplyMsgID and ClientMessageID identify the final IM message. The HTTP
	// send path is asynchronous, so ClientMessageID is the reliable join key
	// even when the worker has not observed the assigned msg_id yet.
	ReplyMsgID      int64         `json:"replyMsgId,omitempty"`
	ClientMessageID string        `json:"clientMessageId,omitempty"`
	ErrorCode       string        `json:"errorCode,omitempty"`
	ErrorClass      string        `json:"errorClass,omitempty"`
	ErrorMessage    string        `json:"errorMessage,omitempty"`
	RetryAfterAt    string        `json:"retryAfterAt,omitempty"`
	Retryable       bool          `json:"retryable,omitempty"`
	InputTokens     int           `json:"inputTokens,omitempty"`
	OutputTokens    int           `json:"outputTokens,omitempty"`
	Citations       []RunCitation `json:"citations,omitempty"`
	TimestampMs     int64         `json:"timestampMs"`
}

// RunEventStore assigns per-run sequence numbers and keeps a bounded replay
// buffer. Implementations must make (run_id, seq) unique and monotonic.
type RunEventStore interface {
	Append(ctx context.Context, event RunEvent) (RunEvent, error)
	Since(ctx context.Context, runID, afterSeq uint64) ([]RunEvent, error)
}

// RunEventSink delivers run events to one or more live users.
type RunEventSink interface {
	Publish(ctx context.Context, event RunEvent, recipients []int64) error
}

// RunPublisher delivers transient run events to one user's live connections.
type RunPublisher interface {
	PublishRunEvent(ctx context.Context, userID int64, event RunEvent) error
}

// RunStream is the client-side subscription contract. Implementations must
// deliver events in (RunID, Seq) order and expose a terminal error after the
// underlying connection is lost and cannot be resumed.
type RunStream interface {
	Events() <-chan RunEvent
	Err() error
	Close() error
}
