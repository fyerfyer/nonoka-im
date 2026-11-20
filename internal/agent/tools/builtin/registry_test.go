package builtin

import (
	"context"
	"testing"
)

func TestBuiltinDefinitionsMatchRegistry(t *testing.T) {
	reg := NewRegistry(&backendStub{}, &guardStub{})
	defs := reg.Definitions()
	if len(defs) != 8 {
		t.Fatalf("expected 8 definitions, got %d", len(defs))
	}
	tools := reg.Tools()
	if len(tools) != 8 {
		t.Fatalf("expected 8 tools, got %d", len(tools))
	}
	seen := map[string]bool{}
	for _, def := range defs {
		seen[def.Name] = true
	}
	for _, tl := range tools {
		info, err := tl.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !seen[info.Name] {
			t.Fatalf("tool %q has no definition", info.Name)
		}
	}
	if len(reg.Resolve(context.Background(), []string{"send_message"})) != 1 {
		t.Fatal("resolve did not filter to send_message")
	}
	if len(reg.Resolve(context.Background(), nil)) != 0 {
		t.Fatal("empty binding list must resolve to no tools")
	}
}
