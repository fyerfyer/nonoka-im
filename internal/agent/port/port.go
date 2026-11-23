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
	BaseURL       string
	APIKey        string
	Model         string
	MaxTokens     int
	ContextWindow int
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
}
type Turn struct {
	ID            uint64
	Input, Output string
}
type Usage struct{ InputTokens, OutputTokens, TotalTokens int }

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
