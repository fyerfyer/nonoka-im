package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"nonoka-im/examples/chatapp/internal/config"
	"nonoka-im/examples/chatapp/pkg/client"
	"nonoka-im/pkg/sdk"
)

// ChatApp represents a simple chat application using the nonoka-im SDK.
type ChatApp struct {
	cfg    *config.Config
	client *client.ChatClient
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Message handlers
	onMessage       func(msg *sdk.Message)
	onReadReceipt   func(topic string, upToSeq uint64)
	onDeliveryReceipt func(topic string, topicSeq uint64)
}

// NewChatApp creates a new chat application.
func NewChatApp(cfg *config.Config) *ChatApp {
	ctx, cancel := context.WithCancel(context.Background())
	return &ChatApp{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
	}
}

// RegisterAndLogin creates an account and logs in.
// PROBLEM: The SDK does NOT expose HTTP auth methods (register/login) through its API.
// We have to use a custom HTTP client for auth, then pass the token to the SDK.
// This is a significant gap - the SDK should provide Layer 1 auth methods.
func (app *ChatApp) RegisterAndLogin(ctx context.Context) (string, int64, error) {
	authClient := client.NewAuthClient(app.cfg.BaseURL)

	// Try to register (ignore duplicate errors)
	_ = authClient.Register(ctx, app.cfg.Username, app.cfg.Password)

	// Login
	resp, err := authClient.Login(ctx, app.cfg.Username, app.cfg.Password, app.cfg.DeviceID)
	if err != nil {
		return "", 0, fmt.Errorf("login failed: %w", err)
	}
	if resp.Token == "" {
		return "", 0, fmt.Errorf("empty token received")
	}

	return resp.Token, resp.UserID, nil
}

// Connect establishes connection to the IM server.
func (app *ChatApp) Connect(token string) error {
	app.client = client.NewChatClient(client.ChatConfig{
		BaseURL:        app.cfg.BaseURL,
		GatewayURL:     app.cfg.GatewayWSURL,
		DeviceID:       app.cfg.DeviceID,
		RequestTimeout: 10 * time.Second,
	})

	// Set up message handlers before connecting
	app.client.OnMessage(func(msg *sdk.Message) {
		if app.onMessage != nil {
			app.onMessage(msg)
		}
	})

	ctx, cancel := context.WithTimeout(app.ctx, 15*time.Second)
	defer cancel()

	if err := app.client.Connect(ctx, token); err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}

	// Start background workers
	app.wg.Add(1)
	go app.heartbeatMonitor()

	return nil
}

// heartbeatMonitor monitors connection health.
func (app *ChatApp) heartbeatMonitor() {
	defer app.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-app.ctx.Done():
			return
		case <-ticker.C:
			if !app.client.IsConnected() {
				fmt.Println("[WARN] Connection lost, SDK should auto-reconnect...")
				// PROBLEM: The SDK has auto-reconnect, but there's no way to check
				// the reconnection state or force a reconnect from outside.
				// Also, there's no "reconnecting" state - only disconnected or connected.
			}
		}
	}
}

// SendMessage sends a message to a topic.
func (app *ChatApp) SendMessage(topic, text string) error {
	ctx, cancel := context.WithTimeout(app.ctx, 10*time.Second)
	defer cancel()

	// PROBLEM: The SendMessage API requires passing msgType as v1.MsgType,
	// but v1 is an internal protobuf package. Users shouldn't need to import
	// internal protobuf types just to send a text message.
	// The SDK should provide convenience methods like SendText(), SendImage().
	if err := app.client.SendText(ctx, topic, text); err != nil {
		return fmt.Errorf("send message failed: %w", err)
	}
	return nil
}

// GetConversation returns the conversation for a topic.
func (app *ChatApp) GetConversation(topic string) (*sdk.Conversation, error) {
	conv := app.client.GetConversation(topic)
	if conv == nil {
		return nil, fmt.Errorf("failed to get conversation for topic: %s", topic)
	}
	return conv, nil
}

// LoadHistory loads message history for a conversation.
// PROBLEM: The Conversation.LoadHistory() API resets local Messages and fills
// with fetched history. This is destructive - you lose any unsent/pending messages.
func (app *ChatApp) LoadHistory(topic string, limit int32) ([]*sdk.Message, error) {
	conv, err := app.GetConversation(topic)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(app.ctx, 10*time.Second)
	defer cancel()

	// PROBLEM: LoadHistory resets the conversation's Messages slice entirely.
	// If there are pending/unacked messages, they will be lost.
	msgs, err := conv.LoadHistory(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("load history failed: %w", err)
	}
	return msgs, nil
}

// MarkRead marks all messages in a conversation as read.
func (app *ChatApp) MarkRead(topic string) error {
	conv, err := app.GetConversation(topic)
	if err != nil {
		return err
	}
	return conv.MarkRead(app.ctx)
}

// SetOnMessage sets the message handler.
func (app *ChatApp) SetOnMessage(handler func(msg *sdk.Message)) {
	app.onMessage = handler
}

// IsConnected returns true if connected.
func (app *ChatApp) IsConnected() bool {
	return app.client != nil && app.client.IsConnected()
}

// UserID returns the current user ID.
func (app *ChatApp) UserID() int64 {
	if app.client == nil {
		return 0
	}
	return app.client.UserID()
}

// Close shuts down the application.
func (app *ChatApp) Close() {
	app.cancel()
	if app.client != nil {
		app.client.Close()
	}
	app.wg.Wait()
}
