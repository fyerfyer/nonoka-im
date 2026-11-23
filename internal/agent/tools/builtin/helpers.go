package builtin

import "github.com/cloudwego/eino/components/tool"

// mustTool panics on a schema inference error. Built-in tool request types are
// static, so an inference failure is a programming error caught by tests and
// startup rather than a runtime condition.
func mustTool(t tool.InvokableTool, err error) tool.InvokableTool {
	if err != nil {
		panic(err)
	}
	return t
}
