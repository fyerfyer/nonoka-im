package builtin

import (
	"nonoka-im/internal/agent/tools"

	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

func newTopicInfoTool(backend tools.Backend, guard tools.Guard, resultBudgetTokens int) tool.InvokableTool {
	return utils.NewTool(&schema.ToolInfo{Name: "get_topic_info", Desc: `Get metadata about the current conversation: whether it is a direct chat or a group, the group's name and member list, or the direct chat counterpart.

Use this tool when you need to know who is in this conversation or what group this is, e.g. before addressing someone by name. Takes no arguments.`}, func(ctx context.Context, _ struct{}) (string, error) {
		r, ok := tools.GetRunContext(ctx)
		if !ok {
			return "", fmt.Errorf("run context missing")
		}
		if err := tools.Authorize(ctx, guard, r, "get_topic_info", "topic", ""); err != nil {
			return "", err
		}
		return backend.TopicInfo(ctx, r)
	})
}
