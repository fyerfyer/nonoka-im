// Package builtin implements the built-in IM tools. The catalog metadata lives
// in tools/catalog; this package only binds handlers to concrete backends.
package builtin

import (
	"nonoka-im/internal/agent/tools"
	"nonoka-im/internal/agent/tools/catalog"

	"github.com/cloudwego/eino/components/tool"
)

// New builds the default built-in tool set.
func New(backend tools.Backend, guard tools.Guard, budgets ...int) []tool.InvokableTool {
	resultBudgetTokens := 8192
	if len(budgets) > 0 {
		resultBudgetTokens = budgets[0]
	}
	if resultBudgetTokens <= 0 {
		resultBudgetTokens = 8192
	}
	return []tool.InvokableTool{
		newSearchTool(backend, guard, resultBudgetTokens),
		newConversationTool(backend, guard, resultBudgetTokens),
		newSendTool(backend, guard, resultBudgetTokens),
		newReadFileTool(backend, guard, resultBudgetTokens),
		newTopicInfoTool(backend, guard, resultBudgetTokens),
		newMyInfoTool(backend, guard, resultBudgetTokens),
		newMyGroupsTool(backend, guard, resultBudgetTokens),
		newGroupMembersTool(backend, guard, resultBudgetTokens),
		newCurrentTimeTool(),
	}
}

// NewRegistry binds the catalog to the built-in handlers.
func NewRegistry(backend tools.Backend, guard tools.Guard, budgets ...int) *tools.Registry {
	return tools.NewRegistry(catalog.BuiltinDefinitions(), New(backend, guard, budgets...))
}

func limitItems(items []string, tokenBudget int) []string {
	return tools.BoundStrings(items, tokenBudget*4)
}

func isTextMime(mime string) bool {
	return mime == "" || mime == "text/plain" || mime == "text/markdown" || mime == "application/json" ||
		len(mime) > 5 && (mime[:5] == "text/" || mime == "application/xml" || mime == "application/javascript" || mime == "application/x-yaml")
}
