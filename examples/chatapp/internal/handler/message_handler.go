package handler

import (
	"fmt"
	"sync"
	"time"

	"nonoka-im/pkg/sdk"
)

// MessageHandler handles incoming chat messages.
// PROBLEM: The SDK's Message type has Status field but it's not always reliable.
// When a message is pushed from server, Status is 0 (Sending) which is misleading.
type MessageHandler struct {
	mu       sync.RWMutex
	messages []*sdk.Message
	unread   map[string]int32 // topic -> unread count
}

// NewMessageHandler creates a new message handler.
func NewMessageHandler() *MessageHandler {
	return &MessageHandler{
		unread: make(map[string]int32),
	}
}

// HandleMessage processes an incoming message.
func (h *MessageHandler) HandleMessage(msg *sdk.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.messages = append(h.messages, msg)
	h.unread[msg.Topic]++

	// PROBLEM: Messages received via push have Status=MessageStatusSending (0)
	// because the SDK's toSDKMessage() doesn't set the status. This is confusing
	// because a received message should have at least "Delivered" status.
	statusStr := msg.Status.String()

	fmt.Printf("[📨] New message | Topic: %s | From: %d | Seq: %d | Status: %s | Content: %s\n",
		msg.Topic, msg.SenderID, msg.TopicSeq, statusStr, string(msg.Content))
}

// GetUnreadCount returns unread count for a topic.
func (h *MessageHandler) GetUnreadCount(topic string) int32 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.unread[topic]
}

// MarkRead marks a topic as read.
func (h *MessageHandler) MarkRead(topic string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.unread[topic] = 0
}

// GetMessages returns all received messages.
func (h *MessageHandler) GetMessages() []*sdk.Message {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make([]*sdk.Message, len(h.messages))
	copy(result, h.messages)
	return result
}

// PrintConversation prints messages for a topic.
func (h *MessageHandler) PrintConversation(topic string, currentUserID int64) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	fmt.Printf("\n=== Conversation: %s ===\n", topic)
	for _, msg := range h.messages {
		if msg.Topic != topic {
			continue
		}
		direction := "←"
		if msg.SenderID == currentUserID {
			direction = "→"
		}
		t := time.Unix(msg.Timestamp, 0).Format("15:04:05")
		fmt.Printf("%s [%s] %s %s\n", direction, t, msg.Status.String(), string(msg.Content))
	}
	fmt.Println("========================")
}
