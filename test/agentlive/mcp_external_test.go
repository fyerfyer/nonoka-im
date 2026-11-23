package agentlive

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/eino"
	agentmcp "nonoka-im/internal/agent/mcp"
	"nonoka-im/internal/agent/port"
)

// recordingGuard records which MCP tools the runtime actually executed.
type recordingGuard struct {
	mu    sync.Mutex
	calls []string
}

func (g *recordingGuard) CanExecuteTool(_ context.Context, _, _ int64, tool, _, _, _ string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls = append(g.calls, tool)
	return nil
}

func (g *recordingGuard) called(tool string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, call := range g.calls {
		if call == tool {
			return true
		}
	}
	return false
}

// TestExternalMCPDiscovery exercises real public MCP servers over Streamable
// HTTP. It is gated because CI should not depend on third-party availability.
func TestExternalMCPDiscovery(t *testing.T) {
	if os.Getenv("NONOKA_MCP_LIVE") != "1" {
		t.Skip("external MCP suite disabled; set NONOKA_MCP_LIVE=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	provider := &agentmcp.Provider{Guard: &agentmcp.Guard{}, Timeout: 30 * time.Second}

	cases := []struct {
		name     string
		url      string
		expected string
	}{
		{name: "deepwiki", url: "https://mcp.deepwiki.com/mcp", expected: "read_wiki_structure"},
		{name: "context7", url: "https://mcp.context7.com/mcp", expected: "resolve-library-id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := provider.Discover(ctx, domain.MCPTarget{
				URL: tc.url, Transport: domain.MCPTransportHTTP, AuthType: domain.MCPAuthNone,
			})
			if err != nil {
				t.Fatalf("discover %s: %v", tc.url, err)
			}
			if len(res.Tools) == 0 {
				t.Fatalf("discover %s: no tools", tc.url)
			}
			found := false
			for _, tool := range res.Tools {
				if tool.Name == tc.expected {
					found = true
				}
				if tool.SchemaHash == "" || tool.InputSchema == "" {
					t.Fatalf("tool %s missing schema metadata", tool.Name)
				}
			}
			if !found {
				t.Fatalf("discover %s: expected tool %q not found", tc.url, tc.expected)
			}
			t.Logf("discovered %d tools from %s", len(res.Tools), tc.url)
		})
	}
}

// TestDeepSeekCallsExternalMCPTool verifies that a real model can see and call
// a discovered DeepWiki MCP tool through the same provider used by the worker.
func TestDeepSeekCallsExternalMCPTool(t *testing.T) {
	if os.Getenv("NONOKA_MCP_LIVE") != "1" {
		t.Skip("external MCP suite disabled; set NONOKA_MCP_LIVE=1")
	}
	key, base, model := liveConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	discovery := &agentmcp.Provider{Guard: &agentmcp.Guard{}, Timeout: 30 * time.Second}
	res, err := discovery.Discover(ctx, domain.MCPTarget{
		URL: "https://mcp.deepwiki.com/mcp", Transport: domain.MCPTransportHTTP, AuthType: domain.MCPAuthNone,
	})
	if err != nil {
		t.Fatalf("discover deepwiki: %v", err)
	}
	server := &domain.MCPServer{
		ID: 1, OwnerUserID: 7, Name: "deepwiki", DisplayName: "DeepWiki",
		Transport: domain.MCPTransportHTTP, URL: "https://mcp.deepwiki.com/mcp",
		AuthType: domain.MCPAuthNone, Visibility: domain.VisibilityPrivate,
		ReviewStatus: domain.ReviewApproved, Enabled: true, ToolsRevision: 1,
	}
	toolsBy := make(map[string]*domain.MCPTool, len(res.Tools))
	for i := range res.Tools {
		tool := res.Tools[i]
		toolsBy[tool.Name] = &tool
	}
	store := &externalMCPStore{server: server, tools: toolsBy}
	guard := &recordingGuard{}
	provider := &agentmcp.Provider{
		Store: store, Guard: &agentmcp.Guard{}, ToolGuard: guard, Timeout: 30 * time.Second,
	}
	rt, err := eino.New(ctx, eino.Deps{MCPToolProvider: provider.Tools})
	if err != nil {
		t.Fatal(err)
	}
	echoName := domain.MCPExposedName(1, "ask_wiki_question")
	in := port.Input{
		Instruction: "Use the bound DeepWiki MCP tool exactly once, then answer with a one-sentence summary.",
		Text:        "Ask DeepWiki: what is the CloudWeGo Eino repository about? Use repoName cloudwego/eino.",
		Model:       liveEndpoint(key, base, model),
		ActorUserID: 7, AgentID: 9, Topic: "p2p_7_9",
		MCPTools: []port.MCPTool{{
			ServerID: 1, Tool: "ask_wiki_question", Exposed: echoName,
			SchemaHash: toolsBy["ask_wiki_question"].SchemaHash, Policy: "auto",
		}},
	}
	answer, err := rt.Run(ctx, in)
	if err != nil {
		t.Fatalf("run with external MCP: %v", err)
	}
	if !guard.called(echoName) {
		t.Fatalf("model did not call %s; answer=%q calls=%v", echoName, answer.Text, guard.calls)
	}
	if len(answer.Text) == 0 {
		t.Fatalf("empty final answer")
	}
	t.Logf("DeepSeek called external MCP tool %s: %s", echoName, answer.Text)
}

type externalMCPStore struct {
	server *domain.MCPServer
	tools  map[string]*domain.MCPTool
}

func (s *externalMCPStore) GetServerByID(context.Context, uint64) (*domain.MCPServer, error) {
	return s.server, nil
}

func (s *externalMCPStore) GetTool(_ context.Context, _ uint64, name string) (*domain.MCPTool, error) {
	tool, ok := s.tools[name]
	if !ok {
		return nil, domain.ErrMCPToolNotFound
	}
	return tool, nil
}

func (s *externalMCPStore) GetConnection(context.Context, uint64, int64) (*domain.MCPConnection, error) {
	return nil, fmt.Errorf("%w: no connection", domain.ErrMCPConnectionNotFound)
}

func (s *externalMCPStore) TouchConnection(context.Context, uint64) error { return nil }
