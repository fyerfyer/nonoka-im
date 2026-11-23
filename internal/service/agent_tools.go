package service

import (
	"context"
	"encoding/json"
	"net/http"

	"nonoka-im/internal/agent/tools/catalog"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

type ToolDefinitionView struct {
	Key            string          `json:"key"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Category       string          `json:"category"`
	Risk           string          `json:"risk"`
	DefaultPolicy  string          `json:"defaultPolicy"`
	DefaultEnabled bool            `json:"defaultEnabled"`
	System         bool            `json:"system"`
	Hidden         bool            `json:"hidden"`
	Advanced       bool            `json:"advanced"`
	SideEffect     bool            `json:"sideEffect"`
	Presets        []string        `json:"presets"`
	InputSchema    json.RawMessage `json:"inputSchema,omitempty"`
}

// AgentTools returns the built-in Tool Registry and capability presets. It is
// the single discovery endpoint shared by the management UI and SDKs.
func (s *AgentHTTP) AgentTools(c khttp.Context) error {
	return s.handle(c, func(_ context.Context, w http.ResponseWriter, _ *http.Request) error {
		defs := catalog.BuiltinDefinitions()
		tools := make([]ToolDefinitionView, 0, len(defs))
		for _, def := range defs {
			tools = append(tools, ToolDefinitionView{
				Key: def.Key, Name: def.Name, Description: def.Description,
				Category: def.Category, Risk: def.Risk, DefaultPolicy: def.DefaultPolicy,
				DefaultEnabled: def.DefaultEnabled, System: def.System, Hidden: def.Hidden,
				Advanced: def.Advanced, SideEffect: def.SideEffect, Presets: def.Presets,
				InputSchema: def.InputSchema,
			})
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{
			"presets": catalog.Presets(),
			"tools":   tools,
		})
		return nil
	})
}
