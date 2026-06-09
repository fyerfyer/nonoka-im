package sdk

import (
	"context"
	"sort"
	"sync"

	v1 "nonoka-im/api/im/v1"
)

// Conversation represents a chat conversation (P2P or Group).
// It manages local message cache, unread count, and provides a high-level API for sending/receiving messages.
type Conversation struct {
	Topic       string
	Type        ConversationType
	Messages    []*Message
	LastSeq     uint64
	UnreadCount int32

	// OnMessage is called when a new message arrives for this conversation.
	OnMessage func(msg *Message)

	// OnReadReceipt is called when the other party reads messages up to a certain seq.
	OnReadReceipt func(upToSeq uint64)

	manager *ConversationManager
	mu      sync.RWMutex
}

// newConversation creates a new Conversation managed by the given ConversationManager.
func newConversation(manager *ConversationManager, topic string, convType ConversationType) *Conversation {
	return &Conversation{
		Topic:    topic,
		Type:     convType,
		Messages: make([]*Message, 0),
		manager:  manager,
	}
}

// SendText sends a text message in this conversation.
func (c *Conversation) SendText(ctx context.Context, text string) (*SendResult, error) {
	return c.manager.client.SendMessage(ctx, c.Topic, v1.MsgType_MSG_TYPE_TEXT, []byte(text))
}

// SendImage sends an image message in this conversation.
func (c *Conversation) SendImage(ctx context.Context, imageURL string) (*SendResult, error) {
	return c.manager.client.SendMessage(ctx, c.Topic, v1.MsgType_MSG_TYPE_IMAGE, []byte(imageURL))
}

// SendFile sends a file message in this conversation.
func (c *Conversation) SendFile(ctx context.Context, fileURL, name string) (*SendResult, error) {
	// For simplicity, file name is not encoded separately in this version.
	return c.manager.client.SendMessage(ctx, c.Topic, v1.MsgType_MSG_TYPE_FILE, []byte(fileURL))
}

// LoadHistory loads the most recent messages from the server.
// It merges fetched history with local pending/sending messages instead of replacing.
func (c *Conversation) LoadHistory(ctx context.Context, limit int32) ([]*Message, error) {
	if limit <= 0 {
		limit = 20
	}

	result, err := c.pullMessages(ctx, 0, limit)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	// Merge with local pending/sending messages to avoid losing in-flight messages
	existing := make(map[uint64]bool, len(c.Messages))
	for _, m := range c.Messages {
		if m.TopicSeq > 0 {
			existing[m.TopicSeq] = true
		}
	}

	for _, m := range result.Messages {
		if !existing[m.TopicSeq] {
			c.Messages = append(c.Messages, m)
			existing[m.TopicSeq] = true
		}
	}

	// Sort by topic seq
	sort.Slice(c.Messages, func(i, j int) bool {
		return c.Messages[i].TopicSeq < c.Messages[j].TopicSeq
	})

	if len(c.Messages) > 0 {
		c.LastSeq = c.Messages[len(c.Messages)-1].TopicSeq
	}
	c.mu.Unlock()

	return result.Messages, nil
}

// LoadMore loads older messages before the current oldest local message.
func (c *Conversation) LoadMore(ctx context.Context, limit int32) ([]*Message, error) {
	if limit <= 0 {
		limit = 20
	}

	c.mu.RLock()
	lastSeq := c.LastSeq
	if len(c.Messages) > 0 {
		// Find the oldest seq
		lastSeq = c.Messages[0].TopicSeq
		for _, m := range c.Messages {
			if m.TopicSeq < lastSeq {
				lastSeq = m.TopicSeq
			}
		}
		if lastSeq > 0 {
			lastSeq--
		}
	}
	c.mu.RUnlock()

	result, err := c.pullMessages(ctx, lastSeq, limit)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	// Merge and deduplicate
	existing := make(map[uint64]bool, len(c.Messages))
	for _, m := range c.Messages {
		existing[m.TopicSeq] = true
	}
	for _, m := range result.Messages {
		if !existing[m.TopicSeq] {
			c.Messages = append(c.Messages, m)
			existing[m.TopicSeq] = true
		}
	}
	// Sort by topic seq
	sort.Slice(c.Messages, func(i, j int) bool {
		return c.Messages[i].TopicSeq < c.Messages[j].TopicSeq
	})
	if len(c.Messages) > 0 {
		c.LastSeq = c.Messages[len(c.Messages)-1].TopicSeq
	}
	c.mu.Unlock()

	return result.Messages, nil
}

// MarkRead marks all messages in this conversation as read and sends a read receipt to the server.
func (c *Conversation) MarkRead(ctx context.Context) error {
	c.mu.Lock()
	c.UnreadCount = 0
	lastSeq := c.LastSeq
	c.mu.Unlock()

	// Send read receipt to server if connected
	if c.manager.client != nil && c.manager.client.Realtime != nil && c.manager.client.Realtime.IsAuthed() {
		if err := c.manager.client.Realtime.SendReadReceipt(c.Topic, lastSeq); err != nil {
			// Log but don't fail — local state is already updated
			_ = err
		}
	}

	return nil
}

// updateMessageStatus updates the status of messages up to the given seq.
func (c *Conversation) updateMessageStatus(upToSeq uint64, status MessageStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, msg := range c.Messages {
		if msg.TopicSeq > 0 && msg.TopicSeq <= upToSeq {
			// Only upgrade status (e.g., Sent -> Delivered -> Read)
			if status > msg.Status {
				msg.Status = status
			}
		}
	}
}

// updateSingleMessageStatus updates the status of a single message by topicSeq.
func (c *Conversation) updateSingleMessageStatus(topicSeq uint64, status MessageStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, msg := range c.Messages {
		if msg.TopicSeq == topicSeq {
			// Only upgrade status
			if status > msg.Status {
				msg.Status = status
			}
			break
		}
	}
}

// GetUnreadCount returns the current unread message count.
func (c *Conversation) GetUnreadCount() int32 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.UnreadCount
}

// LastMessage returns the most recent message in this conversation.
func (c *Conversation) LastMessage() *Message {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.Messages) == 0 {
		return nil
	}
	return c.Messages[len(c.Messages)-1]
}

// appendMessage appends a received message to the conversation.
func (c *Conversation) appendMessage(msg *Message) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Messages = append(c.Messages, msg)
	if msg.TopicSeq > c.LastSeq {
		c.LastSeq = msg.TopicSeq
	}
	if msg.SenderID != c.manager.client.UserID() {
		c.UnreadCount++
	}
}

// pullMessages wraps RealtimeClient.PullMessages with fallback handling.
func (c *Conversation) pullMessages(ctx context.Context, lastSeq uint64, limit int32) (*PullResult, error) {
	return c.manager.client.pullMessagesInternal(ctx, c.Topic, lastSeq, limit)
}
