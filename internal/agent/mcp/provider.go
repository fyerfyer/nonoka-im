// Package mcp connects the agent runtime to platform-registered MCP servers.
// Servers come from the registry table (never from user-submitted URLs), and
// every exposed tool is wrapped so each call passes the shared tool guard.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	mcpc "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"

	mcptool "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"nonoka-im/internal/agent/runtime"
	"nonoka-im/internal/agent/tools"
	"nonoka-im/internal/data"
)

// ServerStore resolves registry entries; data.MCPStore implements it.
type ServerStore interface {
	GetServer(ctx context.Context, name string) (*data.MCPServer, error)
}

// Provider builds guarded Eino tools for the MCP bindings of one agent run.
// Clients are cached per server with bounded connection timeouts.
type Provider struct {
	Store   ServerStore
	Guard   tools.Guard
	Timeout time.Duration

	mu      sync.Mutex
	clients map[string]*clientEntry // server name → cached client + tools
}

type clientEntry struct {
	client *mcpc.Client
	tools  map[string]tool.BaseTool // keyed by exposed name
	server *data.MCPServer
}

// ExposedName is the runtime tool name for one MCP tool.
func ExposedName(server, toolName string) string {
	return "mcp_" + server + "_" + toolName
}

// Tools resolves every binding to a guarded tool. Bindings pointing at
// unknown, disabled or unlisted tools fail instead of silently dropping.
func (p *Provider) Tools(ctx context.Context, bindings []runtime.MCPTool) ([]tool.BaseTool, error) {
	result := make([]tool.BaseTool, 0, len(bindings))
	for _, binding := range bindings {
		t, err := p.tool(ctx, binding.Server, binding.Tool)
		if err != nil {
			return nil, fmt.Errorf("mcp tool %s/%s: %w", binding.Server, binding.Tool, err)
		}
		result = append(result, t)
	}
	return result, nil
}

func (p *Provider) tool(ctx context.Context, serverName, toolName string) (tool.BaseTool, error) {
	entry, err := p.entry(ctx, serverName)
	if err != nil {
		return nil, err
	}
	if !entry.server.Enabled {
		return nil, fmt.Errorf("mcp server %q is disabled", serverName)
	}
	exposed := ExposedName(serverName, toolName)
	if t, ok := entry.tools[exposed]; ok {
		return t, nil
	}
	return nil, fmt.Errorf("tool %q not bound on server %q", toolName, serverName)
}

func (p *Provider) entry(ctx context.Context, serverName string) (*clientEntry, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.clients == nil {
		p.clients = make(map[string]*clientEntry)
	}
	if cached, ok := p.clients[serverName]; ok {
		return cached, nil
	}
	server, err := p.Store.GetServer(ctx, serverName)
	if err != nil {
		return nil, err
	}
	if !server.Enabled {
		return nil, fmt.Errorf("mcp server %q is disabled", serverName)
	}
	var allowed []string
	if err := json.Unmarshal([]byte(server.AllowedTools), &allowed); err != nil {
		return nil, fmt.Errorf("mcp server %q has invalid tool list", serverName)
	}
	if server.Transport != "" && server.Transport != "http" && server.Transport != "sse" {
		return nil, fmt.Errorf("unsupported mcp transport %q", server.Transport)
	}
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	cli, err := mcpc.NewStreamableHttpClient(server.URL, transport.WithHTTPTimeout(timeout))
	if err != nil {
		return nil, fmt.Errorf("connect mcp server %q: %w", serverName, err)
	}
	initCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if _, err := cli.Initialize(initCtx, mcp.InitializeRequest{}); err != nil {
		return nil, fmt.Errorf("initialize mcp server %q: %w", serverName, err)
	}
	listCtx, cancel2 := context.WithTimeout(ctx, timeout)
	defer cancel2()
	rawTools, err := mcptool.GetTools(listCtx, &mcptool.Config{
		Cli:          cli,
		ToolNameList: allowed,
		ToolCallResultHandler: func(_ context.Context, name string, result *mcp.CallToolResult) (*mcp.CallToolResult, error) {
			return truncateResult(result), nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("discover mcp server %q tools: %w", serverName, err)
	}
	entry := &clientEntry{client: cli, tools: make(map[string]tool.BaseTool, len(rawTools)), server: server}
	for _, t := range rawTools {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		allowedName := false
		for _, name := range allowed {
			if name == info.Name {
				allowedName = true
				break
			}
		}
		if !allowedName {
			continue
		}
		exposed := ExposedName(serverName, info.Name)
		entry.tools[exposed] = &guardedTool{inner: t, guard: p.Guard, exposed: exposed}
	}
	p.clients[serverName] = entry
	return entry, nil
}

const maxResultChars = 64 << 10

// truncateResult bounds MCP tool output before it reaches the model.
func truncateResult(result *mcp.CallToolResult) *mcp.CallToolResult {
	if result == nil {
		return result
	}
	for i, content := range result.Content {
		text, ok := content.(mcp.TextContent)
		if !ok || len(text.Text) <= maxResultChars {
			continue
		}
		runes := []rune(text.Text)
		if len(runes) > maxResultChars {
			runes = runes[:maxResultChars]
		}
		text.Text = string(runes) + "\n...[truncated]"
		result.Content[i] = text
	}
	return result
}

// guardedTool renames an MCP tool to its exposed name and authorizes every
// call through the shared guard. MCP tools carry no IM resource, so the
// resource scope is "external".
type guardedTool struct {
	inner   tool.BaseTool
	guard   tools.Guard
	exposed string
}

func (g *guardedTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	info, err := g.inner.Info(ctx)
	if err != nil || info == nil {
		return info, err
	}
	copied := *info
	copied.Name = g.exposed
	return &copied, nil
}

func (g *guardedTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	rc, ok := tools.GetRunContext(ctx)
	if !ok {
		return "", fmt.Errorf("run context missing")
	}
	if g.guard == nil {
		return "", fmt.Errorf("tool authorization unavailable")
	}
	if err := g.guard.CanExecuteTool(ctx, rc.ActorUserID, rc.AgentID, g.exposed, rc.Topic, "external", ""); err != nil {
		return "", fmt.Errorf("tool %s not authorized", g.exposed)
	}
	inner, ok := g.inner.(tool.InvokableTool)
	if !ok {
		return "", fmt.Errorf("mcp tool %s is not invokable", g.exposed)
	}
	return inner.InvokableRun(ctx, argumentsInJSON, opts...)
}
