package domain

import "time"

// Tool invocation states. A side-effecting tool is recorded before it runs so
// a resumed or retried run can never execute the same tool call twice.
const (
	ToolInvocationStarted   = "started"
	ToolInvocationCompleted = "completed"
	ToolInvocationFailed    = "failed"
)

// ToolInvocation is the durable ledger entry for one side-effecting tool call.
// ArgsHash is the SHA-256 of the raw arguments; raw arguments never enter this
// table. ResultSummary is a bounded, model-safe summary used when a completed
// call is replayed.
type ToolInvocation struct {
	ID            uint64
	RunID         uint64
	ToolCallID    string
	Tool          string
	ArgsHash      string
	Status        string
	ResultSummary string
	ErrorCode     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
