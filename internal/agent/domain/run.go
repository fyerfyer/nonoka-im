package domain

import "time"

// Run status values. Only a worker may move queued -> running; all terminal
// writes are fenced by AttemptID.
const (
	RunQueued          = "queued"
	RunRunning         = "running"
	RunWaitingApproval = "waiting_approval"
	RunCompleted       = "completed"
	RunFailed          = "failed"
	RunCancelled       = "cancelled"
)

// Durable run-step types. Delta events are intentionally absent: they are
// transient and only kept in the Redis run-event buffer.
const (
	RunStepStarted          = "started"
	RunStepToolStarted      = "tool_started"
	RunStepToolFinished     = "tool_finished"
	RunStepApproval         = "approval_required"
	RunStepApprovalResolved = "approval_resolved"
	RunStepCompleted        = "completed"
	RunStepFailed           = "failed"
	RunStepCancelled        = "cancelled"
)

// Failure classes/codes that are shared by the persistence adapter and the
// executor without importing the runtime package.
const (
	FailureClassSideEffect = "side_effect_committed"
	FailureCodeManualRetry = "manual_retry_required"
)

// AgentRun is one triggered execution. EventID+AgentID is the idempotency
// boundary for Kafka redelivery.
type AgentRun struct {
	ID            uint64
	EventID       string
	AgentID       int64
	AgentVersion  int64
	SessionID     uint64
	ActorUserID   int64
	Topic         string
	MsgID         int64
	TopicSeq      uint64
	Status        string
	ErrorCode     string
	ErrorClass    string
	LastError     string
	Attempts      int
	AttemptID     string
	LeaseUntil    *time.Time
	NextAttemptAt *time.Time
	ReplyMsgID    int64
	CheckpointID  string
	// SideEffectAt records that a write tool already produced an external
	// effect during this run. Retryable failures after this point are forced
	// terminal until an operator explicitly acknowledges the risk.
	SideEffectAt     *time.Time
	InputTokens      int
	OutputTokens     int
	TotalTokens      int
	ModelKey         string
	EstimatedCostUSD float64
	RetryAfterAt     *time.Time
	CreatedAt        time.Time
	StartedAt        *time.Time
	FinishedAt       *time.Time
}

// AgentRunStep is one durable point in a run timeline. Summary is redacted and
// never contains raw tool arguments.
type AgentRunStep struct {
	ID         uint64
	RunID      uint64
	Seq        uint64
	Type       string
	Tool       string
	ToolCallID string
	Summary    string
	Status     string
	CreatedAt  time.Time
}

type AgentTurn struct {
	ID           uint64
	AgentID      int64
	ActorUserID  int64
	Topic        string
	SessionID    uint64
	RunID        uint64
	TriggerMsgID int64
	ReplyMsgID   int64
	Input        string
	Output       string
	CreatedAt    time.Time
}

type AgentSession struct {
	ID          uint64
	AgentID     int64
	ActorUserID int64
	Topic       string
	UpdatedAt   time.Time
	CreatedAt   time.Time
}

type AgentContextSnapshot struct {
	ID            uint64
	SessionID     uint64
	LastTurnID    uint64
	AgentVersion  int64
	Summary       string
	SummaryTokens int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// PreparedRun is the claimed run plus the conversation state loaded for it.
type PreparedRun struct {
	Session  *AgentSession
	Run      *AgentRun
	Snapshot *AgentContextSnapshot
	Turns    []AgentTurn
}

// Failure is the terminal or retryable outcome of one execution attempt.
type Failure struct {
	Class      string
	Code       string
	Message    string
	Retryable  bool
	RetryAfter time.Duration
}
