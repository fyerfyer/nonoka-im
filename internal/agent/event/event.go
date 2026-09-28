package event

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

const SchemaVersion = 1

// MessageEvent is emitted after an IM message has been durably stored.
// Content is intentionally omitted; the worker re-reads it through authorized APIs.
type MessageEvent struct {
	SchemaVersion     int     `json:"schema_version"`
	EventID           string  `json:"event_id"`
	MsgID             int64   `json:"msg_id"`
	Topic             string  `json:"topic"`
	SenderID          int64   `json:"sender_id"`
	MsgType           int32   `json:"msg_type"`
	Content           string  `json:"-"` // populated by the worker after authorized storage lookup
	Timestamp         int64   `json:"timestamp"`
	MentionedAgentIDs []int64 `json:"mentioned_agent_ids,omitempty"`
	TraceParent       string  `json:"traceparent,omitempty"`
}

func NewMessageEvent(msgID int64, topic string, senderID int64, msgType int32, timestamp int64, mentions []int64) MessageEvent {
	base := fmt.Sprintf("%d:%s:%d", msgID, topic, senderID)
	h := sha256.Sum256([]byte(base))
	return MessageEvent{SchemaVersion: SchemaVersion, EventID: hex.EncodeToString(h[:]), MsgID: msgID, Topic: topic, SenderID: senderID, MsgType: msgType, Timestamp: timestamp, MentionedAgentIDs: mentions}
}

func (e MessageEvent) Marshal() ([]byte, error) {
	if e.SchemaVersion == 0 {
		e.SchemaVersion = SchemaVersion
	}
	return json.Marshal(e)
}
func (e MessageEvent) Time() time.Time { return time.Unix(e.Timestamp, 0) }

type Publisher interface {
	Publish(context.Context, MessageEvent) error
}
