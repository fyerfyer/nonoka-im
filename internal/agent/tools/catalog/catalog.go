// Package catalog is the single source of truth for built-in tool metadata:
// model-facing names, categories, risk, default approval policy, capability
// presets, input schema and the redacted approval summary strategy. It contains
// no handlers, so the management API and runtime adapter can share it without
// importing tool implementations.
package catalog

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Policy values accepted by the runtime and management API.
const (
	PolicyAuto = "auto"
	PolicyAsk  = "ask"
	PolicyDeny = "deny"
)

// Definition is the catalog entry for one tool.
type Definition struct {
	Key            string
	Name           string
	Description    string
	Category       string
	Risk           string
	DefaultPolicy  string
	DefaultEnabled bool
	System         bool
	Hidden         bool
	Advanced       bool
	SideEffect     bool
	Presets        []string
	InputSchema    json.RawMessage
	// Summarize builds a redacted, human-readable approval summary from parsed
	// arguments. It must never include raw values (except non-sensitive IDs
	// explicitly chosen by the tool author). nil falls back to GenericSummary.
	Summarize func(args map[string]json.RawMessage) string
}

// Preset is a named capability bundle shown to users instead of raw tools.
type Preset struct {
	Key            string   `json:"key"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	DefaultEnabled bool     `json:"defaultEnabled"`
	Tools          []string `json:"tools"`
}

// Catalog resolves metadata by model-facing tool name.
type Catalog interface {
	Definition(name string) (Definition, bool)
	Definitions() []Definition
}

var builtinPresets = []Preset{
	{
		Key: "im.read", Name: "IM read", DefaultEnabled: true,
		Description: "Read the current conversation, history, members and the actor's own profile.",
		Tools:       []string{"search_messages", "get_conversation_context", "read_file", "get_topic_info", "get_my_info", "list_my_groups", "get_group_members"},
	},
	{
		Key: "im.write", Name: "IM write", DefaultEnabled: false,
		Description: "Send messages. Writes stay behind approval unless the owner explicitly changes the policy.",
		Tools:       []string{"send_message"},
	},
	{
		Key: "system", Name: "System", DefaultEnabled: true,
		Description: "Platform-provided context tools that are always available.",
		Tools:       []string{"get_current_time"},
	},
}

// Presets returns the built-in capability presets in display order.
func Presets() []Preset {
	out := make([]Preset, len(builtinPresets))
	for i := range builtinPresets {
		out[i] = builtinPresets[i]
		out[i].Tools = append([]string(nil), builtinPresets[i].Tools...)
	}
	return out
}

// Preset resolves one preset by key.
func PresetByKey(key string) (Preset, bool) {
	for _, p := range builtinPresets {
		if p.Key == key {
			return p, true
		}
	}
	return Preset{}, false
}

// DefaultPresetKeys is the capability set of a newly created agent.
func DefaultPresetKeys() []string {
	out := make([]string, 0, len(builtinPresets))
	for _, p := range builtinPresets {
		if p.DefaultEnabled {
			out = append(out, p.Key)
		}
	}
	return out
}

// ResolveTools expands presets plus explicit tool names into a de-duplicated
// list. Explicit names remain supported for advanced users and older clients.
func ResolveTools(presets, explicit []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(explicit)+8)
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
	for _, key := range presets {
		preset, ok := PresetByKey(key)
		if !ok {
			return nil, fmt.Errorf("unknown capability preset %q", key)
		}
		for _, name := range preset.Tools {
			add(name)
		}
	}
	for _, name := range explicit {
		if _, ok := definitionByName(name); !ok {
			return nil, fmt.Errorf("unknown tool %q", name)
		}
		add(name)
	}
	// Hidden system tools are always injected; callers cannot disable them.
	for _, def := range BuiltinDefinitions() {
		if def.System {
			add(def.Name)
		}
	}
	return out, nil
}

// BuiltinDefinitions is the single catalog of built-in IM tools.
func BuiltinDefinitions() []Definition {
	return []Definition{
		{
			Key: "search_messages", Name: "search_messages", Description: "Search the current conversation history.",
			Category: "im.read", Risk: "read_only", DefaultPolicy: PolicyAuto, DefaultEnabled: true,
			Presets: []string{"im.read"}, InputSchema: schema(`{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer"},"since":{"type":"integer"},"sender_id":{"type":"integer"}},"required":["query"]}`),
		},
		{
			Key: "get_conversation_context", Name: "get_conversation_context", Description: "Read recent messages in the current conversation.",
			Category: "im.read", Risk: "read_only", DefaultPolicy: PolicyAuto, DefaultEnabled: true,
			Presets: []string{"im.read"}, InputSchema: schema(`{"type":"object","properties":{"limit":{"type":"integer"}}}`),
		},
		{
			Key: "send_message", Name: "send_message", Description: "Send a message to the current conversation.",
			Category: "im.write", Risk: "write", DefaultPolicy: PolicyAsk, SideEffect: true,
			Presets: []string{"im.write"}, InputSchema: schema(`{"type":"object","properties":{"content":{"type":"string"}},"required":["content"]}`),
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
			Category: "im.read", Risk: "read_only", DefaultPolicy: PolicyAuto, DefaultEnabled: true,
			Presets: []string{"im.read"}, InputSchema: schema(`{"type":"object","properties":{"file_id":{"type":"string"}},"required":["file_id"]}`),
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
			Category: "im.read", Risk: "read_only", DefaultPolicy: PolicyAuto, DefaultEnabled: true,
			Presets: []string{"im.read"}, InputSchema: schema(`{"type":"object","properties":{}}`),
		},
		{
			Key: "get_my_info", Name: "get_my_info", Description: "Read the actor's own profile.",
			Category: "im.read", Risk: "read_only", DefaultPolicy: PolicyAuto, DefaultEnabled: true,
			Presets: []string{"im.read"}, InputSchema: schema(`{"type":"object","properties":{}}`),
		},
		{
			Key: "list_my_groups", Name: "list_my_groups", Description: "List the groups the actor belongs to.",
			Category: "im.read", Risk: "read_only", DefaultPolicy: PolicyAuto, DefaultEnabled: true,
			Presets: []string{"im.read"}, InputSchema: schema(`{"type":"object","properties":{}}`),
		},
		{
			Key: "get_group_members", Name: "get_group_members", Description: "List members of a group the actor belongs to.",
			Category: "im.read", Risk: "read_only", DefaultPolicy: PolicyAuto, DefaultEnabled: true,
			Presets: []string{"im.read"}, InputSchema: schema(`{"type":"object","properties":{"group_id":{"type":"string"}},"required":["group_id"]}`),
			Summarize: func(args map[string]json.RawMessage) string {
				var groupID string
				if json.Unmarshal(args["group_id"], &groupID) == nil && groupID != "" {
					return fmt.Sprintf("list members of group %s", groupID)
				}
				return "list group members"
			},
		},
		{
			Key: "get_current_time", Name: "get_current_time", Description: "Get the current date and time in UTC and Unix seconds. This is a hidden system tool.",
			Category: "system", Risk: "read_only", DefaultPolicy: PolicyAuto, DefaultEnabled: true,
			System: true, Hidden: true, Presets: []string{"system"},
			InputSchema: schema(`{"type":"object","properties":{}}`),
		},
	}
}

func schema(raw string) json.RawMessage { return json.RawMessage(raw) }

func definitionByName(name string) (Definition, bool) {
	for _, def := range BuiltinDefinitions() {
		if def.Name == name || def.Key == name {
			return def, true
		}
	}
	return Definition{}, false
}

// Policy resolves the effective policy for a tool. Explicit overrides win;
// otherwise the catalog default applies (write tools default to ask).
func Policy(def Definition, overrides map[string]string) string {
	if p, ok := overrides[def.Name]; ok {
		switch p {
		case PolicyAuto, PolicyAsk, PolicyDeny:
			return p
		}
	}
	if def.DefaultPolicy != "" {
		return def.DefaultPolicy
	}
	if def.SideEffect || def.Risk == "write" {
		return PolicyAsk
	}
	return PolicyAuto
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

// SystemToolNames returns hidden tools that must always be available.
func SystemToolNames() []string {
	out := make([]string, 0, 2)
	for _, def := range BuiltinDefinitions() {
		if def.System {
			out = append(out, def.Name)
		}
	}
	return out
}
