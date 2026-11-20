package builtin

import (
	"nonoka-im/internal/agent/tools"

	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

func newMyGroupsTool(backend tools.Backend, guard tools.Guard, resultBudgetTokens int) tool.InvokableTool {
	return utils.NewTool(&schema.ToolInfo{Name: "list_my_groups", Desc: `List the groups the current user has joined: group name, group ID and member count for each (up to 50 groups).

Use this tool when the user asks which groups they are in, or before using get_group_members to find a group ID. Only returns groups the current user belongs to. Takes no arguments.`}, func(ctx context.Context, _ struct{}) (string, error) {
		r, ok := tools.GetRunContext(ctx)
		if !ok {
			return "", fmt.Errorf("run context missing")
		}
		if err := tools.Authorize(ctx, guard, r, "list_my_groups", "external", ""); err != nil {
			return "", err
		}
		return backend.MyGroups(ctx, r)
	})
}
