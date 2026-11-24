package sdk

import (
	"context"
)

// ConversationClient is a lightweight, stateless convenience wrapper around a
// topic. It does not keep a message cache; callers own their store.
type ConversationClient struct {
	Topic         string
	OnMessage     func(*Message)
	OnReadReceipt func(upToSeq uint64)

	messages *MessagesResource
	api      *ConversationsResource
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

func (r *ConversationsResource) dispatch(event RealtimeEvent) {
	r.mu.Lock()
	clients := make([]*ConversationClient, 0, len(r.clients))
	for _, client := range r.clients {
		clients = append(clients, client)
	}
	r.mu.Unlock()
	for _, client := range clients {
		if event.Type == RealtimeMessage && event.Message != nil && event.Message.Topic == client.Topic && client.OnMessage != nil {
			client.OnMessage(event.Message)
		}
		if event.Type == RealtimeReadReceipt && event.ReadReceipt != nil && event.ReadReceipt.Topic == client.Topic && client.OnReadReceipt != nil {
			client.OnReadReceipt(event.ReadReceipt.UpToSeq)
		}
	}
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
