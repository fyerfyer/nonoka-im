package builtin

import (
	"nonoka-im/internal/agent/tools"

	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

func newMyInfoTool(backend tools.Backend, guard tools.Guard, resultBudgetTokens int) tool.InvokableTool {
	return utils.NewTool(&schema.ToolInfo{Name: "get_my_info", Desc: `Get the current user's own profile: user ID, username and account creation time.

Use this tool when the user asks who they are, what their account is, or what their own user ID or username is. Returns only the profile of the user you are talking to. Takes no arguments.`}, func(ctx context.Context, _ struct{}) (string, error) {
		r, ok := tools.GetRunContext(ctx)
		if !ok {
			return "", fmt.Errorf("run context missing")
		}
		if err := tools.Authorize(ctx, guard, r, "get_my_info", "external", ""); err != nil {
			return "", err
		}
		return backend.MyInfo(ctx, r)
	})
}
