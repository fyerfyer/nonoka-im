package app

import "nonoka-im/internal/messaging"

// DeliveryEvent is the webhook envelope body.
type DeliveryEvent struct {
	SpecVersion string       `json:"spec_version"`
	ID          string       `json:"id"`
	Type        string       `json:"type"`
	CreatedAt   string       `json:"created_at"`
	Bot         EventBot     `json:"bot"`
	Data        DeliveryData `json:"data"`
}

type EventBot struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Name   string `json:"name"`
}

type DeliveryData struct {
	Message      *EventMessage      `json:"message,omitempty"`
	Conversation *EventConversation `json:"conversation,omitempty"`
}

type EventConversation struct {
	Topic string `json:"topic"`
	Type  string `json:"type"`
}

type EventMessage struct {
	ID        string         `json:"id"`
	Topic     string         `json:"topic"`
	TopicSeq  string         `json:"topic_seq"`
	Type      string         `json:"type"`
	Content   string         `json:"content"`
	Sender    EventSender    `json:"sender"`
	Mentions  []EventMention `json:"mentions,omitempty"`
	CreatedAt string         `json:"created_at"`
}

type EventSender struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	IsBot    bool   `json:"is_bot"`
}

type EventMention struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

// MessageEvent is the subset of the Kafka message contract used by the bot
// dispatcher. Keeping a local shape avoids importing transport adapters.
type MessageEvent = messaging.MessageEvent
