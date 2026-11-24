package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"nonoka-im/pkg/sdk"
)

// ChatClient wraps the SDK with the small set of high-level operations the demo
// needs. Authentication and resource modules come from one SDK client.
type ChatClient struct {
	mu        sync.RWMutex
	sdkClient *sdk.Client
	config    ChatConfig
	token     string
	userID    int64

	// OnSendReceipt and OnDeliveryReceipt bridge realtime receipts into the
	// demo's local message store.
	OnSendReceipt     func(clientMsgID string, msgID int64, topic string, topicSeq uint64)
	OnDeliveryReceipt func(topic string, topicSeq uint64, msgID int64)
}

type ChatConfig struct {
	BaseURL           string
	GatewayURL        string
	DeviceID          string
	RequestTimeout    time.Duration
	HeartbeatInterval time.Duration
}

func NewChatClient(cfg ChatConfig) *ChatClient {
	return &ChatClient{config: cfg}
}

func (c *ChatClient) Register(ctx context.Context, username, password string) error {
	client, err := sdk.New(sdk.Config{BaseURL: c.config.BaseURL, Timeout: c.config.RequestTimeout})
	if err != nil {
		return err
	}
	defer client.Close()
	_, err = client.Auth.Register(ctx, username, password)
	return err
}

func (c *ChatClient) Login(ctx context.Context, username, password string) (string, int64, error) {
	client, err := sdk.New(sdk.Config{BaseURL: c.config.BaseURL, Timeout: c.config.RequestTimeout})
	if err != nil {
		return "", 0, err
	}
	defer client.Close()
	pair, err := client.Auth.Login(ctx, username, password, c.config.DeviceID)
	if err != nil {
		return "", 0, fmt.Errorf("login failed: %w", err)
	}
	c.mu.Lock()
	c.token, c.userID = pair.Token, pair.UserID
	c.mu.Unlock()
	return pair.Token, pair.UserID, nil
}

func (c *ChatClient) Connect(ctx context.Context, token string) error {
	c.mu.Lock()
	c.token = token
	c.mu.Unlock()
	if err := c.initSDKLocked(); err != nil {
		return err
	}
	c.mu.RLock()
	client := c.sdkClient
	c.mu.RUnlock()
	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("sdk connect failed: %w", err)
	}
	go c.consumeEvents(client)
	return nil
}

func (c *ChatClient) initSDKLocked() error {
	if c.sdkClient != nil {
		return nil
	}
	realtime := sdk.DefaultRealtimeConfig()
	realtime.DeviceID = c.config.DeviceID
	realtime.HeartbeatInterval = c.config.HeartbeatInterval
	if c.config.GatewayURL != "" {
		realtime.GatewayURLs = []string{c.config.GatewayURL}
	}
	client, err := sdk.New(
		sdk.Config{
			BaseURL: c.config.BaseURL, TokenSource: sdk.StaticToken(c.token),
			Timeout: c.config.RequestTimeout, Realtime: realtime,
		},
	)
	if err != nil {
		return err
	}
	c.sdkClient = client
	return nil
}

func (c *ChatClient) consumeEvents(client *sdk.Client) {
	for event := range client.Realtime.Events() {
		switch event.Type {
		case sdk.RealtimeSendReceipt:
			if event.SendReceipt != nil && c.OnSendReceipt != nil {
				c.OnSendReceipt(event.SendReceipt.ClientMessageID, event.SendReceipt.MessageID, event.SendReceipt.Topic, event.SendReceipt.TopicSeq)
			}
		case sdk.RealtimeDeliveryReceipt:
			if event.DeliveryReceipt != nil && c.OnDeliveryReceipt != nil {
				c.OnDeliveryReceipt(event.DeliveryReceipt.Topic, event.DeliveryReceipt.TopicSeq, event.DeliveryReceipt.MessageID)
			}
		case sdk.RealtimeConnected:
			fmt.Println("[SDK] Connected")
		case sdk.RealtimeDisconnected:
			fmt.Printf("[SDK] Disconnected: %v\n", event.Err)
		case sdk.RealtimeError:
			if event.Err != nil {
				fmt.Printf("[SDK] Realtime error: %v\n", event.Err)
			}
		}
	}
}

func (c *ChatClient) Close() error {
	c.mu.Lock()
	client := c.sdkClient
	c.sdkClient = nil
	c.mu.Unlock()
	if client != nil {
		return client.Close()
	}
	return nil
}

func (c *ChatClient) SDK() *sdk.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sdkClient
}

func (c *ChatClient) IsConnected() bool {
	client := c.SDK()
	return client != nil && client.IsConnected()
}

func (c *ChatClient) IsAuthed() bool {
	client := c.SDK()
	return client != nil && client.IsAuthed()
}

func (c *ChatClient) ConnectionState() string {
	client := c.SDK()
	if client == nil {
		return "uninitialized"
	}
	return client.ConnectionState()
}

func (c *ChatClient) UserID() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.userID
}

func (c *ChatClient) SendText(ctx context.Context, topic, text string) (*sdk.SendResult, error) {
	client := c.SDK()
	if client == nil {
		return nil, fmt.Errorf("client not connected")
	}
	return client.Messages.SendText(ctx, topic, text)
}

func (c *ChatClient) GetConversation(topic string) *sdk.ConversationClient {
	client := c.SDK()
	if client == nil {
		return nil
	}
	return client.Conversations.Get(topic)
}
