// Package mcp connects the agent runtime to platform-registered MCP servers.
// Servers come from the registry table (never from user-submitted URLs), and
// every exposed tool is wrapped so each call passes the shared tool guard.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	mcpc "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"

	mcptool "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/port"
	"nonoka-im/internal/agent/tools"
	"nonoka-im/internal/authz"
)

// ServerStore resolves registry entries; data.MCPStore implements it.
type ServerStore interface {
	GetServer(ctx context.Context, name string) (*domain.MCPServer, error)
}

// Provider builds guarded Eino tools for the MCP bindings of one agent run.
// Clients are cached per server with bounded connection timeouts.
type Provider struct {
	Store   ServerStore
	Guard   tools.Guard
	Timeout time.Duration
	// IdleTTL closes MCP clients that have not been used for this long.
	IdleTTL time.Duration
	// MaxClients bounds the number of cached server connections.
	MaxClients int

	mu      sync.Mutex
	clients map[string]*clientEntry // server name → cached client + tools
}

type clientEntry struct {
	client   *mcpc.Client
	tools    map[string]tool.BaseTool // keyed by exposed name
	server   *domain.MCPServer
	lastUsed time.Time
}

// ExposedName is the runtime tool name for one MCP tool.
func ExposedName(server, toolName string) string {
	return tools.ExposedMCPName(server, toolName)
}

// Tools resolves every binding to a guarded tool. Bindings pointing at
// unknown, disabled or unlisted tools fail instead of silently dropping.
func (p *Provider) Tools(ctx context.Context, bindings []port.MCPTool) ([]tool.BaseTool, error) {
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
	exposed := ExposedName(serverName, toolName)
	if t, ok := entry.tools[exposed]; ok {
		return t, nil
	}
	return nil, fmt.Errorf("tool %q not bound on server %q", toolName, serverName)
}

// entry returns the cached client for a server only when the registry record
// still matches the snapshot the cache was built from. The record is re-read
// from the Store on every call so disabling, deleting or reconfiguring a
// server takes effect immediately instead of being served from a stale cache.
func (p *Provider) entry(ctx context.Context, serverName string) (*clientEntry, error) {
	server, err := p.Store.GetServer(ctx, serverName)
	if err != nil {
		if errors.Is(err, domain.ErrMCPNotFound) {
			p.evict(serverName)
		}
		return nil, err
	}
	if !server.Enabled {
		p.evict(serverName)
		return nil, fmt.Errorf("mcp server %q is disabled", serverName)
	}
	p.mu.Lock()
	if cached, ok := p.clients[serverName]; ok && sameServerConfig(cached.server, server) {
		cached.lastUsed = time.Now()
		p.mu.Unlock()
		return cached, nil
	}
	p.mu.Unlock()

	entry, err := p.connect(ctx, serverName, server)
	if err != nil {
		return nil, err
	}
	// Concurrent misses may both connect; the last write wins and the
	// displaced client is closed. Close happens outside the lock.
	var toClose []*clientEntry
	p.mu.Lock()
	if p.clients == nil {
		p.clients = make(map[string]*clientEntry)
	}
	if old := p.clients[serverName]; old != nil && old != entry {
		toClose = append(toClose, old)
	}
	if p.MaxClients > 0 && len(p.clients) >= p.MaxClients {
		var oldestName string
		var oldest time.Time
		for name, cached := range p.clients {
			if name == serverName {
				continue
			}
			if oldestName == "" || cached.lastUsed.Before(oldest) {
				oldestName, oldest = name, cached.lastUsed
			}
		}
		if oldestName != "" {
			toClose = append(toClose, p.clients[oldestName])
			delete(p.clients, oldestName)
		}
	}
	p.clients[serverName] = entry
	p.mu.Unlock()
	for _, old := range toClose {
		old.client.Close()
	}
	return entry, nil
}

// evict drops the cached client for a server (disabled or deleted from the
// registry) so it is neither served again nor resurrected on re-enable.
func (p *Provider) evict(serverName string) {
	p.mu.Lock()
	old := p.clients[serverName]
	delete(p.clients, serverName)
	p.mu.Unlock()
	if old != nil {
		old.client.Close()
	}
}

// EvictIdle closes clients unused for longer than IdleTTL. It returns the
// number of evicted connections.
func (p *Provider) EvictIdle(now time.Time) int {
	if p.IdleTTL <= 0 {
		return 0
	}
	p.mu.Lock()
	var stale []*clientEntry
	for name, entry := range p.clients {
		if now.Sub(entry.lastUsed) > p.IdleTTL {
			stale = append(stale, entry)
			delete(p.clients, name)
		}
	}
	p.mu.Unlock()
	for _, entry := range stale {
		entry.client.Close()
	}
	return len(stale)
}

// StartJanitor runs idle eviction until ctx is canceled. Call it in a
// goroutine from the composition root.
func (p *Provider) StartJanitor(ctx context.Context, interval time.Duration) {
	if p.IdleTTL <= 0 {
		return
	}
	if interval <= 0 {
		interval = max(p.IdleTTL/2, time.Minute)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			p.EvictIdle(now)
		}
	}
}

// Close closes every cached MCP client. It is safe to call at shutdown.
func (p *Provider) Close() {
	p.mu.Lock()
	entries := make([]*clientEntry, 0, len(p.clients))
	for name, entry := range p.clients {
		entries = append(entries, entry)
		delete(p.clients, name)
	}
	p.mu.Unlock()
	for _, entry := range entries {
		entry.client.Close()
	}
}

// sameServerConfig reports whether the freshly read registry record still
// matches the snapshot the cached client was built from. A drift in any
// connection-shaping field invalidates the cache.
func sameServerConfig(snapshot, fresh *domain.MCPServer) bool {
	return snapshot.Transport == fresh.Transport &&
		snapshot.URL == fresh.URL &&
		snapshot.AllowedTools == fresh.AllowedTools &&
		snapshot.RiskClass == fresh.RiskClass
}

// connect builds a fresh client + tool table for a validated registry record.
// It performs network I/O and must be called without p.mu held.
func (p *Provider) connect(ctx context.Context, serverName string, server *domain.MCPServer) (*clientEntry, error) {
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
	entry := &clientEntry{client: cli, tools: make(map[string]tool.BaseTool, len(rawTools)), server: server, lastUsed: time.Now()}
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
	return entry, nil
}

const maxResultChars = 64 << 10

// mcpToolCallTimeout bounds a single MCP tool call. It mirrors the per-call
// timeout of the built-in IM tools so one hung MCP request cannot consume the
// whole run budget.
const mcpToolCallTimeout = 20 * time.Second

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
		if errors.Is(err, authz.ErrDenied) {
			return "", fmt.Errorf("tool %s not authorized", g.exposed)
		}
		return "", &port.Error{Class: port.ClassTransientInfra, Err: err}
	}
	inner, ok := g.inner.(tool.InvokableTool)
	if !ok {
		return "", fmt.Errorf("mcp tool %s is not invokable", g.exposed)
	}
	// Bound one MCP call so a stuck server cannot eat the whole run budget.
	// WithTimeout keeps an earlier run-context deadline when there is one.
	ctx, cancel := context.WithTimeout(ctx, mcpToolCallTimeout)
	defer cancel()
	return inner.InvokableRun(ctx, argumentsInJSON, opts...)
}
