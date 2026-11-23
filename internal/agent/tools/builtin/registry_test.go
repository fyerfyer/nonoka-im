package builtin

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

func TestBuiltinDefinitionsMatchRegistry(t *testing.T) {
	reg := NewRegistry(&backendStub{}, &guardStub{})
	defs := reg.Definitions()
	if len(defs) != 9 {
		t.Fatalf("expected 9 definitions, got %d", len(defs))
	}
	tools := reg.Tools()
	if len(tools) != 9 {
		t.Fatalf("expected 9 tools, got %d", len(tools))
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

func TestCatalogSchemasMatchEinoInferredSchemas(t *testing.T) {
	reg := NewRegistry(&backendStub{}, &guardStub{})
	for _, tl := range reg.Tools() {
		info, err := tl.Info(context.Background())
		if err != nil || info == nil {
			t.Fatalf("info: %v", err)
		}
		actual := map[string]any{}
		if info.ParamsOneOf != nil {
			js, err := info.ParamsOneOf.ToJSONSchema()
			if err != nil {
				t.Fatalf("tool %s schema: %v", info.Name, err)
			}
			actualRaw, _ := json.Marshal(js)
			_ = json.Unmarshal(actualRaw, &actual)
		}
		def, _ := reg.Definition(info.Name)
		var wanted map[string]any
		_ = json.Unmarshal(def.InputSchema, &wanted)
		if keys(actual["properties"]) != keys(wanted["properties"]) {
			t.Fatalf("tool %s property mismatch: eino=%v catalog=%v", info.Name, keys(actual["properties"]), keys(wanted["properties"]))
		}
		if keys(actual["required"]) != keys(wanted["required"]) {
			t.Fatalf("tool %s required mismatch: eino=%v catalog=%v", info.Name, keys(actual["required"]), keys(wanted["required"]))
		}
	}
}

func keys(v any) string {
	m, _ := v.(map[string]any)
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}
