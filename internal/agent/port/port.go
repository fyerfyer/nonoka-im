// Package port is the application-owned port for agent execution. The core
// (app) depends on these interfaces and DTOs; infrastructure adapters such as
// eino implement them. It deliberately contains no persistence, transport or
// model-provider types.
package port

import (
	"context"

	"nonoka-im/internal/agent/domain"
)

// ModelEndpoint is the resolved platform model access point for one run. The
// agent runtime never reads arbitrary per-agent URLs or provider credentials.
type ModelEndpoint struct {
	BaseURL            string
	APIKey             string
	VirtualKey         string
	Model              string
	ModelKey           string
	ModelID            uint64
	Provider           string
	MaxTokens          int
	ContextWindow      int
	SupportsVision     bool
	InputPricePerMTok  float64
	OutputPricePerMTok float64
}

type Input struct {
	Text                        string
	Instruction                 string
	Model                       ModelEndpoint
	AllowedTools                []string
	RestrictTools               bool
	Summary                     string
	EventID                     string
	RunID                       uint64
	AttemptID                   string
	History                     []Turn
	AgentID, ActorUserID, BotID int64
	Topic                       string
	DisableSummary              bool
	SummaryThroughTurnID        uint64
	MCPTools                    []MCPTool
	Skills                      []SkillBinding
	// CheckPointID scopes the Eino execution checkpoint for this run. It must
	// be stable across the run and its resume (e.g. derived from the run ID).
	CheckPointID string
	// ApprovalTools lists tool names whose calls interrupt for human
	// confirmation instead of executing directly.
	ApprovalTools []string
	// SideEffectTools lists tools that must be wrapped by the durable
	// at-most-once ledger even when they do not require approval.
	SideEffectTools []string
	// OnEvent receives transient runtime progress. Implementations must not
	// block model generation on slow consumers.
	OnEvent EventSink
	// Guard enforces per-run token/cost budgets and records usage. Nil disables
	// accounting (tests and legacy callers).
	Guard UsageGuard
	// MaxToolTurns caps model iterations for this run. Zero uses the adapter
	// default.
	MaxToolTurns int
	// Attachments carries the current trigger message's images/files. Only the
	// current message is multimodal; historical attachments are text
	// placeholders.
	Attachments []Attachment
}

// Attachment is one authorized attachment supplied to the runtime.
type Attachment struct {
	Kind   string // image | file
	FileID string
	Name   string
	Mime   string
	Data   []byte
}

// StreamEventType is the transport-neutral runtime progress event type.
type StreamEventType string

const (
	EventDelta        StreamEventType = "delta"
	EventToolStarted  StreamEventType = "tool_started"
	EventToolFinished StreamEventType = "tool_finished"
	EventHeartbeat    StreamEventType = "heartbeat"
)

// StreamEvent is a transient progress signal emitted by the runtime.
type StreamEvent struct {
	Type       StreamEventType
	Delta      string
	Tool       string
	ToolCallID string
	Summary    string
	Status     string
}

// EventSink consumes runtime progress events.
type EventSink func(StreamEvent)
type Turn struct {
	ID            uint64
	Input, Output string
}
type Usage struct{ InputTokens, OutputTokens, TotalTokens int }

// UsageGuard observes model/tool execution for one run. AfterModel receives the
// cumulative usage of the run; implementations can settle deltas and abort the
// next model/tool call when a daily budget is exhausted.
type UsageGuard interface {
	BeforeModel(ctx context.Context, turn int) error
	AfterModel(ctx context.Context, cumulative Usage) error
	BeforeTool(ctx context.Context, tool string) error
}

// MCPTool selects one tool from a bound MCP server for this run. It aliases
// the domain value object so the run contract does not duplicate it.
type MCPTool = domain.MCPBinding

// SkillBinding selects one published skill version for this run.
type SkillBinding = domain.SkillBinding

// Run status values returned in Result.Status.
const (
	StatusCompleted   = "completed"
	StatusInterrupted = "interrupted"
)

// InterruptInfo describes a tool call that paused the run for human approval.
type InterruptInfo struct {
	// ToolCallID is the Eino interrupt address (e.g.
	// "agent:A;node:...;tool:<id>") used to target the resume.
	ToolCallID string
	Tool       string
	// Summary is a redacted, human-readable description. Raw arguments never
	// leave the checkpoint.
	Summary string
	// ArgsHash is the SHA-256 of the raw arguments, used for audit/matching.
	ArgsHash string
}

type Result struct {
	Text                 string
	Status               string
	Interrupt            *InterruptInfo
	Summary              string
	SummaryThroughTurnID uint64
	Usage                Usage
}

// ResumeInput continues an interrupted run from its checkpoint.
type ResumeInput struct {
	Input
	// InterruptID is the ToolCallID of the approval being resolved.
	InterruptID string
	// Approved decides what the interrupted tool call returns: execute the
	// underlying tool, or feed the model a rejection message.
	Approved bool
}

type Runtime interface {
	Run(context.Context, Input) (Result, error)
	Resume(context.Context, ResumeInput) (Result, error)
}
