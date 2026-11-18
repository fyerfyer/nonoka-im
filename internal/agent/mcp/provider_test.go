package mcp

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	einotool "github.com/cloudwego/eino/components/tool"
	"nonoka-im/internal/agent/runtime"
	"nonoka-im/internal/agent/tools"
	"nonoka-im/internal/data"
)

type fakeStore struct{ server *data.MCPServer }

// GetServer returns a copy so tests exercise the provider re-reading the
// registry instead of observing mutations through a shared pointer.
func (f *fakeStore) GetServer(_ context.Context, name string) (*data.MCPServer, error) {
	if f.server == nil || f.server.Name != name {
		return nil, data.ErrMCPNotFound
	}
	snap := *f.server
	return &snap, nil
}

type guardRecorder struct{ calls []string }

func (g *guardRecorder) CanExecuteTool(_ context.Context, _, _ int64, tool, _, _, _ string) error {
	g.calls = append(g.calls, tool)
	return nil
}

// newFixtureServer starts a read-only MCP server with one tool, "echo", that
// returns its input text prefixed with "echo:". A second tool "hidden" exists
// on the server but must never be exposed unless the registry allows it.
func newFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := server.NewMCPServer("fixture", "1.0.0")
	echo := mcpgo.NewTool("echo",
		mcpgo.WithDescription("Echo the input text back"),
		mcpgo.WithString("text", mcpgo.Required(), mcpgo.Description("Text to echo")),
	)
	s.AddTool(echo, func(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		text, _ := req.RequireString("text")
		if len(text) > 1024 {
			text = text[:1024]
		}
		return mcpgo.NewToolResultText("echo:" + text), nil
	})
	hidden := mcpgo.NewTool("hidden", mcpgo.WithDescription("Not exposed"))
	s.AddTool(hidden, func(_ context.Context, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return mcpgo.NewToolResultText("hidden result"), nil
	})
	httpServer := server.NewStreamableHTTPServer(s)
	srv := httptest.NewServer(httpServer)
	t.Cleanup(srv.Close)
	return srv
}

func registryEntry(t *testing.T, srv *httptest.Server, tools ...string) *data.MCPServer {
	t.Helper()
	raw, _ := json.Marshal(tools)
	return &data.MCPServer{Name: "fixture", Transport: "http", URL: srv.URL, AllowedTools: string(raw), RiskClass: "read_only", Enabled: true}
}

func TestProviderExposesOnlyBoundTools(t *testing.T) {
	srv := newFixtureServer(t)
	store := &fakeStore{server: registryEntry(t, srv, "echo")}
	p := &Provider{Store: store, Guard: &guardRecorder{}, Timeout: 5 * time.Second}

	ts, err := p.Tools(context.Background(), []runtime.MCPTool{{Server: "fixture", Tool: "echo"}})
	if err != nil || len(ts) != 1 {
		t.Fatalf("tools: %v, %v", ts, err)
	}
	info, err := ts[0].Info(context.Background())
	if err != nil || info.Name != "mcp_fixture_echo" {
		t.Fatalf("exposed name: %+v, %v", info, err)
	}

	// "hidden" is not in the registry allowlist, so binding it must fail.
	if _, err := p.Tools(context.Background(), []runtime.MCPTool{{Server: "fixture", Tool: "hidden"}}); err == nil {
		t.Fatalf("unlisted tool exposed")
	}
	if _, err := p.Tools(context.Background(), []runtime.MCPTool{{Server: "fixture", Tool: "missing"}}); err == nil {
		t.Fatalf("unknown tool exposed")
	}

	// A disabled server refuses tool resolution.
	store.server.Enabled = false
	if _, err := p.Tools(context.Background(), []runtime.MCPTool{{Server: "fixture", Tool: "echo"}}); err == nil {
		t.Fatalf("disabled server still serves tools")
	}
}

// TestProviderInvalidatesCachedClient verifies the provider re-reads the
// registry on every Tools call: a cached client must stop serving tools when
// the server is disabled or deleted, and must be rebuilt when the registry
// configuration (allowlist) changes.
func TestProviderInvalidatesCachedClient(t *testing.T) {
	srv := newFixtureServer(t)
	store := &fakeStore{server: registryEntry(t, srv, "echo")}
	p := &Provider{Store: store, Guard: &guardRecorder{}, Timeout: 5 * time.Second}

	if _, err := p.Tools(context.Background(), []runtime.MCPTool{{Server: "fixture", Tool: "echo"}}); err != nil {
		t.Fatal(err)
	}

	// Disable after a successful resolution: the stale cache must not serve.
	store.server.Enabled = false
	if _, err := p.Tools(context.Background(), []runtime.MCPTool{{Server: "fixture", Tool: "echo"}}); err == nil {
		t.Fatal("disabled server still serves cached tools")
	}

	// Re-enable with an expanded allowlist: the client reconnects and the
	// newly allowed tool becomes resolvable.
	store.server.Enabled = true
	store.server.AllowedTools = `["echo","hidden"]`
	ts, err := p.Tools(context.Background(), []runtime.MCPTool{{Server: "fixture", Tool: "hidden"}})
	if err != nil || len(ts) != 1 {
		t.Fatalf("config change did not rebuild tools: %v, %v", ts, err)
	}

	// Delete the server: cached tools must fail closed.
	store.server = nil
	if _, err := p.Tools(context.Background(), []runtime.MCPTool{{Server: "fixture", Tool: "echo"}}); err == nil {
		t.Fatal("deleted server still serves cached tools")
	}
}

func TestGuardedToolAuthorizesEveryCall(t *testing.T) {
	srv := newFixtureServer(t)
	store := &fakeStore{server: registryEntry(t, srv, "echo")}
	guard := &guardRecorder{}
	p := &Provider{Store: store, Guard: guard, Timeout: 5 * time.Second}
	ts, err := p.Tools(context.Background(), []runtime.MCPTool{{Server: "fixture", Tool: "echo"}})
	if err != nil {
		t.Fatal(err)
	}
	inner, ok := ts[0].(interface {
		InvokableRun(ctx context.Context, argumentsInJSON string, opts ...einotool.Option) (string, error)
	})
	if !ok {
		t.Fatalf("tool not invokable")
	}
	ctx := tools.WithRunContext(context.Background(), tools.RunContext{ActorUserID: 7, AgentID: 9, Topic: "p2p_7_9"})
	out, err := inner.InvokableRun(ctx, `{"text":"hello"}`)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !strings.Contains(out, "echo:hello") {
		t.Fatalf("unexpected result %q", out)
	}
	if len(guard.calls) != 1 || guard.calls[0] != "mcp_fixture_echo" {
		t.Fatalf("guard calls: %v", guard.calls)
	}
	// Without a trusted run context the call is rejected.
	if _, err := inner.InvokableRun(context.Background(), `{"text":"x"}`); err == nil {
		t.Fatalf("invoke without run context allowed")
	}
}

func TestProviderFailsWithoutGuard(t *testing.T) {
	srv := newFixtureServer(t)
	store := &fakeStore{server: registryEntry(t, srv, "echo")}
	p := &Provider{Store: store, Timeout: 5 * time.Second}
	ts, err := p.Tools(context.Background(), []runtime.MCPTool{{Server: "fixture", Tool: "echo"}})
	if err != nil {
		t.Fatal(err)
	}
	inner, ok := ts[0].(interface {
		InvokableRun(ctx context.Context, argumentsInJSON string, opts ...einotool.Option) (string, error)
	})
	if !ok {
		t.Fatalf("tool not invokable")
	}
	ctx := tools.WithRunContext(context.Background(), tools.RunContext{ActorUserID: 7, AgentID: 9})
	if _, err := inner.InvokableRun(ctx, `{"text":"x"}`); err == nil {
		t.Fatalf("invoke without guard allowed")
	}
}
