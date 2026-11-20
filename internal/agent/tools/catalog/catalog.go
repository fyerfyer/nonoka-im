// Package catalog is the single source of truth for built-in tool metadata:
// model-facing names, categories, risk, default approval policy, side-effect
// classification and the redacted approval summary strategy. It contains no
// handlers, so the management API and runtime adapter can share it without
// importing tool implementations.
package catalog

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Definition is the catalog entry for one tool.
type Definition struct {
	Key           string
	Name          string
	Description   string
	Category      string
	Risk          string
	DefaultPolicy string
	System        bool
	Hidden        bool
	SideEffect    bool
	// Summarize builds a redacted, human-readable approval summary from parsed
	// arguments. It must never include raw values (except non-sensitive IDs
	// explicitly chosen by the tool author). nil falls back to GenericSummary.
	Summarize func(args map[string]json.RawMessage) string
}

// Catalog resolves metadata by model-facing tool name.
type Catalog interface {
	Definition(name string) (Definition, bool)
	Definitions() []Definition
}

// BuiltinDefinitions is the single catalog of built-in IM tools.
func BuiltinDefinitions() []Definition {
	return []Definition{
		{
			Key: "search_messages", Name: "search_messages", Description: "Search the current conversation history.",
			Category: "im.read", Risk: "read_only", DefaultPolicy: "auto",
		},
		{
			Key: "get_conversation_context", Name: "get_conversation_context", Description: "Read recent messages in the current conversation.",
			Category: "im.read", Risk: "read_only", DefaultPolicy: "auto",
		},
		{
			Key: "send_message", Name: "send_message", Description: "Send a message to the current conversation.",
			Category: "im.write", Risk: "write", DefaultPolicy: "ask", SideEffect: true,
			Summarize: func(args map[string]json.RawMessage) string {
				var content string
				if json.Unmarshal(args["content"], &content) == nil {
					return fmt.Sprintf("send a message to the current conversation (%d chars)", len(content))
				}
				return "send a message to the current conversation"
			},
		},
		{
			Key: "read_file", Name: "read_file", Description: "Read an attachment the actor may access.",
			Category: "im.read", Risk: "read_only", DefaultPolicy: "auto",
			Summarize: func(args map[string]json.RawMessage) string {
				var fileID string
				if json.Unmarshal(args["file_id"], &fileID) == nil && fileID != "" {
					return fmt.Sprintf("read file %s", fileID)
				}
				return "read an attachment"
			},
		},
		{
			Key: "get_topic_info", Name: "get_topic_info", Description: "Describe the current conversation and its members.",
			Category: "im.read", Risk: "read_only", DefaultPolicy: "auto",
		},
		{
			Key: "get_my_info", Name: "get_my_info", Description: "Read the actor's own profile.",
			Category: "im.read", Risk: "read_only", DefaultPolicy: "auto",
		},
		{
			Key: "list_my_groups", Name: "list_my_groups", Description: "List the groups the actor belongs to.",
			Category: "im.read", Risk: "read_only", DefaultPolicy: "auto",
		},
		{
			Key: "get_group_members", Name: "get_group_members", Description: "List members of a group the actor belongs to.",
			Category: "im.read", Risk: "read_only", DefaultPolicy: "auto",
			Summarize: func(args map[string]json.RawMessage) string {
				var groupID string
				if json.Unmarshal(args["group_id"], &groupID) == nil && groupID != "" {
					return fmt.Sprintf("list members of group %s", groupID)
				}
				return "list group members"
			},
		},
	}
}

// GenericSummary is the safe fallback for tools without a custom summarizer
// (including dynamically discovered MCP tools): it lists argument keys only.
func GenericSummary(toolName string, args map[string]json.RawMessage) string {
	if len(args) == 0 {
		return fmt.Sprintf("call %s (no arguments)", toolName)
	}
	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > 12 {
		keys = keys[:12]
	}
	return fmt.Sprintf("call %s (arguments: %s)", toolName, strings.Join(keys, ", "))
}

// DefaultBootstrapToolNames is the preset agent's built-in capability set.
func DefaultBootstrapToolNames() []string {
	defs := BuiltinDefinitions()
	out := make([]string, 0, len(defs))
	for _, def := range defs {
		out = append(out, def.Name)
	}
	return out
}
