package realtime

import "context"

// RunEventType is the closed set of agent run stream events. Deltas are
// transient and never enter Kafka/Mongo; the final answer still goes through
// the normal IM message pipeline.
type RunEventType string

const (
	RunStarted        RunEventType = "started"
	RunDelta          RunEventType = "delta"
	RunToolStarted    RunEventType = "tool_started"
	RunToolFinished   RunEventType = "tool_finished"
	RunApprovalNeeded RunEventType = "approval_required"
	RunCompleted      RunEventType = "completed"
	RunFailed         RunEventType = "failed"
	RunCancelled      RunEventType = "cancelled"
	RunHeartbeat      RunEventType = "heartbeat"
)

// RunEvent is the single event shape carried by the realtime run stream. The
// pair (RunID, Seq) is the ordering and deduplication key.
type RunEvent struct {
	RunID   uint64       `json:"runId"`
	Seq     uint64       `json:"seq"`
	Type    RunEventType `json:"type"`
	Topic   string       `json:"topic"`
	AgentID int64        `json:"agentId"`

	// Delta is set for RunDelta only.
	Delta string `json:"delta,omitempty"`
	// Tool fields are set for tool events.
	Tool    string `json:"tool,omitempty"`
	Summary string `json:"summary,omitempty"`
	// ApprovalID is set for RunApprovalNeeded.
	ApprovalID uint64 `json:"approvalId,omitempty"`
	// ReplyMsgID and Usage are set for RunCompleted.
	ReplyMsgID   int64  `json:"replyMsgId,omitempty"`
	ErrorCode    string `json:"errorCode,omitempty"`
	InputTokens  int    `json:"inputTokens,omitempty"`
	OutputTokens int    `json:"outputTokens,omitempty"`
	TimestampMs  int64  `json:"timestampMs"`
}

// RunPublisher delivers transient run events to one user's live connections.
// Phase 4 implements it over the Gateway gRPC push API; Phase 0 only fixes the
// contract so callers and SDK can be written against a stable shape.
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
