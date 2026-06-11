package handler

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"nonoka-im/pkg/sdk"
)

// MessageHandler handles incoming chat messages and tracks state transitions.
type MessageHandler struct {
	mu       sync.RWMutex
	messages []*sdk.Message
	unread   map[string]int32 // topic -> unread count

	// Track messages we've sent (clientMsgID -> *sdk.Message)
	sentMessages map[string]*sdk.Message
}

// NewMessageHandler creates a new message handler.
func NewMessageHandler() *MessageHandler {
	return &MessageHandler{
		unread:       make(map[string]int32),
		sentMessages: make(map[string]*sdk.Message),
	}
}

// HandleMessage processes an incoming message.
// With the refactored SDK, push messages now have Status=Delivered (was Sending before).
func (h *MessageHandler) HandleMessage(msg *sdk.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.messages = append(h.messages, msg)
	h.unread[msg.Topic]++

	statusIcon := statusToIcon(msg.Status)
	direction := "←"
	if msg.SenderID == 0 { // unknown sender
		direction = "?"
	}

	fmt.Printf("[📨] %s New message | Topic: %s | From: %d | Seq: %d | Status: %s %s | Content: %s\n",
		direction, msg.Topic, msg.SenderID, msg.TopicSeq, statusIcon, msg.Status.String(), string(msg.Content))
}

// TrackSentMessage tracks a message we've sent for status monitoring.
func (h *MessageHandler) TrackSentMessage(msg *sdk.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if msg.ClientMsgID != "" {
		h.sentMessages[msg.ClientMsgID] = msg
	}
	h.messages = append(h.messages, msg)
}

// UpdateSentStatus updates the status of a sent message by clientMsgID.
func (h *MessageHandler) UpdateSentStatus(clientMsgID string, status sdk.MessageStatus) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if msg, ok := h.sentMessages[clientMsgID]; ok {
		oldStatus := msg.Status
		msg.Status = status
		fmt.Printf("[📤] Status update: %s -> %s (clientMsgID=%s)\n",
			oldStatus.String(), status.String(), clientMsgID)
	}
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

// GetMessages returns all messages sorted by topic seq.
func (h *MessageHandler) GetMessages() []*sdk.Message {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make([]*sdk.Message, len(h.messages))
	copy(result, h.messages)
	sort.Slice(result, func(i, j int) bool {
		return result[i].TopicSeq < result[j].TopicSeq
	})
	return result
}

// GetConversationMessages returns messages for a specific topic.
func (h *MessageHandler) GetConversationMessages(topic string) []*sdk.Message {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var result []*sdk.Message
	for _, msg := range h.messages {
		if msg.Topic == topic {
			result = append(result, msg)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].TopicSeq < result[j].TopicSeq
	})
	return result
}

// PrintConversation prints messages for a topic with status indicators.
func (h *MessageHandler) PrintConversation(topic string, currentUserID int64) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	fmt.Printf("\n╔════════════════════════════════════════════════════╗\n")
	fmt.Printf("║  Conversation: %-35s ║\n", topic)
	fmt.Printf("╠════════════════════════════════════════════════════╣\n")

	for _, msg := range h.messages {
		if msg.Topic != topic {
			continue
		}
		direction := "←"
		if msg.SenderID == currentUserID {
			direction = "→"
		}
		t := time.Unix(msg.Timestamp, 0).Format("15:04:05")
		statusIcon := statusToIcon(msg.Status)
		fmt.Printf("║ %s [%s] %s %s\n", direction, t, statusIcon, string(msg.Content))
	}
	fmt.Printf("╚════════════════════════════════════════════════════╝\n")
}

// PrintStatusSummary prints a summary of all message statuses.
func (h *MessageHandler) PrintStatusSummary() {
	h.mu.RLock()
	defer h.mu.RUnlock()

	counts := make(map[sdk.MessageStatus]int)
	for _, msg := range h.messages {
		counts[msg.Status]++
	}

	fmt.Println("\n=== Message Status Summary ===")
	for status := sdk.MessageStatusSending; status <= sdk.MessageStatusFailed; status++ {
		if count := counts[status]; count > 0 {
			fmt.Printf("  %s %s: %d\n", statusToIcon(status), status.String(), count)
		}
	}
	fmt.Println("==============================")
}

// statusToIcon returns an emoji icon for a message status.
func statusToIcon(status sdk.MessageStatus) string {
	switch status {
	case sdk.MessageStatusSending:
		return "⏳"
	case sdk.MessageStatusSent:
		return "✓"
	case sdk.MessageStatusDelivered:
		return "✓✓"
	case sdk.MessageStatusRead:
		return "✓✓✓"
	case sdk.MessageStatusFailed:
		return "✗"
	default:
		return "?"
	}
}
