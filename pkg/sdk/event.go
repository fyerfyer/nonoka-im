package sdk

// RunEventType is the closed set of agent run stream events.
type RunEventType string

const (
	RunEventStarted          RunEventType = "started"
	RunEventDelta            RunEventType = "delta"
	RunEventToolStarted      RunEventType = "tool_started"
	RunEventToolFinished     RunEventType = "tool_finished"
	RunEventApprovalRequired RunEventType = "approval_required"
	RunEventApprovalResolved RunEventType = "approval_resolved"
	RunEventCompleted        RunEventType = "completed"
	RunEventFailed           RunEventType = "failed"
	RunEventCancelled        RunEventType = "cancelled"
	RunEventHeartbeat        RunEventType = "heartbeat"
)

func (t RunEventType) Terminal() bool {
	return t == RunEventCompleted || t == RunEventFailed || t == RunEventCancelled
}

// RunEvent is one transient agent run event.
type RunEvent struct {
	RunID        uint64       `json:"runId"`
	Seq          uint64       `json:"seq"`
	Type         RunEventType `json:"type"`
	Topic        string       `json:"topic"`
	AgentID      int64        `json:"agentId"`
	BotUserID    int64        `json:"botUserId"`
	ActorUserID  int64        `json:"actorUserId"`
	TriggerMsgID int64        `json:"triggerMsgId,omitempty"`
	Test         bool         `json:"test,omitempty"`

	Delta           string `json:"delta,omitempty"`
	Tool            string `json:"tool,omitempty"`
	ToolCallID      string `json:"toolCallId,omitempty"`
	Summary         string `json:"summary,omitempty"`
	Status          string `json:"status,omitempty"`
	ApprovalID      uint64 `json:"approvalId,omitempty"`
	ReplyMsgID      int64  `json:"replyMsgId,omitempty"`
	ClientMessageID string `json:"clientMessageId,omitempty"`
	ErrorCode       string `json:"errorCode,omitempty"`
	ErrorClass      string `json:"errorClass,omitempty"`
	Retryable       bool   `json:"retryable,omitempty"`
	InputTokens     int    `json:"inputTokens,omitempty"`
	OutputTokens    int    `json:"outputTokens,omitempty"`
	TimestampMs     int64  `json:"timestampMs,omitempty"`
}
