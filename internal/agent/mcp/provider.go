// Package mcp connects the agent runtime to platform-registered MCP servers.
// It also implements discovery, SSRF-safe transport and credential isolation
// for the management usecases.
package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

// Store resolves registry entries and per-user connections.
type Store interface {
	GetServerByID(ctx context.Context, id uint64) (*domain.MCPServer, error)
	GetTool(ctx context.Context, serverID uint64, name string) (*domain.MCPTool, error)
	GetConnection(ctx context.Context, serverID uint64, userID int64) (*domain.MCPConnection, error)
	TouchConnection(ctx context.Context, id uint64) error
}

// Sealer decrypts stored MCP credentials.
type Sealer interface {
	Open(sealed string) (string, error)
}

// Target aliases the domain connection target.
type Target = domain.MCPTarget

// DiscoveryResult aliases the domain discovery result.
type DiscoveryResult = domain.MCPDiscoveryResult

// Provider builds guarded Eino tools for the MCP bindings of one agent run and
// provides discovery/health primitives for management usecases.
type Provider struct {
	Store     Store
	Sealer    Sealer
	Guard     *Guard
	ToolGuard tools.Guard
	Timeout   time.Duration
	// IdleTTL closes MCP clients that have not been used for this long.
	IdleTTL time.Duration
	// MaxClients bounds the number of cached connections.
	MaxClients int

	mu      sync.Mutex
	clients map[string]*clientEntry
}

type clientEntry struct {
	client   *mcpc.Client
	tools    map[string]tool.BaseTool // keyed by raw MCP tool name
	server   *domain.MCPServer
	authHash string
	lastUsed time.Time
}

// ExposedName is the runtime tool name for one MCP tool.
func ExposedName(serverID uint64, toolName string) string {
	return domain.MCPExposedName(serverID, toolName)
}

// Tools resolves every binding to a guarded tool. Bindings pointing at
// unknown, disabled, stale or unauthorized tools fail instead of silently
// dropping.
func (p *Provider) Tools(ctx context.Context, bindings []port.MCPTool) ([]tool.BaseTool, error) {
	result := make([]tool.BaseTool, 0, len(bindings))
	for _, binding := range bindings {
		if binding.ServerID == 0 || binding.Tool == "" {
			return nil, fmt.Errorf("invalid mcp binding")
		}
		current, err := p.Store.GetTool(ctx, binding.ServerID, binding.Tool)
		if err != nil {
			return nil, fmt.Errorf("mcp tool %d/%s: %w", binding.ServerID, binding.Tool, err)
		}
		if binding.SchemaHash != "" && current.SchemaHash != binding.SchemaHash {
			return nil, fmt.Errorf("mcp tool %d/%s schema changed; republish the agent", binding.ServerID, binding.Tool)
		}
		entry, err := p.entry(ctx, binding.ServerID)
		if err != nil {
			return nil, err
		}
		raw, ok := entry.tools[binding.Tool]
		if !ok {
			return nil, fmt.Errorf("tool %q not found on mcp server %d", binding.Tool, binding.ServerID)
		}
		exposed := binding.Exposed
		if exposed == "" {
			exposed = ExposedName(binding.ServerID, binding.Tool)
		}
		result = append(result, &guardedTool{inner: raw, guard: p.ToolGuard, exposed: exposed})
	}
	return result, nil
}

// Discover connects with the supplied target credential (or none) and returns
// the complete tools/list snapshot. It never reads the database.
func (p *Provider) Discover(ctx context.Context, target Target) (*DiscoveryResult, error) {
	if err := p.validateTarget(target); err != nil {
		return nil, err
	}
	cli, listed, _, err := p.connect(ctx, target)
	if err != nil {
		if isAuthError(err) {
			return &DiscoveryResult{NeedsAuth: true}, domain.ErrMCPAuthRequired
		}
		return nil, err
	}
	defer cli.Close()
	out := make([]domain.MCPTool, 0, len(listed))
	for _, t := range listed {
		input, _ := json.Marshal(t.InputSchema)
		output, _ := json.Marshal(t.OutputSchema)
		annotations, _ := json.Marshal(t.Annotations)
		hash := sha256.Sum256(append(append(input, output...), annotations...))
		risk := domain.MCPRiskWrite
		if t.Annotations.ReadOnlyHint != nil && *t.Annotations.ReadOnlyHint {
			// readOnly wins: MCP SDKs default destructiveHint to true, so a
			// read-only tool would otherwise be misclassified as a write.
			risk = domain.MCPRiskReadOnly
		} else if t.Annotations.DestructiveHint != nil && *t.Annotations.DestructiveHint {
			risk = domain.MCPRiskWrite
		}
		out = append(out, domain.MCPTool{
			Name: t.Name, Description: t.Description,
			InputSchema: string(input), OutputSchema: string(output), Annotations: string(annotations),
			Risk: risk, SchemaHash: hex.EncodeToString(hash[:]), Enabled: true,
			DiscoveredAt: time.Now(), UpdatedAt: time.Now(),
		})
	}
	return &DiscoveryResult{Tools: out}, nil
}

// Test performs initialize + tools/list and reports whether the target works.
func (p *Provider) Test(ctx context.Context, target Target) error {
	if err := p.validateTarget(target); err != nil {
		return err
	}
	cli, _, _, err := p.connect(ctx, target)
	if err != nil {
		return err
	}
	return cli.Close()
}

func (p *Provider) validateTarget(target Target) error {
	if target.URL == "" {
		return fmt.Errorf("MCP server URL is required")
	}
	if target.Transport != "" && target.Transport != domain.MCPTransportHTTP {
		return fmt.Errorf("unsupported MCP transport %q", target.Transport)
	}
	if p.Guard == nil {
		return fmt.Errorf("MCP network guard unavailable")
	}
	if err := p.Guard.ValidateURL(target.URL); err != nil {
		return fmt.Errorf("%w: %v", domain.ErrInvalidMCPTarget, err)
	}
	return nil
}

func (p *Provider) entry(ctx context.Context, serverID uint64) (*clientEntry, error) {
	run, _ := tools.GetRunContext(ctx)
	server, err := p.Store.GetServerByID(ctx, serverID)
	if err != nil {
		if errors.Is(err, domain.ErrMCPNotFound) {
			p.evict(serverID)
		}
		return nil, err
	}
	if !server.Enabled {
		p.evict(serverID)
		return nil, fmt.Errorf("mcp server %q is disabled", server.Name)
	}
	if server.Visibility != domain.VisibilityPrivate && server.ReviewStatus != domain.ReviewApproved {
		return nil, domain.ErrMCPReviewRequired
	}
	if server.Visibility == domain.VisibilityPrivate && run.ActorUserID != server.OwnerUserID && run.ActorUserID != 0 {
		return nil, fmt.Errorf("mcp server %q is private", server.Name)
	}
	target, err := p.targetForActor(ctx, server, run.ActorUserID)
	if err != nil {
		return nil, err
	}
	authHash := sha256.Sum256([]byte(target.AuthType + "\x00" + target.Credential))
	key := fmt.Sprintf("%d:%x", serverID, authHash[:8])
	p.mu.Lock()
	if cached, ok := p.clients[key]; ok && sameServerConfig(cached.server, server, cached.authHash, hex.EncodeToString(authHash[:])) {
		cached.lastUsed = time.Now()
		p.mu.Unlock()
		return cached, nil
	}
	p.mu.Unlock()

	cli, _, rawTools, err := p.connect(ctx, target)
	if err != nil {
		if isAuthError(err) {
			return nil, domain.ErrMCPAuthRequired
		}
		return nil, err
	}
	entry := &clientEntry{client: cli, tools: make(map[string]tool.BaseTool, len(rawTools)), server: server, authHash: hex.EncodeToString(authHash[:]), lastUsed: time.Now()}
	for _, t := range rawTools {
		if info, ierr := t.Info(ctx); ierr == nil && info != nil {
			entry.tools[info.Name] = t
		}
	}
	var toClose []*clientEntry
	p.mu.Lock()
	if p.clients == nil {
		p.clients = make(map[string]*clientEntry)
	}
	if old := p.clients[key]; old != nil && old != entry {
		toClose = append(toClose, old)
	}
	if p.MaxClients > 0 && len(p.clients) >= p.MaxClients {
		var oldestKey string
		var oldest time.Time
		for k, cached := range p.clients {
			if k == key {
				continue
			}
			if oldestKey == "" || cached.lastUsed.Before(oldest) {
				oldestKey, oldest = k, cached.lastUsed
			}
		}
		if oldestKey != "" {
			toClose = append(toClose, p.clients[oldestKey])
			delete(p.clients, oldestKey)
		}
	}
	p.clients[key] = entry
	p.mu.Unlock()
	for _, old := range toClose {
		old.client.Close()
	}
	return entry, nil
}

// targetForActor enforces credential isolation: the owner may use the server
// credential, everyone else must have a personal connection when auth is
// required.
func (p *Provider) targetForActor(ctx context.Context, server *domain.MCPServer, actorID int64) (Target, error) {
	target := Target{URL: server.URL, Transport: server.Transport, AuthType: server.AuthType}
	if server.AuthType == "" || server.AuthType == domain.MCPAuthNone {
		return target, nil
	}
	if actorID > 0 && actorID == server.OwnerUserID && server.CredentialEnc != "" {
		cred, err := p.openCredential(server.CredentialEnc)
		if err != nil {
			return Target{}, err
		}
		target.Credential = cred
		return target, nil
	}
	conn, err := p.Store.GetConnection(ctx, server.ID, actorID)
	if err != nil {
		if errors.Is(err, domain.ErrMCPConnectionNotFound) {
			return Target{}, domain.ErrMCPAuthRequired
		}
		return Target{}, err
	}
	if conn.Status != "connected" {
		return Target{}, domain.ErrMCPAuthRequired
	}
	cred, err := p.openCredential(conn.CredentialEnc)
	if err != nil {
		return Target{}, err
	}
	target.Credential = cred
	if conn.ID > 0 {
		_ = p.Store.TouchConnection(context.WithoutCancel(ctx), conn.ID)
	}
	return target, nil
}

func (p *Provider) openCredential(sealed string) (string, error) {
	if p.Sealer == nil {
		return "", fmt.Errorf("MCP credential sealer unavailable")
	}
	return p.Sealer.Open(sealed)
}

func (p *Provider) connect(ctx context.Context, target Target) (*mcpc.Client, []mcp.Tool, []tool.BaseTool, error) {
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	headers, err := authHeaders(target.AuthType, target.Credential)
	if err != nil {
		return nil, nil, nil, err
	}
	httpClient := p.Guard.HTTPClient(timeout)
	opts := []transport.StreamableHTTPCOption{
		transport.WithHTTPTimeout(timeout),
		transport.WithHTTPBasicClient(httpClient),
	}
	if len(headers) > 0 {
		opts = append(opts, transport.WithHTTPHeaders(headers))
	}
	cli, err := mcpc.NewStreamableHttpClient(target.URL, opts...)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("connect mcp server: %w", err)
	}
	initCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if _, err := cli.Initialize(initCtx, mcp.InitializeRequest{}); err != nil {
		cli.Close()
		return nil, nil, nil, fmt.Errorf("initialize mcp server: %w", err)
	}
	listCtx, cancel2 := context.WithTimeout(ctx, timeout)
	defer cancel2()
	listed, err := cli.ListTools(listCtx, mcp.ListToolsRequest{})
	if err != nil {
		cli.Close()
		return nil, nil, nil, fmt.Errorf("list mcp tools: %w", err)
	}
	names := make([]string, 0, len(listed.Tools))
	for _, t := range listed.Tools {
		names = append(names, t.Name)
	}
	rawTools, err := mcptool.GetTools(listCtx, &mcptool.Config{
		Cli:          cli,
		ToolNameList: names,
		ToolCallResultHandler: func(_ context.Context, _ string, result *mcp.CallToolResult) (*mcp.CallToolResult, error) {
			return truncateResult(result), nil
		},
	})
	if err != nil {
		cli.Close()
		return nil, nil, nil, fmt.Errorf("discover mcp tools: %w", err)
	}
	return cli, listed.Tools, rawTools, nil
}

func authHeaders(authType, credential string) (map[string]string, error) {
	switch authType {
	case "", domain.MCPAuthNone:
		return nil, nil
	case domain.MCPAuthBearer:
		if strings.TrimSpace(credential) == "" {
			return nil, domain.ErrMCPAuthRequired
		}
		return map[string]string{"Authorization": "Bearer " + credential}, nil
	case domain.MCPAuthHeader:
		var spec struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		}
		if err := json.Unmarshal([]byte(credential), &spec); err != nil || spec.Name == "" || spec.Value == "" {
			return nil, fmt.Errorf("MCP header credential must be {\"name\":\"X-...\",\"value\":\"...\"}")
		}
		if !validHeaderName(spec.Name) {
			return nil, fmt.Errorf("invalid MCP header name")
		}
		return map[string]string{spec.Name: spec.Value}, nil
	default:
		return nil, fmt.Errorf("unsupported MCP auth type %q", authType)
	}
}

func validHeaderName(name string) bool {
	for _, r := range name {
		if !(r == '-' || r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return name != ""
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, domain.ErrMCPAuthRequired) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "401") || strings.Contains(msg, "403") ||
		strings.Contains(msg, "unauthorized") || strings.Contains(msg, "forbidden")
}

// evict drops cached clients for a server (disabled/deleted/reconfigured).
func (p *Provider) evict(serverID uint64) {
	prefix := fmt.Sprintf("%d:", serverID)
	p.mu.Lock()
	var stale []*clientEntry
	for key, entry := range p.clients {
		if strings.HasPrefix(key, prefix) {
			stale = append(stale, entry)
			delete(p.clients, key)
		}
	}
	p.mu.Unlock()
	for _, entry := range stale {
		entry.client.Close()
	}
}

// EvictIdle closes clients unused for longer than IdleTTL.
func (p *Provider) EvictIdle(now time.Time) int {
	if p.IdleTTL <= 0 {
		return 0
	}
	p.mu.Lock()
	var stale []*clientEntry
	for key, entry := range p.clients {
		if now.Sub(entry.lastUsed) > p.IdleTTL {
			stale = append(stale, entry)
			delete(p.clients, key)
		}
	}
	p.mu.Unlock()
	for _, entry := range stale {
		entry.client.Close()
	}
	return len(stale)
}

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

func (p *Provider) Close() {
	p.mu.Lock()
	entries := make([]*clientEntry, 0, len(p.clients))
	for key, entry := range p.clients {
		entries = append(entries, entry)
		delete(p.clients, key)
	}
	p.mu.Unlock()
	for _, entry := range entries {
		entry.client.Close()
	}
}

func sameServerConfig(snapshot, fresh *domain.MCPServer, oldAuth, newAuth string) bool {
	return oldAuth == newAuth &&
		snapshot.Transport == fresh.Transport &&
		snapshot.URL == fresh.URL &&
		snapshot.ToolsRevision == fresh.ToolsRevision
}

const maxResultChars = 64 << 10

const mcpToolCallTimeout = 20 * time.Second

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
	ctx, cancel := context.WithTimeout(ctx, mcpToolCallTimeout)
	defer cancel()
	return inner.InvokableRun(ctx, argumentsInJSON, opts...)
}
