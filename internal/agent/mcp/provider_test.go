package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	einotool "github.com/cloudwego/eino/components/tool"
	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/port"
	"nonoka-im/internal/agent/tools"
)

type fakeStore struct {
	server      *domain.MCPServer
	tools       map[string]*domain.MCPTool
	connections map[int64]*domain.MCPConnection
}

func (f *fakeStore) GetServerByID(_ context.Context, id uint64) (*domain.MCPServer, error) {
	if f.server == nil || f.server.ID != id {
		return nil, domain.ErrMCPNotFound
	}
	snap := *f.server
	return &snap, nil
}

func (f *fakeStore) GetTool(_ context.Context, serverID uint64, name string) (*domain.MCPTool, error) {
	if t, ok := f.tools[name]; ok && t.ServerID == serverID {
		snap := *t
		return &snap, nil
	}
	return nil, domain.ErrMCPToolNotFound
}

func (f *fakeStore) GetConnection(_ context.Context, serverID uint64, userID int64) (*domain.MCPConnection, error) {
	if c, ok := f.connections[userID]; ok && c.ServerID == serverID {
		snap := *c
		return &snap, nil
	}
	return nil, domain.ErrMCPConnectionNotFound
}

func (f *fakeStore) TouchConnection(context.Context, uint64) error { return nil }

type guardRecorder struct{ calls []string }

func (g *guardRecorder) CanExecuteTool(_ context.Context, _, _ int64, tool, _, _, _ string) error {
	g.calls = append(g.calls, tool)
	return nil
}

func newFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := server.NewMCPServer("fixture", "1.0.0")
	echo := mcpgo.NewTool("echo",
		mcpgo.WithDescription("Echo the input text back"),
		mcpgo.WithString("text", mcpgo.Required(), mcpgo.Description("Text to echo")),
		mcpgo.WithReadOnlyHintAnnotation(true),
	)
	s.AddTool(echo, func(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		text, _ := req.RequireString("text")
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

func fixtureStore(t *testing.T, srv *httptest.Server) *fakeStore {
	t.Helper()
	server := &domain.MCPServer{
		ID: 1, OwnerUserID: 7, Name: "fixture", DisplayName: "Fixture",
		Transport: domain.MCPTransportHTTP, URL: srv.URL, AuthType: domain.MCPAuthNone,
		Visibility: domain.VisibilityPrivate, ReviewStatus: domain.ReviewApproved,
		Enabled: true, ToolsRevision: 1,
	}
	hash := sha256.Sum256([]byte("schema"))
	return &fakeStore{
		server: server,
		tools: map[string]*domain.MCPTool{
			"echo":   {ServerID: 1, Name: "echo", Risk: domain.MCPRiskReadOnly, SchemaHash: hex.EncodeToString(hash[:]), Enabled: true},
			"hidden": {ServerID: 1, Name: "hidden", Risk: domain.MCPRiskWrite, SchemaHash: "other", Enabled: true},
		},
		connections: map[int64]*domain.MCPConnection{},
	}
}

func testGuard() *Guard {
	return &Guard{AllowPrivate: true, AllowInsecure: true}
}

func TestProviderExposesOnlyBoundTools(t *testing.T) {
	srv := newFixtureServer(t)
	store := fixtureStore(t, srv)
	p := &Provider{Store: store, Guard: testGuard(), ToolGuard: &guardRecorder{}, Timeout: 5 * time.Second}

	ctx := tools.WithRunContext(context.Background(), tools.RunContext{ActorUserID: 7, AgentID: 9, Topic: "p2p_7_9"})
	ts, err := p.Tools(ctx, []port.MCPTool{{ServerID: 1, Tool: "echo"}})
	if err != nil || len(ts) != 1 {
		t.Fatalf("tools: %v, %v", ts, err)
	}
	info, err := ts[0].Info(ctx)
	if err != nil || info.Name != "mcp_1_echo" {
		t.Fatalf("exposed name: %+v, %v", info, err)
	}
	if _, err := p.Tools(ctx, []port.MCPTool{{ServerID: 1, Tool: "missing"}}); err == nil {
		t.Fatalf("unknown tool exposed")
	}
	store.server.Enabled = false
	if _, err := p.Tools(ctx, []port.MCPTool{{ServerID: 1, Tool: "echo"}}); err == nil {
		t.Fatalf("disabled server still serves tools")
	}
}

func TestProviderInvalidatesCachedClient(t *testing.T) {
	srv := newFixtureServer(t)
	store := fixtureStore(t, srv)
	p := &Provider{Store: store, Guard: testGuard(), ToolGuard: &guardRecorder{}, Timeout: 5 * time.Second}
	ctx := tools.WithRunContext(context.Background(), tools.RunContext{ActorUserID: 7, AgentID: 9})
	if _, err := p.Tools(ctx, []port.MCPTool{{ServerID: 1, Tool: "echo"}}); err != nil {
		t.Fatal(err)
	}
	store.server.Enabled = false
	if _, err := p.Tools(ctx, []port.MCPTool{{ServerID: 1, Tool: "echo"}}); err == nil {
		t.Fatal("disabled server still serves cached tools")
	}
	store.server.Enabled = true
	store.server.ToolsRevision = 2
	if _, err := p.Tools(ctx, []port.MCPTool{{ServerID: 1, Tool: "hidden"}}); err != nil {
		t.Fatalf("config change did not rebuild tools: %v", err)
	}
	store.server = nil
	if _, err := p.Tools(ctx, []port.MCPTool{{ServerID: 1, Tool: "echo"}}); err == nil {
		t.Fatal("deleted server still serves cached tools")
	}
}

func TestProviderEvictsIdleClients(t *testing.T) {
	srv := newFixtureServer(t)
	store := fixtureStore(t, srv)
	p := &Provider{Store: store, Guard: testGuard(), ToolGuard: &guardRecorder{}, Timeout: 5 * time.Second, IdleTTL: time.Minute}
	ctx := tools.WithRunContext(context.Background(), tools.RunContext{ActorUserID: 7, AgentID: 9})
	if _, err := p.Tools(ctx, []port.MCPTool{{ServerID: 1, Tool: "echo"}}); err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	before := len(p.clients)
	p.mu.Unlock()
	if before != 1 {
		t.Fatalf("expected one cached client, got %d", before)
	}
	if evicted := p.EvictIdle(time.Now().Add(2 * time.Minute)); evicted != 1 {
		t.Fatalf("expected one idle eviction, got %d", evicted)
	}
	p.Close()
}

func TestGuardedToolAuthorizesEveryCall(t *testing.T) {
	srv := newFixtureServer(t)
	store := fixtureStore(t, srv)
	guard := &guardRecorder{}
	p := &Provider{Store: store, Guard: testGuard(), ToolGuard: guard, Timeout: 5 * time.Second}
	ctx := tools.WithRunContext(context.Background(), tools.RunContext{ActorUserID: 7, AgentID: 9, Topic: "p2p_7_9"})
	ts, err := p.Tools(ctx, []port.MCPTool{{ServerID: 1, Tool: "echo"}})
	if err != nil {
		t.Fatal(err)
	}
	inner, ok := ts[0].(interface {
		InvokableRun(ctx context.Context, argumentsInJSON string, opts ...einotool.Option) (string, error)
	})
	if !ok {
		t.Fatalf("tool not invokable")
	}
	out, err := inner.InvokableRun(ctx, `{"text":"hello"}`)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !strings.Contains(out, "echo:hello") {
		t.Fatalf("unexpected result %q", out)
	}
	if len(guard.calls) != 1 || guard.calls[0] != "mcp_1_echo" {
		t.Fatalf("guard calls: %v", guard.calls)
	}
	if _, err := inner.InvokableRun(context.Background(), `{"text":"x"}`); err == nil {
		t.Fatalf("invoke without run context allowed")
	}
}

func TestProviderFailsWithoutGuard(t *testing.T) {
	srv := newFixtureServer(t)
	store := fixtureStore(t, srv)
	p := &Provider{Store: store, Guard: testGuard(), Timeout: 5 * time.Second}
	ctx := tools.WithRunContext(context.Background(), tools.RunContext{ActorUserID: 7, AgentID: 9})
	ts, err := p.Tools(ctx, []port.MCPTool{{ServerID: 1, Tool: "echo"}})
	if err != nil {
		t.Fatal(err)
	}
	inner := ts[0].(interface {
		InvokableRun(ctx context.Context, argumentsInJSON string, opts ...einotool.Option) (string, error)
	})
	if _, err := inner.InvokableRun(ctx, `{"text":"x"}`); err == nil {
		t.Fatalf("invoke without guard allowed")
	}
}

func TestDiscoverReturnsSchemasAndRisk(t *testing.T) {
	srv := newFixtureServer(t)
	p := &Provider{Guard: testGuard(), Timeout: 5 * time.Second}
	res, err := p.Discover(context.Background(), domain.MCPTarget{URL: srv.URL, Transport: domain.MCPTransportHTTP, AuthType: domain.MCPAuthNone})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(res.Tools))
	}
	for _, tool := range res.Tools {
		if tool.SchemaHash == "" || tool.InputSchema == "" {
			t.Fatalf("tool metadata incomplete: %+v", tool)
		}
		if tool.Name == "echo" && tool.Risk != domain.MCPRiskReadOnly {
			t.Fatalf("read-only annotation not honored: %+v", tool)
		}
	}
}

func TestSSRFGuardRejectsPrivateTargets(t *testing.T) {
	g := &Guard{}
	for _, raw := range []string{"http://127.0.0.1:1/mcp", "http://169.254.169.254/mcp", "https://10.0.0.5/mcp"} {
		if err := g.ValidateURL(raw); err == nil {
			t.Fatalf("private target accepted: %s", raw)
		}
	}
	if err := g.ValidateURL("https://example.com/mcp"); err != nil {
		t.Fatalf("public https rejected: %v", err)
	}
}

func TestAuthHeadersSupportsBearerAndCustomHeader(t *testing.T) {
	bearer, err := authHeaders(domain.MCPAuthBearer, "secret-token")
	if err != nil || bearer["Authorization"] != "Bearer secret-token" {
		t.Fatalf("bearer headers: %+v, %v", bearer, err)
	}
	header, err := authHeaders(domain.MCPAuthHeader, `{"name":"X-API-Key","value":"header-secret"}`)
	if err != nil || header["X-API-Key"] != "header-secret" {
		t.Fatalf("custom header: %+v, %v", header, err)
	}
	if _, err := authHeaders(domain.MCPAuthBearer, ""); err == nil {
		t.Fatal("empty bearer token accepted")
	}
	if _, err := authHeaders(domain.MCPAuthHeader, `{"name":"bad header","value":"x"}`); err == nil {
		t.Fatal("invalid header name accepted")
	}
}
