package sdk

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	v1 "nonoka-im/api/im/v1"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// Client is the single entry point of the SDK. Resources are grouped by
// domain; Realtime is optional and only needed for live events.
type Client struct {
	cfg  Config
	http *apiTransport
	kb   *khttp.Client
	auth *authRoundTripper

	Auth          *AuthResource
	Users         *UsersResource
	Conversations *ConversationsResource
	Messages      *MessagesResource
	Groups        *GroupsResource
	Files         *FilesResource
	Agents        *AgentsResource
	LLM           *LLMResource
	MCP           *MCPResource
	Skills        *SkillsResource
	Runs          *RunsResource
	Realtime      *RealtimeClient
}

type protoClients struct {
	auth         v1.AuthServiceHTTPClient
	user         v1.UserServiceHTTPClient
	conversation v1.ConversationServiceHTTPClient
	message      v1.MessageServiceHTTPClient
	group        v1.GroupServiceHTTPClient
	dispatch     v1.DispatchServiceHTTPClient
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
	authTransport := &authRoundTripper{base: http.DefaultTransport, tokens: cfg.TokenSource, userAgent: cfg.UserAgent}
	kb, err := newProtoHTTPClient(cfg.BaseURL, cfg.Timeout, authTransport)
	if err != nil {
		return nil, err
	}
	pc := protoClients{
		auth: v1.NewAuthServiceHTTPClient(kb), user: v1.NewUserServiceHTTPClient(kb),
		conversation: v1.NewConversationServiceHTTPClient(kb), message: v1.NewMessageServiceHTTPClient(kb),
		group: v1.NewGroupServiceHTTPClient(kb), dispatch: v1.NewDispatchServiceHTTPClient(kb),
	}
	client := &Client{
		cfg: cfg, kb: kb, auth: authTransport,
		http: newAPITransport(cfg.BaseURL, cfg.HTTPClient, cfg.TokenSource, cfg.UserAgent),
	}
	client.Messages = &MessagesResource{proto: pc.message, client: client}
	client.Auth = &AuthResource{proto: pc.auth}
	client.Users = &UsersResource{proto: pc.user, http: client.http}
	client.Conversations = &ConversationsResource{
		proto: pc.conversation, http: client.http, messages: client.Messages, clients: map[string]*ConversationClient{},
	}
	client.Groups = &GroupsResource{proto: pc.group, http: client.http}
	client.Files = &FilesResource{baseURL: strings.TrimRight(cfg.BaseURL, "/"), client: cfg.HTTPClient, tokens: cfg.TokenSource, userAgent: cfg.UserAgent}
	client.Agents = &AgentsResource{http: client.http}
	client.LLM = &LLMResource{http: client.http}
	client.MCP = &MCPResource{http: client.http}
	client.Skills = &SkillsResource{http: client.http}
	client.Runs = &RunsResource{http: client.http, client: client}
	client.Realtime = newRealtimeClient(cfg, pc.dispatch)
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
	if c.kb != nil {
		if err := c.kb.Close(); err != nil && firstErr == nil {
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

func (c *Client) refreshToken(ctx context.Context) (string, error) {
	if refreshable, ok := c.cfg.TokenSource.(RefreshableTokenSource); ok {
		return refreshable.Refresh(ctx)
	}
	return "", fmt.Errorf("token source is not refreshable")
}

// UpdateToken replaces the token source with a fixed token. It is intended for
// login flows that already hold an access token.
func (c *Client) UpdateToken(token string) {
	source := StaticToken(token)
	c.cfg.TokenSource = source
	c.http.tokens = source
	if c.auth != nil {
		c.auth.tokens = source
	}
	if c.Realtime != nil {
		c.Realtime.cfg.TokenSource = source
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
