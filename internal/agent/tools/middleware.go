package tools

import (
	"context"
	"time"

	"github.com/cloudwego/eino/compose"
)

// ToolCallTimeout bounds a single tool execution so one stuck backend call
// (e.g. a hung MCP request) cannot consume the whole run budget. The run
// context may already carry an earlier deadline; context.WithTimeout keeps the
// earlier one automatically.
const ToolCallTimeout = 20 * time.Second

// TimeoutMiddleware applies ToolCallTimeout at the Eino tools node, which is
// the single place every tool call passes through. Tool implementations stay
// free of timeout boilerplate.
func TimeoutMiddleware(timeout time.Duration) compose.ToolMiddleware {
	if timeout <= 0 {
		timeout = ToolCallTimeout
	}
	wrap := func(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
		return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			return next(ctx, input)
		}
	}
	return compose.ToolMiddleware{Invokable: wrap}
}
