package sdk

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// ConversationManager manages the lifecycle of all Conversation instances.
type ConversationManager struct {
	client *Client
	convs  map[string]*Conversation
	mu     sync.RWMutex
}

// newConversationManager creates a new ConversationManager bound to the given Client.
func newConversationManager(client *Client) *ConversationManager {
	return &ConversationManager{
		client: client,
		convs:  make(map[string]*Conversation),
	}
}

// Get returns the Conversation for the given topic, creating one if it doesn't exist.
func (cm *ConversationManager) Get(topic string) *Conversation {
	cm.mu.RLock()
	conv, ok := cm.convs[topic]
	cm.mu.RUnlock()
	if ok {
		return conv
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()
	conv, ok = cm.convs[topic]
	if ok {
		return conv
	}

	conv = newConversation(cm, topic, inferConversationType(topic))
	cm.convs[topic] = conv
	return conv
}

// All returns all managed conversations.
func (cm *ConversationManager) All() []*Conversation {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	result := make([]*Conversation, 0, len(cm.convs))
	for _, conv := range cm.convs {
		result = append(result, conv)
	}
	return result
}

// onMessage routes a pushed message to the corresponding Conversation.
func (cm *ConversationManager) onMessage(msg *Message) {
	conv := cm.Get(msg.Topic)
	conv.appendMessage(msg)
	if conv.OnMessage != nil {
		conv.OnMessage(msg)
	}
}

// pullOfflineForAll pulls offline messages for all conversations after reconnect.
// It returns a map of topic -> error for any failed pulls (#9).
func (cm *ConversationManager) pullOfflineForAll(ctx context.Context) map[string]error {
	cm.mu.RLock()
	convs := make([]*Conversation, 0, len(cm.convs))
	for _, conv := range cm.convs {
		convs = append(convs, conv)
	}
	cm.mu.RUnlock()

	errs := make(map[string]error)
	for _, conv := range convs {
		lastSeq := conv.LastSeq
		// Pull messages after lastSeq
		result, err := cm.client.pullMessagesInternal(ctx, conv.Topic, lastSeq, 50)
		if err != nil {
			errs[conv.Topic] = fmt.Errorf("pull offline for topic %s: %w", conv.Topic, err)
			continue
		}
		for _, msg := range result.Messages {
			conv.appendMessage(msg)
			if conv.OnMessage != nil {
				conv.OnMessage(msg)
			}
		}
	}
	return errs
}

// inferConversationType determines conversation type from topic string.
// p2p_* -> P2P, grp_* -> Group, others default to P2P.
func inferConversationType(topic string) ConversationType {
	if strings.HasPrefix(topic, "grp_") {
		return ConversationTypeGroup
	}
	return ConversationTypeP2P
}
