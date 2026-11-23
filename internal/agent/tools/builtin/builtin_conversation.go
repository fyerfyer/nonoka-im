package builtin

import (
	"nonoka-im/internal/agent/tools"

	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

func newConversationTool(backend tools.Backend, guard tools.Guard, resultBudgetTokens int) tool.InvokableTool {
	return mustTool(utils.InferTool("get_conversation_context", `Read the most recent messages of the current conversation, oldest first.

Use this tool to catch up on what happened before you were called, or to quote earlier messages accurately. Do not use it when you only need a keyword lookup — prefer search_messages for that.

Args:
- limit: max messages, 1-50 (default 20).`, func(ctx context.Context, in tools.ContextRequest) (tools.ContextResult, error) {
		r, ok := tools.GetRunContext(ctx)
		if !ok {
			return tools.ContextResult{}, fmt.Errorf("run context missing")
		}
		if in.Limit <= 0 || in.Limit > 50 {
			in.Limit = 20
		}
		if err := tools.Authorize(ctx, guard, r, "get_conversation_context", "topic", ""); err != nil {
			return tools.ContextResult{}, err
		}
		v, e := backend.ConversationContext(ctx, r, in.Limit)
		return tools.ContextResult{Messages: limitItems(v, resultBudgetTokens)}, e
	}))
}
