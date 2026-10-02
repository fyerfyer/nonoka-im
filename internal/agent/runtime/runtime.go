package runtime

import "context"

type Input struct {
	Text                        string
	Instruction                 string
	Model                       string
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
}
type Turn struct {
	ID            uint64
	Input, Output string
}
type Usage struct{ InputTokens, OutputTokens, TotalTokens int }
type Result struct {
	Text                 string
	Summary              string
	SummaryThroughTurnID uint64
	Usage                Usage
}
type Runtime interface {
	Run(context.Context, Input) (Result, error)
}
