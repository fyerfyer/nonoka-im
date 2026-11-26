package app

import (
	"context"
	"time"
)

// EventPublisher fans an event out to live IM clients.
type EventPublisher interface {
	PublishBotStream(ctx context.Context, event StreamEvent, recipients []int64) error
}

// RecipientResolver resolves the human recipients of a topic.
type RecipientResolver interface {
	Recipients(ctx context.Context, topic string, exclude int64) ([]int64, error)
}

// StreamStore keeps ephemeral bot reply streams.
type StreamStore interface {
	Create(ctx context.Context, state StreamState, ttl time.Duration) error
	Get(ctx context.Context, id string) (*StreamState, error)
	Append(ctx context.Context, id string, botID int64, seq uint64, delta string, maxBytes int, ttl time.Duration) (*StreamState, error)
	MarkFinal(ctx context.Context, id string, botID int64) (*StreamState, error)
	Delete(ctx context.Context, id string, botID int64) (*StreamState, error)
	AllowChunk(ctx context.Context, id string, limit int) (bool, error)
}

// StreamState is the Redis-backed state of one bot reply stream.
type StreamState struct {
	ID          string
	BotID       int64
	BotUserID   int64
	Topic       string
	ClientMsgID string
	Seq         uint64
	Bytes       int
	State       string
	Content     string
	CreatedAt   time.Time
}

// StreamEvent is the realtime event pushed to IM clients.
type StreamEvent struct {
	StreamID    string `json:"streamId"`
	Seq         uint64 `json:"seq"`
	Type        string `json:"type"`
	Topic       string `json:"topic"`
	BotID       int64  `json:"botId"`
	BotUserID   int64  `json:"botUserId"`
	Delta       string `json:"delta,omitempty"`
	ClientMsgID string `json:"clientMsgId,omitempty"`
	ReplyMsgID  int64  `json:"replyMsgId,omitempty"`
	TimestampMs int64  `json:"timestampMs"`
}

const (
	StreamStateOpen  = "open"
	StreamStateFinal = "final"
)
