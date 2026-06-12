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

// ChatApp represents a chat application using the refactored nonoka-im SDK.
type ChatApp struct {
	cfg    *config.Config
	client *client.ChatClient
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// OnSendReceipt is forwarded to the SDK so the UI can update a sent
	// message with its server-assigned msg_id and topic_seq.
	OnSendReceipt func(clientMsgID string, msgID int64, topic string, topicSeq uint64)

	// OnDeliveryReceipt is forwarded to the SDK so the UI can mark a message
	// as delivered when the recipient receives it.
	OnDeliveryReceipt func(topic string, topicSeq uint64, msgID int64)
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

// RegisterAndLogin creates an account (ignoring duplicate) and logs in.
// The refactored SDK now exposes Auth.Register/Auth.Login — no custom HTTP client needed.
func (app *ChatApp) RegisterAndLogin(ctx context.Context) (string, int64, error) {
	app.client = client.NewChatClient(client.ChatConfig{
		BaseURL:           app.cfg.BaseURL,
		GatewayURL:        app.cfg.GatewayWSURL,
		DeviceID:          app.cfg.DeviceID,
		RequestTimeout:    app.cfg.RequestTimeout,
		HeartbeatInterval: app.cfg.HeartbeatInterval,
	})
	app.client.OnSendReceipt = app.OnSendReceipt

	// Try register (ignore if already exists)
	_ = app.client.Register(ctx, app.cfg.Username, app.cfg.Password)

	// Login
	token, userID, err := app.client.Login(ctx, app.cfg.Username, app.cfg.Password)
	if err != nil {
		return "", 0, fmt.Errorf("login failed: %w", err)
	}
	if token == "" {
		return "", 0, fmt.Errorf("empty token received")
	}

	return token, userID, nil
}

// Connect establishes connection to the IM server.
func (app *ChatApp) Connect(token string) error {
	ctx, cancel := context.WithTimeout(app.ctx, app.cfg.ConnectTimeout)
	defer cancel()

	// Re-sync the send receipt callback in case it was set after the client
	// was created during RegisterAndLogin.
	if app.client != nil {
		app.client.OnSendReceipt = app.OnSendReceipt
		app.client.OnDeliveryReceipt = app.OnDeliveryReceipt
	}

	if err := app.client.Connect(ctx, token); err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}

	// Start background connection monitor
	app.wg.Add(1)
	go app.connectionMonitor()

	return nil
}

// connectionMonitor periodically prints connection state.
// The new SDK exposes State() including "reconnecting" state.
func (app *ChatApp) connectionMonitor() {
	defer app.wg.Done()
	ticker := time.NewTicker(app.cfg.ConnectionMonitorInterval)
	defer ticker.Stop()

	prevState := "uninitialized"
	// Print initial state immediately so the first state change is not missed.
	if app.client != nil {
		state := app.client.ConnectionState()
		if state != prevState {
			fmt.Printf("[CONN] State changed: %s -> %s\n", prevState, state)
			prevState = state
		}
	}
	for {
		select {
		case <-app.ctx.Done():
			return
		case <-ticker.C:
			state := app.client.ConnectionState()
			if state != prevState {
				fmt.Printf("[CONN] State changed: %s -> %s\n", prevState, state)
				prevState = state
			}
		}
	}
}

// SendMessage sends a text message to a topic via Conversation API.
func (app *ChatApp) SendMessage(topic, text string) (*sdk.SendResult, error) {
	ctx, cancel := context.WithTimeout(app.ctx, app.cfg.SendTimeout)
	defer cancel()

	result, err := app.client.SendText(ctx, topic, text)
	if err != nil {
		return nil, fmt.Errorf("send message failed: %w", err)
	}
	return result, nil
}

// GetConversation returns the conversation for a topic.
func (app *ChatApp) GetConversation(topic string) *sdk.Conversation {
	return app.client.GetConversation(topic)
}

// LoadHistory loads message history for a conversation.
// The refactored SDK merges fetched history with local pending messages.
func (app *ChatApp) LoadHistory(topic string, limit int32) ([]*sdk.Message, error) {
	conv := app.GetConversation(topic)
	if conv == nil {
		return nil, fmt.Errorf("no conversation for topic: %s", topic)
	}

	ctx, cancel := context.WithTimeout(app.ctx, app.cfg.LoadHistoryTimeout)
	defer cancel()

	msgs, err := conv.LoadHistory(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("load history failed: %w", err)
	}
	return msgs, nil
}

// MarkRead marks all messages in a conversation as read.
func (app *ChatApp) MarkRead(topic string) error {
	conv := app.GetConversation(topic)
	if conv == nil {
		return fmt.Errorf("no conversation for topic: %s", topic)
	}

	ctx, cancel := context.WithTimeout(app.ctx, app.cfg.MarkReadTimeout)
	defer cancel()

	return conv.MarkRead(ctx)
}

// IsConnected returns true if connected.
func (app *ChatApp) IsConnected() bool {
	return app.client != nil && app.client.IsConnected()
}

// IsAuthed returns true if authenticated.
func (app *ChatApp) IsAuthed() bool {
	return app.client != nil && app.client.IsAuthed()
}

// ConnectionState returns the current connection state.
func (app *ChatApp) ConnectionState() string {
	if app.client == nil {
		return "uninitialized"
	}
	return app.client.ConnectionState()
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
