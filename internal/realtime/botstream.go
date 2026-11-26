package realtime

import (
	"context"

	pb "nonoka-im/api/im/v1"
)

// BotStreamEventType is the closed set of external-bot reply stream events.
type BotStreamEventType string

const (
	BotStreamStarted BotStreamEventType = "started"
	BotStreamDelta   BotStreamEventType = "delta"
	BotStreamFinal   BotStreamEventType = "final"
	BotStreamAborted BotStreamEventType = "aborted"
)

// BotStreamEvent is a transient bot reply update. It is never persisted; the
// final IM message remains the source of truth.
type BotStreamEvent struct {
	StreamID    string             `json:"streamId"`
	Seq         uint64             `json:"seq"`
	Type        BotStreamEventType `json:"type"`
	Topic       string             `json:"topic"`
	BotID       int64              `json:"botId"`
	BotUserID   int64              `json:"botUserId"`
	Delta       string             `json:"delta,omitempty"`
	ClientMsgID string             `json:"clientMsgId,omitempty"`
	ReplyMsgID  int64              `json:"replyMsgId,omitempty"`
	TimestampMs int64              `json:"timestampMs"`
}

var botStreamTypeToProto = map[BotStreamEventType]pb.BotStreamEventType{
	BotStreamStarted: pb.BotStreamEventType_BOT_STREAM_STARTED,
	BotStreamDelta:   pb.BotStreamEventType_BOT_STREAM_DELTA,
	BotStreamFinal:   pb.BotStreamEventType_BOT_STREAM_FINAL,
	BotStreamAborted: pb.BotStreamEventType_BOT_STREAM_ABORTED,
}

// ToProto converts the shared event contract to the WebSocket payload.
func (e BotStreamEvent) ToProto() *pb.BotStreamEvent {
	eventType, ok := botStreamTypeToProto[e.Type]
	if !ok {
		eventType = pb.BotStreamEventType_BOT_STREAM_EVENT_UNKNOWN
	}
	return &pb.BotStreamEvent{
		StreamId: e.StreamID, Seq: e.Seq, Type: eventType, Topic: e.Topic,
		BotId: e.BotID, BotUserId: e.BotUserID, Delta: e.Delta,
		ClientMsgId: e.ClientMsgID, ReplyMsgId: e.ReplyMsgID, TimestampMs: e.TimestampMs,
	}
}

// BotStreamTransport pushes a bot stream event to all live connections of one
// user.
type BotStreamTransport interface {
	PushBotStreamEventToUser(ctx context.Context, userID int64, event BotStreamEvent) (int32, error)
}
