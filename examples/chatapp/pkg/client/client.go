package client

import (
	"context"
	"fmt"
	"time"

	"nonoka-im/pkg/sdk"
)

// ChatClient wraps the SDK client with higher-level chat operations.
// With the refactored SDK, we no longer need a custom AuthClient —
// sdk.Client.Auth provides Register/Login out of the box.
type ChatClient struct {
	sdkClient *sdk.Client
	config    ChatConfig
}

// ChatConfig holds configuration for the chat client.
type ChatConfig struct {
	BaseURL        string
	GatewayURL     string // optional: empty means auto-resolve via dispatch
	DeviceID       string
	RequestTimeout time.Duration
}

// NewChatClient creates a new chat client.
func NewChatClient(cfg ChatConfig) *ChatClient {
	return &ChatClient{
		config: cfg,
	}
}

// Register creates a new user account via SDK AuthService.
func (c *ChatClient) Register(ctx context.Context, username, password string) error {
	if c.sdkClient == nil {
		c.initSDK("")
	}
	_, err := c.sdkClient.Auth.Register(ctx, username, password)
	return err
}

// Login authenticates a user and returns token + userID.
func (c *ChatClient) Login(ctx context.Context, username, password string) (string, int64, error) {
	if c.sdkClient == nil {
		c.initSDK("")
	}
	result, err := c.sdkClient.Auth.Login(ctx, username, password)
	if err != nil {
		return "", 0, fmt.Errorf("login failed: %w", err)
	}
	return result.Token, result.UserID, nil
}

// Connect establishes a WebSocket connection.
// If GatewayURL is empty in config, the SDK auto-resolves it via DispatchService.
func (c *ChatClient) Connect(ctx context.Context, token string) error {
	c.initSDK(token)
	if err := c.sdkClient.Connect(ctx); err != nil {
		return fmt.Errorf("sdk connect failed: %w", err)
	}
	return nil
}

// initSDK lazily initializes the underlying SDK client.
// Uses UpdateToken to avoid recreating the client when token changes.
func (c *ChatClient) initSDK(token string) {
	// If client already exists, just update the token.
	if c.sdkClient != nil {
		c.sdkClient.UpdateToken(token)
		return
	}
	c.sdkClient = sdk.NewClient(sdk.Options{
		BaseURL:           c.config.BaseURL,
		GatewayURL:        c.config.GatewayURL,
		Token:             token,
		DeviceID:          c.config.DeviceID,
		RequestTimeout:    c.config.RequestTimeout,
		HeartbeatInterval: 30 * time.Second,
		AutoReconnect:     true,
		AutoAck:           true,
		OnConnect: func() {
			fmt.Println("[SDK] Connected and authenticated")
		},
		OnDisconnect: func(reason error) {
			fmt.Printf("[SDK] Disconnected: %v\n", reason)
		},
	})
}

// Close closes all client resources.
func (c *ChatClient) Close() error {
	if c.sdkClient != nil {
		return c.sdkClient.Close()
	}
	return nil
}

// SDK returns the underlying SDK client for advanced usage.
func (c *ChatClient) SDK() *sdk.Client {
	return c.sdkClient
}

// IsConnected returns true if the client is connected.
func (c *ChatClient) IsConnected() bool {
	if c.sdkClient == nil {
		return false
	}
	return c.sdkClient.IsConnected()
}

// IsAuthed returns true if the client is authenticated.
func (c *ChatClient) IsAuthed() bool {
	if c.sdkClient == nil || c.sdkClient.Realtime == nil {
		return false
	}
	return c.sdkClient.Realtime.IsAuthed()
}

// ConnectionState returns the current connection state string.
func (c *ChatClient) ConnectionState() string {
	if c.sdkClient == nil || c.sdkClient.Realtime == nil {
		return "uninitialized"
	}
	return string(c.sdkClient.Realtime.State())
}

// UserID returns the authenticated user ID.
func (c *ChatClient) UserID() int64 {
	if c.sdkClient == nil {
		return 0
	}
	return c.sdkClient.UserID()
}

// SendText sends a text message to a topic via Conversation.
func (c *ChatClient) SendText(ctx context.Context, topic string, text string) (*sdk.SendResult, error) {
	if c.sdkClient == nil {
		return nil, fmt.Errorf("client not connected")
	}
	conv := c.sdkClient.Conversations.Get(topic)
	return conv.SendText(ctx, text)
}

// GetConversation returns a conversation for the given topic.
func (c *ChatClient) GetConversation(topic string) *sdk.Conversation {
	if c.sdkClient == nil {
		return nil
	}
	return c.sdkClient.Conversations.Get(topic)
}
