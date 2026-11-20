package catalog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuiltinDefinitionsAreComplete(t *testing.T) {
	defs := BuiltinDefinitions()
	if len(defs) != 8 {
		t.Fatalf("expected 8 built-in definitions, got %d", len(defs))
	}
	byName := map[string]Definition{}
	for _, def := range defs {
		byName[def.Name] = def
		if def.Key == "" || def.Category == "" || def.Risk == "" || def.DefaultPolicy == "" {
			t.Fatalf("incomplete definition: %+v", def)
		}
	}
	if !byName["send_message"].SideEffect {
		t.Fatal("send_message must be marked as a side effect")
	}
	if len(DefaultBootstrapToolNames()) != len(defs) {
		t.Fatal("bootstrap preset must cover the full built-in catalog")
	}
}

func TestSummarizersNeverExposeRawValues(t *testing.T) {
	byName := map[string]Definition{}
	for _, def := range BuiltinDefinitions() {
		byName[def.Name] = def
	}
	args := map[string]json.RawMessage{
		"content":  json.RawMessage(`"top-secret-message"`),
		"file_id":  json.RawMessage(`"file-123"`),
		"group_id": json.RawMessage(`"group-456"`),
	}
	send := byName["send_message"].Summarize(args)
	if strings.Contains(send, "top-secret-message") {
		t.Fatalf("send summary leaked content: %q", send)
	}
	read := byName["read_file"].Summarize(args)
	if !strings.Contains(read, "file-123") {
		t.Fatalf("read_file summary should identify the target: %q", read)
	}
	group := byName["get_group_members"].Summarize(args)
	if !strings.Contains(group, "group-456") {
		t.Fatalf("group summary should identify the target: %q", group)
	}
	generic := GenericSummary("mcp_demo_tool", map[string]json.RawMessage{
		"token": json.RawMessage(`"secret"`),
		"query": json.RawMessage(`"hello"`),
	})
	if strings.Contains(generic, "secret") || strings.Contains(generic, "hello") {
		t.Fatalf("generic summary leaked a value: %q", generic)
	}
	if !strings.Contains(generic, "token") || !strings.Contains(generic, "query") {
		t.Fatalf("generic summary must list argument keys: %q", generic)
	}
}
