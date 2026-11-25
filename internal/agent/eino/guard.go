package eino

import (
	"context"
	"sync/atomic"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"nonoka-im/internal/agent/port"
)

// guardedChatModel applies the per-run budget guard before each model call.
// It wraps a ToolCallingChatModel so Eino's tool binding stays intact.
type guardedChatModel struct {
	base  model.ToolCallingChatModel
	guard port.UsageGuard
	turn  *atomic.Int32
}

func (m *guardedChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if err := m.guard.BeforeModel(ctx, int(m.turn.Add(1))); err != nil {
		return nil, err
	}
	return m.base.Generate(ctx, input, opts...)
}

func (m *guardedChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if err := m.guard.BeforeModel(ctx, int(m.turn.Add(1))); err != nil {
		return nil, err
	}
	return m.base.Stream(ctx, input, opts...)
}

func (m *guardedChatModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	base, err := m.base.WithTools(tools)
	if err != nil {
		return nil, err
	}
	return &guardedChatModel{base: base, guard: m.guard, turn: m.turn}, nil
}

// guardToolMiddleware prevents a tool from executing after the run has already
// exhausted its budget. It runs outside the durable ledger, so a blocked tool
// never commits a side effect.
func guardToolMiddleware(guard port.UsageGuard) compose.ToolMiddleware {
	wrap := func(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
		return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
			if err := guard.BeforeTool(ctx, input.Name); err != nil {
				return nil, err
			}
			return next(ctx, input)
		}
	}
	return compose.ToolMiddleware{Invokable: wrap}
}
