package builtin

import (
	"nonoka-im/internal/agent/tools"

	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

func newGroupMembersTool(backend tools.Backend, guard tools.Guard, resultBudgetTokens int) tool.InvokableTool {
	return utils.NewTool(&schema.ToolInfo{Name: "get_group_members", Desc: `List the members of a group by its group ID: usernames and user IDs (up to 200 members).

Use this tool when the user asks who is in a specific group. You can only look up groups the current user belongs to; for other group IDs the tool fails without revealing whether the group exists.

Args:
- group_id: the ID of the group (from list_my_groups or the current conversation) (required).`}, func(ctx context.Context, in tools.GroupMembersRequest) (string, error) {
		r, ok := tools.GetRunContext(ctx)
		if !ok {
			return "", fmt.Errorf("run context missing")
		}
		if in.GroupID == "" {
			return "", fmt.Errorf("group_id is required")
		}
		if err := tools.Authorize(ctx, guard, r, "get_group_members", "external", ""); err != nil {
			return "", err
		}
		return backend.GroupMembers(ctx, r, in.GroupID)
	})
}
