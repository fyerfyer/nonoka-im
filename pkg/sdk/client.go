package sdk

import (
	"context"
	"fmt"
	"strings"
)

// Client is the single entry point of the SDK. Resources are grouped by
// domain; Realtime is optional and only needed for live events.
type Client struct {
	cfg  Config
	http *apiTransport

	Auth          *AuthResource
	Users         *UsersResource
	Conversations *ConversationsResource
	Messages      *MessagesResource
	Groups        *GroupsResource
	Files         *FilesResource
	Agents        *AgentsResource
	Models        *ModelsResource
	Quotas        *QuotasResource
	Audit         *AuditResource
	MCP           *MCPResource
	Skills        *SkillsResource
	Runs          *RunsResource
	Realtime      *RealtimeClient
}

// New creates an SDK client. A BaseURL and a TokenSource (or WithToken) are
// required for authenticated APIs.
func New(cfg Config, opts ...Option) (*Client, error) {
	for _, opt := range opts {
		opt(&cfg)
	}
	cfg = cfg.withDefaults()
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("%w: base URL is required", ErrInvalidConfiguration)
	}
	client := &Client{
		cfg:  cfg,
		http: newAPITransport(cfg.BaseURL, cfg.HTTPClient, cfg.TokenSource, cfg.UserAgent),
	}
	client.Messages = &MessagesResource{http: client.http, client: client}
	client.Auth = &AuthResource{http: client.http}
	client.Users = &UsersResource{http: client.http}
	client.Conversations = &ConversationsResource{
		http: client.http, messages: client.Messages, clients: map[string]*ConversationClient{},
	}
	client.Groups = &GroupsResource{http: client.http}
	client.Files = &FilesResource{http: client.http}
	client.Agents = &AgentsResource{http: client.http}
	client.Models = &ModelsResource{http: client.http}
	client.Quotas = &QuotasResource{http: client.http}
	client.Audit = &AuditResource{http: client.http}
	client.MCP = &MCPResource{http: client.http}
	client.Skills = &SkillsResource{http: client.http}
	client.Runs = &RunsResource{http: client.http, client: client}
	client.Realtime = newRealtimeClient(cfg, client.http)
	go client.dispatchLoop()
	return client, nil
}

// Connect opens the realtime WebSocket connection. It is optional for
// server-side agents that only use HTTP APIs.
func (c *Client) Connect(ctx context.Context) error {
	if c.Realtime == nil {
		return fmt.Errorf("%w: realtime is not configured", ErrInvalidConfiguration)
	}
	return c.Realtime.Connect(ctx)
}

// Close closes the realtime connection and HTTP transports.
func (c *Client) Close() error {
	var firstErr error
	if c.Realtime != nil {
		if err := c.Realtime.Close(); err != nil {
			firstErr = err
		}
	}
	return firstErr
}

// Config returns the effective client configuration (excluding secrets).
func (c *Client) Config() Config { return c.cfg }

// UserID returns the user encoded in the current access token, or zero.
func (c *Client) UserID() int64 {
	if c == nil || c.cfg.TokenSource == nil {
		return 0
	}
	token, err := c.cfg.TokenSource.Token(context.Background())
	if err != nil {
		return 0
	}
	return userIDFromToken(token)
}

// UpdateToken replaces the token source with a fixed token. It is intended for
// login flows that already hold an access token.
func (c *Client) UpdateToken(token string) {
	source := StaticToken(token)
	c.cfg.TokenSource = source
	c.http.Tokens = source
	if c.Realtime != nil {
		c.Realtime.setTokenSource(source)
	}
}

// IsConnected reports whether the realtime WebSocket is open and authed.
func (c *Client) IsConnected() bool {
	return c != nil && c.Realtime != nil && c.Realtime.IsConnected()
}

// IsAuthed is an alias for IsConnected; both require a completed auth handshake.
func (c *Client) IsAuthed() bool { return c.IsConnected() }

// ConnectionState returns the last realtime state.
func (c *Client) ConnectionState() string {
	if c == nil || c.Realtime == nil {
		return "uninitialized"
	}
	return c.Realtime.State()
}

// httpClient exposes the authenticated JSON transport to subresources.
func (c *Client) httpClient() *apiTransport { return c.http }
