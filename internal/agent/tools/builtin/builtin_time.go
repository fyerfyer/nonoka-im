package builtin

import (
	"context"
	"strconv"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

// newCurrentTimeTool is a hidden system capability. It reads no IM resource and
// never accepts a caller-supplied time, so it needs no guard/backend.
func newCurrentTimeTool() tool.InvokableTool {
	return utils.NewTool(&schema.ToolInfo{
		Name: "get_current_time",
		Desc: `Get the current date and time. Use this when the user asks about "today", "now", dates, deadlines or elapsed time. Takes no arguments.`,
	}, func(context.Context, struct{}) (string, error) {
		now := time.Now()
		return now.UTC().Format(time.RFC3339) + " (unix=" + strconv.FormatInt(now.Unix(), 10) + ")", nil
	})
}
