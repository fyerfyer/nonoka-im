package tools

import (
	"context"

	"nonoka-im/internal/agent/tools/catalog"

	"github.com/cloudwego/eino/components/tool"
)

// Registry binds a catalog to concrete tool instances.
type Registry struct {
	defs  []catalog.Definition
	tools []tool.InvokableTool
}

// NewRegistry builds a registry from catalog entries and concrete tools.
func NewRegistry(defs []catalog.Definition, ts []tool.InvokableTool) *Registry {
	return &Registry{defs: defs, tools: ts}
}

// Definitions returns a copy of the catalog.
func (r *Registry) Definitions() []catalog.Definition {
	out := make([]catalog.Definition, len(r.defs))
	copy(out, r.defs)
	return out
}

// Definition resolves one catalog entry by model-facing name.
func (r *Registry) Definition(name string) (catalog.Definition, bool) {
	for _, def := range r.defs {
		if def.Name == name {
			return def, true
		}
	}
	return catalog.Definition{}, false
}

// Tools returns every registered tool.
func (r *Registry) Tools() []tool.InvokableTool {
	out := make([]tool.InvokableTool, len(r.tools))
	copy(out, r.tools)
	return out
}

// Resolve returns the tools whose model-facing names are in names.
func (r *Registry) Resolve(ctx context.Context, names []string) []tool.BaseTool {
	if len(names) == 0 {
		return nil
	}
	wanted := make(map[string]bool, len(names))
	for _, name := range names {
		wanted[name] = true
	}
	out := make([]tool.BaseTool, 0, len(names))
	for _, t := range r.tools {
		info, err := t.Info(ctx)
		if err == nil && info != nil && wanted[info.Name] {
			out = append(out, t)
		}
	}
	return out
}
