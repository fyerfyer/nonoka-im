package runtime

import "context"

type Input struct {
	Text                        string
	Instruction                 string
	Model                       string
	BaseURL                     string
	APIKey                      string
	MaxTokens                   int
	AllowedTools                []string
	RestrictTools               bool
	Summary                     string
	EventID                     string
	History                     []Turn
	AgentID, ActorUserID, BotID int64
	Topic                       string
	TokenBudget                 int
	OutputTokenBudget           int
	ToolResultTokenBudget       int
	TriggerTokens               int
	NeedsSummary                bool
	DisableSummary              bool
	SummaryThroughTurnID        uint64
	MCPTools                    []MCPTool
	Skills                      []SkillBinding
}
type Turn struct {
	ID            uint64
	Input, Output string
}
type Usage struct{ InputTokens, OutputTokens, TotalTokens int }

// MCPTool selects one tool from a bound MCP server for this run.
type MCPTool struct {
	Server string
	Tool   string
}

// SkillBinding selects one published skill version for this run.
type SkillBinding struct {
	Name    string
	Version int64
}

type Result struct {
	Text                 string
	Summary              string
	SummaryThroughTurnID uint64
	Usage                Usage
}
type Runtime interface {
	Run(context.Context, Input) (Result, error)
}
