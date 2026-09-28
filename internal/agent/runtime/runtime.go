package runtime

import "context"

type Input struct {
	Text                        string
	EventID                     string
	History                     []Turn
	AgentID, ActorUserID, BotID int64
	Topic                       string
}
type Turn struct{ Input, Output string }
type Result struct{ Text string }
type Runtime interface {
	Run(context.Context, Input) (Result, error)
}
