package domain

import "time"

// Approval status values. The resume path consumes approved/rejected/expired
// exactly once so a crash after the decision cannot re-run a side effect.
const (
	ApprovalPending  = "pending"
	ApprovalApproved = "approved"
	ApprovalRejected = "rejected"
	ApprovalExpired  = "expired"
	ApprovalConsumed = "consumed"
)

// AgentCheckpoint persists an Eino execution checkpoint (opaque gob bytes)
// keyed by the run-scoped checkpoint ID so an interrupted run can resume
// after a worker restart.
type AgentCheckpoint struct {
	ID           uint64
	RunID        uint64
	CheckpointID string
	State        []byte
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

// AgentApproval is the business record of a tool call that interrupted for
// human confirmation. Raw tool arguments never enter this table.
type AgentApproval struct {
	ID          uint64
	RunID       uint64
	ToolCallID  string
	Tool        string
	ArgsSummary string
	ArgsHash    string
	ApproverID  int64
	Status      string
	ExpiresAt   time.Time
	DecidedAt   *time.Time
	DecidedBy   int64
	AttemptID   string
	ConsumedAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
