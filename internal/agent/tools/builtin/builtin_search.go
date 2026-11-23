package builtin

import (
	"nonoka-im/internal/agent/tools"

	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

func newSearchTool(backend tools.Backend, guard tools.Guard, resultBudgetTokens int) tool.InvokableTool {
	return mustTool(utils.InferTool("search_messages", `Search the current conversation's message history for messages containing a keyword or phrase.

Use this tool when the user asks about something discussed earlier, wants you to find a specific message, or references facts you do not have in your current context. Results are the most recent matching messages.

Args:
- query: keyword or phrase to search for (required).
- limit: max results, 1-20 (default 10).
- since: optional unix timestamp (seconds); only messages sent at or after this time are returned.
- sender_id: optional user ID; only messages sent by this user are returned.`, func(ctx context.Context, in tools.SearchRequest) (tools.SearchResult, error) {
		r, ok := tools.GetRunContext(ctx)
		if !ok {
			return tools.SearchResult{}, fmt.Errorf("run context missing")
		}
		if in.Query == "" {
			return tools.SearchResult{}, fmt.Errorf("query is required")
		}
		if in.Limit <= 0 || in.Limit > 20 {
			in.Limit = 10
		}
		if err := tools.Authorize(ctx, guard, r, "search_messages", "topic", ""); err != nil {
			return tools.SearchResult{}, err
		}
		v, e := backend.SearchMessages(ctx, r, in.Query, in.Limit, in.Filter())
		return tools.SearchResult{Results: limitItems(v, resultBudgetTokens)}, e
	}))
}
