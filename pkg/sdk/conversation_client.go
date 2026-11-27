package sdk

import (
	"context"
	"sync"
)

// ConversationClient is a lightweight, stateless convenience wrapper around a
// topic. It does not keep a message cache; callers own their store.
type ConversationClient struct {
	Topic string

	mu            sync.RWMutex
	onMessage     func(*Message)
	onReadReceipt func(upToSeq uint64)

	messages *MessagesResource
	api      *ConversationsResource
}

// SetOnMessage installs the message callback. It is safe to call while the
// realtime dispatcher is running.
func (c *ConversationClient) SetOnMessage(fn func(*Message)) {
	c.mu.Lock()
	c.onMessage = fn
	c.mu.Unlock()
}

// SetOnReadReceipt installs the read-receipt callback.
func (c *ConversationClient) SetOnReadReceipt(fn func(uint64)) {
	c.mu.Lock()
	c.onReadReceipt = fn
	c.mu.Unlock()
}

func (c *ConversationClient) messageHandler() func(*Message) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.onMessage
}

func (c *ConversationClient) readReceiptHandler() func(uint64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.onReadReceipt
}

func (c *ConversationClient) SendText(ctx context.Context, text string) (*SendResult, error) {
	return c.messages.SendText(ctx, c.Topic, text)
}

func (c *ConversationClient) Send(ctx context.Context, req SendRequest) (*SendResult, error) {
	req.Topic = c.Topic
	return c.messages.Send(ctx, req)
}

func (c *ConversationClient) LoadHistory(ctx context.Context, limit int32) ([]*Message, error) {
	page, err := c.messages.Pull(ctx, PullRequest{Topic: c.Topic, Limit: limit})
	if err != nil {
		return nil, err
	}
	return page.Messages, nil
}

func (c *ConversationClient) MarkRead(ctx context.Context) error {
	return c.api.MarkRead(ctx, c.Topic)
}

// Get returns the lightweight client for a topic.
func (r *ConversationsResource) Get(topic string) *ConversationClient {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.clients[topic]; ok {
		return existing
	}
	client := &ConversationClient{Topic: topic, messages: r.messages, api: r}
	r.clients[topic] = client
	return client
}

// dispatchLoop consumes the realtime event stream for conversation callbacks.
func (c *Client) dispatchLoop() {
	events := c.Realtime.Events()
	for event := range events {
		if c.Conversations != nil {
			c.Conversations.dispatch(event)
		}
	}
}

func (r *ConversationsResource) dispatch(event RealtimeEvent) {
	if event.Type != RealtimeMessage && event.Type != RealtimeReadReceipt {
		return
	}
	topic := ""
	if event.Type == RealtimeMessage && event.Message != nil {
		topic = event.Message.Topic
	}
	if event.Type == RealtimeReadReceipt && event.ReadReceipt != nil {
		topic = event.ReadReceipt.Topic
	}
	r.mu.Lock()
	client := r.clients[topic]
	r.mu.Unlock()
	if client == nil {
		return
	}
	if event.Type == RealtimeMessage {
		if fn := client.messageHandler(); fn != nil {
			fn(event.Message)
		}
		return
	}
	if fn := client.readReceiptHandler(); fn != nil {
		fn(event.ReadReceipt.UpToSeq)
	}
}
