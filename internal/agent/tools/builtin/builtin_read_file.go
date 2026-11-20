package builtin

import (
	"nonoka-im/internal/agent/tools"

	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

func newReadFileTool(backend tools.Backend, guard tools.Guard, resultBudgetTokens int) tool.InvokableTool {
	return utils.NewTool(&schema.ToolInfo{Name: "read_file", Desc: `Read the content of an attachment (file or image) by its file ID, as long as the user who triggered you is allowed to see it.

Use this tool when the user asks about the contents of a file shared in this conversation, e.g. "what does the attached log say". Text files are returned as text; other files return metadata only.

Args:
- file_id: the attachment ID from the message that shared the file (required).`}, func(ctx context.Context, in tools.ReadFileRequest) (tools.ReadFileResult, error) {
		r, ok := tools.GetRunContext(ctx)
		if !ok {
			return tools.ReadFileResult{}, fmt.Errorf("run context missing")
		}
		if in.FileID == "" {
			return tools.ReadFileResult{}, fmt.Errorf("file_id is required")
		}
		if err := tools.Authorize(ctx, guard, r, "read_file", "file", in.FileID); err != nil {
			return tools.ReadFileResult{}, err
		}
		const maxBytes = 256 << 10
		name, mime, content, truncated, err := backend.ReadFile(ctx, r, in.FileID, maxBytes)
		if err != nil {
			return tools.ReadFileResult{}, err
		}
		result := tools.ReadFileResult{Name: name, Mime: mime, Truncated: truncated}
		if isTextMime(mime) {
			result.Content = string(content)
		} else {
			result.Content = fmt.Sprintf("[%s attachment: %d bytes, content not shown]", mime, len(content))
		}
		return result, nil
	})
}
