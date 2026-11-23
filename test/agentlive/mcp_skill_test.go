package agentlive

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

	"github.com/cloudwego/eino/adk/middlewares/skill"
	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/eino"
	agentmcp "nonoka-im/internal/agent/mcp"
	"nonoka-im/internal/agent/port"
	agentskills "nonoka-im/internal/agent/skills"
)

type liveStore struct {
	server *domain.MCPServer
	tools  map[string]*domain.MCPTool
	skills map[uint64]*domain.SkillVersion
}

func (f *liveStore) GetServerByID(_ context.Context, id uint64) (*domain.MCPServer, error) {
	if f.server == nil || f.server.ID != id {
		return nil, domain.ErrMCPNotFound
	}
	return f.server, nil
}

func (f *liveStore) GetTool(_ context.Context, serverID uint64, name string) (*domain.MCPTool, error) {
	if row, ok := f.tools[name]; ok && row.ServerID == serverID {
		return row, nil
	}
	return nil, domain.ErrMCPToolNotFound
}

func (f *liveStore) GetConnection(context.Context, uint64, int64) (*domain.MCPConnection, error) {
	return nil, domain.ErrMCPConnectionNotFound
}

func (f *liveStore) TouchConnection(context.Context, uint64) error { return nil }

func (f *liveStore) GetSkillVersion(_ context.Context, skillID uint64, version int64) (*domain.SkillVersion, error) {
	row, ok := f.skills[skillID]
	if !ok || row.Version != version {
		return nil, domain.ErrSkillVersionNotFound
	}
	return row, nil
}

type allowAllGuard struct{}

func (allowAllGuard) CanExecuteTool(context.Context, int64, int64, string, string, string, string) error {
	return nil
}

func startEchoFixture(t *testing.T) *httptest.Server {
	t.Helper()
	s := server.NewMCPServer("live-fixture", "1.0.0")
	s.AddTool(mcpgo.NewTool("echo",
		mcpgo.WithDescription("Echo the input text back with an echo: prefix"),
		mcpgo.WithString("text", mcpgo.Required(), mcpgo.Description("Text to echo")),
		mcpgo.WithReadOnlyHintAnnotation(true),
	), func(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		text, _ := req.RequireString("text")
		return mcpgo.NewToolResultText("echo:" + text), nil
	})
	srv := httptest.NewServer(server.NewStreamableHTTPServer(s))
	t.Cleanup(srv.Close)
	return srv
}

// TestDeepSeekMCPToolUse drives the real model to call a bound MCP tool and
// cite its result.
func TestDeepSeekMCPToolUse(t *testing.T) {
	key, base, model := liveConfig(t)
	srv := startEchoFixture(t)
	hash := sha256.Sum256([]byte("echo-schema"))
	store := &liveStore{
		server: &domain.MCPServer{
			ID: 1, OwnerUserID: 7, Name: "live", Transport: domain.MCPTransportHTTP, URL: srv.URL,
			AuthType: domain.MCPAuthNone, Visibility: domain.VisibilityPrivate,
			ReviewStatus: domain.ReviewApproved, Enabled: true, ToolsRevision: 1,
		},
		tools: map[string]*domain.MCPTool{
			"echo": {ServerID: 1, Name: "echo", Risk: domain.MCPRiskReadOnly, SchemaHash: hex.EncodeToString(hash[:]), Enabled: true},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	provider := &agentmcp.Provider{
		Store: store, Guard: &agentmcp.Guard{AllowPrivate: true, AllowInsecure: true},
		ToolGuard: allowAllGuard{}, Timeout: 10 * time.Second,
	}
	rt, err := eino.New(ctx, eino.Deps{MCPToolProvider: provider.Tools})
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := provider.Tools(ctx, []port.MCPTool{{ServerID: 1, Tool: "echo"}})
	if err != nil || len(resolved) != 1 {
		t.Fatalf("resolve mcp tools: %v, %v", resolved, err)
	}
	echoTool := resolved[0]
	info, _ := echoTool.Info(ctx)
	if info.Name != "mcp_1_echo" {
		t.Fatalf("unexpected exposed name %q", info.Name)
	}

	in := port.Input{
		Instruction: "You can use the tool mcp_1_echo to echo text. Use it exactly once, then answer with the tool output.",
		Text:        "Please echo the word marigold-bot and tell me the exact tool output.",
		Model:       liveEndpoint(key, base, model),
		MCPTools:    []port.MCPTool{{ServerID: 1, Tool: "echo"}},
	}
	res, err := rt.Run(ctx, in)
	if err != nil {
		t.Fatalf("run with mcp tools: %v", err)
	}
	if !strings.Contains(strings.ToLower(res.Text), "echo:marigold-bot") {
		t.Fatalf("model did not cite the MCP tool result: %q", res.Text)
	}
	t.Logf("DeepSeek MCP acceptance passed: model=%s, tool=mcp_1_echo", model)
}

// TestDeepSeekSkillUse binds a platform skill and verifies the model loads it
// on demand and applies its instruction.
func TestDeepSeekSkillUse(t *testing.T) {
	key, base, model := liveConfig(t)
	store := &liveStore{skills: map[uint64]*domain.SkillVersion{
		1: {
			SkillID: 1, Version: 1, Status: domain.SkillVersionPublished,
			Content: "---\nname: codeword-format\ndescription: Format codeword answers\n---\nWhen the user asks for the codeword, answer in the form 'CODEWORD=<value>' and nothing else.",
		},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	rt, err := eino.New(ctx, eino.Deps{
		SkillBackendFactory: func(bindings []port.SkillBinding) (skill.Backend, error) {
			return agentskills.NewBackend(store, bindings), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	in := port.Input{
		Instruction: "Follow the codeword-format skill when the user asks for the codeword.",
		Text:        "The codeword is marigold-9. What is the codeword?",
		Model:       liveEndpoint(key, base, model),
		Skills:      []port.SkillBinding{{SkillID: 1, Name: "codeword-format", Version: 1}},
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
