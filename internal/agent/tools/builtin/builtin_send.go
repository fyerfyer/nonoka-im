package builtin

import (
	"nonoka-im/internal/agent/tools"

	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

func newSendTool(backend tools.Backend, guard tools.Guard, resultBudgetTokens int) tool.InvokableTool {
	return utils.NewTool(&schema.ToolInfo{Name: "send_message", Desc: `Send a message to the current conversation as the agent. The message is delivered to every participant immediately.

Use this tool when the user explicitly asks you to post or announce something. Your normal final reply is already delivered to the conversation, so do not use this tool to repeat your answer.

Args:
- content: message text (required).`}, func(ctx context.Context, in tools.SendRequest) (tools.SendResult, error) {
		r, ok := tools.GetRunContext(ctx)
		if !ok {
			return tools.SendResult{}, fmt.Errorf("run context missing")
		}
		if in.Content == "" {
			return tools.SendResult{}, fmt.Errorf("content is required")
		}
		if err := tools.Authorize(ctx, guard, r, "send_message", "topic", ""); err != nil {
			return tools.SendResult{}, err
		}
		e := backend.SendMessage(ctx, r, in.Content)
		return tools.SendResult{Accepted: e == nil}, e
	})
}
