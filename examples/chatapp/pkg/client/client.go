package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"nonoka-im/pkg/sdk"
)

// AuthClient handles authentication-related HTTP calls.
// NOTE: We need this because the SDK only provides auth through the WebSocket layer
// after Connect(). There's no standalone HTTP auth client exposed.
type AuthClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewAuthClient creates a new auth client.
func NewAuthClient(baseURL string) *AuthClient {
	return &AuthClient{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// RegisterRequest represents a registration request.
type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginRequest represents a login request.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	DeviceID string `json:"deviceId"`
}

// LoginResponse represents a login response.
type LoginResponse struct {
	Token  string `json:"token"`
	UserID int64  `json:"user_id"`
}

// Register creates a new user account.
func (c *AuthClient) Register(ctx context.Context, username, password string) error {
	body, _ := json.Marshal(RegisterRequest{Username: username, Password: password})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v1/auth/register", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("register request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("register failed with status: %d", resp.StatusCode)
	}
	return nil
}

// Login authenticates a user and returns a JWT token.
func (c *AuthClient) Login(ctx context.Context, username, password, deviceID string) (*LoginResponse, error) {
	body, _ := json.Marshal(LoginRequest{Username: username, Password: password, DeviceID: deviceID})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v1/auth/login", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("login failed with status: %d", resp.StatusCode)
	}

	var result LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode login response: %w", err)
	}
	return &result, nil
}

// ChatClient wraps the SDK client with higher-level chat operations.
type ChatClient struct {
	sdkClient  *sdk.Client
	authClient *AuthClient
	config     ChatConfig
}

// ChatConfig holds configuration for the chat client.
type ChatConfig struct {
	BaseURL        string
	GatewayURL     string
	DeviceID       string
	RequestTimeout time.Duration
}

// NewChatClient creates a new chat client.
// PROBLEM: The SDK's Options struct mixes Layer 1 (HTTP) and Layer 2 (WebSocket) concerns.
// It's unclear which fields are required for which layer.
func NewChatClient(cfg ChatConfig) *ChatClient {
	return &ChatClient{
		authClient: NewAuthClient(cfg.BaseURL),
		config:     cfg,
	}
}

// Connect authenticates and establishes a WebSocket connection.
func (c *ChatClient) Connect(ctx context.Context, token string) error {
	// PROBLEM: The SDK requires GatewayURL to be provided upfront, but the gateway URL
	// should ideally be resolved dynamically via the dispatch service. The SDK does have
	// auto-resolve logic in Connect(), but it requires the token to extract user_id.
	// This creates a chicken-and-egg problem if you want to resolve gateway before connecting.
	c.sdkClient = sdk.NewClient(sdk.Options{
		BaseURL:           c.config.BaseURL,
		GatewayURL:        c.config.GatewayURL,
		Token:             token,
		DeviceID:          c.config.DeviceID,
		RequestTimeout:    c.config.RequestTimeout,
		HeartbeatInterval: 30 * time.Second,
		AutoReconnect:     true,
		AutoAck:           true,
	})

	if err := c.sdkClient.Connect(ctx); err != nil {
		return fmt.Errorf("sdk connect failed: %w", err)
	}

	return nil
}

// Close closes all client resources.
func (c *ChatClient) Close() error {
	if c.sdkClient != nil {
		return c.sdkClient.Close()
	}
	return nil
}

// IsConnected returns true if the client is connected.
func (c *ChatClient) IsConnected() bool {
	if c.sdkClient == nil {
		return false
	}
	return c.sdkClient.IsConnected()
}

// UserID returns the authenticated user ID.
func (c *ChatClient) UserID() int64 {
	if c.sdkClient == nil {
		return 0
	}
	return c.sdkClient.UserID()
}

// SendText sends a text message to a topic.
func (c *ChatClient) SendText(ctx context.Context, topic string, text string) error {
	if c.sdkClient == nil {
		return fmt.Errorf("client not connected")
	}
	_, err := c.sdkClient.SendMessage(ctx, topic, 1, []byte(text)) // 1 = MSG_TYPE_TEXT
	return err
}

// OnMessage registers a message handler.
func (c *ChatClient) OnMessage(handler func(msg *sdk.Message)) {
	if c.sdkClient != nil {
		c.sdkClient.OnMessage(handler)
	}
}

// GetConversation returns a conversation for the given topic.
func (c *ChatClient) GetConversation(topic string) *sdk.Conversation {
	if c.sdkClient == nil {
		return nil
	}
	return c.sdkClient.Conversations.Get(topic)
}

// Auth returns the auth client for standalone HTTP auth operations.
func (c *ChatClient) Auth() *AuthClient {
	return c.authClient
}
