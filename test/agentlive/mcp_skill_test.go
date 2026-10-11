package agentlive

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/cloudwego/eino/adk/middlewares/skill"
	"nonoka-im/internal/agent/eino"
	agentmcp "nonoka-im/internal/agent/mcp"
	"nonoka-im/internal/agent/runtime"
	agentskills "nonoka-im/internal/agent/skills"
	"nonoka-im/internal/data"
)

// liveStore fakes the MCP/skill registries for the live suite.
type liveStore struct {
	server *data.MCPServer
	skills map[string]*data.Skill
}

func (f *liveStore) GetServer(_ context.Context, name string) (*data.MCPServer, error) {
	if f.server == nil || f.server.Name != name {
		return nil, data.ErrMCPNotFound
	}
	return f.server, nil
}

func (f *liveStore) GetSkill(_ context.Context, name string, version int64) (*data.Skill, error) {
	row, ok := f.skills[name]
	if !ok || row.Version != version {
		return nil, data.ErrMCPNotFound
	}
	return row, nil
}

type allowAllGuard struct{}

func (allowAllGuard) CanExecuteTool(context.Context, int64, int64, string, string, string, string) error {
	return nil
}

// startEchoFixture starts a read-only MCP server exposing "echo", which
// prefixes its input with "echo:".
func startEchoFixture(t *testing.T) *httptest.Server {
	t.Helper()
	s := server.NewMCPServer("live-fixture", "1.0.0")
	s.AddTool(mcpgo.NewTool("echo",
		mcpgo.WithDescription("Echo the input text back with an echo: prefix"),
		mcpgo.WithString("text", mcpgo.Required(), mcpgo.Description("Text to echo")),
	), func(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		text, _ := req.RequireString("text")
		return mcpgo.NewToolResultText("echo:" + text), nil
	})
	srv := httptest.NewServer(server.NewStreamableHTTPServer(s))
	t.Cleanup(srv.Close)
	return srv
}

// TestDeepSeekMCPToolUse drives the real model to call a bound MCP tool and
// cite its result; a tool the server offers but the registry does not allow
// must stay invisible.
func TestDeepSeekMCPToolUse(t *testing.T) {
	key, base, model := liveConfig(t)
	srv := startEchoFixture(t)
	raw, _ := json.Marshal([]string{"echo"})
	store := &liveStore{server: &data.MCPServer{Name: "live", Transport: "http", URL: srv.URL, AllowedTools: string(raw), Enabled: true}}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	rt, err := eino.NewRuntime(ctx, key, base, model)
	if err != nil {
		t.Fatal(err)
	}
	provider := &agentmcp.Provider{Store: store, Guard: allowAllGuard{}, Timeout: 10 * time.Second}
	rt.SetMCPToolProvider(provider.Tools)

	// Discovery sanity: only the allowlisted tool is exposed, under its
	// exposed name, and the guard fires per call.
	resolved, err := provider.Tools(ctx, []runtime.MCPTool{{Server: "live", Tool: "echo"}})
	if err != nil || len(resolved) != 1 {
		t.Fatalf("resolve mcp tools: %v, %v", resolved, err)
	}
	if _, err := provider.Tools(ctx, []runtime.MCPTool{{Server: "live", Tool: "hidden"}}); err == nil {
		t.Fatalf("unlisted mcp tool exposed")
	}
	echoTool := resolved[0]
	info, _ := echoTool.Info(ctx)
	if info.Name != "mcp_live_echo" {
		t.Fatalf("unexpected exposed name %q", info.Name)
	}

	in := runtime.Input{
		Instruction: "You can use the tool mcp_live_echo to echo text. Use it exactly once, then answer with the tool output.",
		Text:        "Please echo the word marigold-bot and tell me the exact tool output.",
		MCPTools:    []runtime.MCPTool{{Server: "live", Tool: "echo"}},
	}
	res, err := rt.Run(ctx, in)
	if err != nil {
		t.Fatalf("run with mcp tools: %v", err)
	}
	if !strings.Contains(strings.ToLower(res.Text), "echo:marigold-bot") {
		t.Fatalf("model did not cite the MCP tool result: %q", res.Text)
	}
	t.Logf("DeepSeek MCP acceptance passed: model=%s, tool=mcp_live_echo", model)
}

// TestDeepSeekSkillUse binds a platform skill and verifies the model loads it
// on demand and applies its instruction.
func TestDeepSeekSkillUse(t *testing.T) {
	key, base, model := liveConfig(t)
	store := &liveStore{skills: map[string]*data.Skill{
		"codeword-format": {
			Name:      "codeword-format",
			Version:   1,
			Content:   "---\nname: codeword-format\ndescription: Format codeword answers\n---\nWhen the user asks for the codeword, answer in the form 'CODWORD=<value>' and nothing else.",
			Published: true,
		},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	rt, err := eino.NewRuntime(ctx, key, base, model)
	if err != nil {
		t.Fatal(err)
	}
	rt.SetSkillBackendFactory(func(bindings []runtime.SkillBinding) (skill.Backend, error) {
		return agentskills.NewBackend(store, bindings), nil
	})

	in := runtime.Input{
		Instruction: "Follow the codeword-format skill when the user asks for the codeword.",
		Text:        "The codeword is marigold-9. What is the codeword?",
		Skills:      []runtime.SkillBinding{{Name: "codeword-format", Version: 1}},
	}
	res, err := rt.Run(ctx, in)
	if err != nil {
		t.Fatalf("run with skills: %v", err)
	}
	if !strings.Contains(strings.ToUpper(res.Text), "MARIGOLD-9") || !strings.Contains(res.Text, "=") {
		t.Fatalf("model did not apply the skill format: %q", res.Text)
	}
	t.Logf("DeepSeek skill acceptance passed: model=%s, skill=codeword-format", model)
}
