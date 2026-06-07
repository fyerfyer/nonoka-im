package sdk

import (
	"errors"
	"time"

	v1 "nonoka-im/api/im/v1"
)

// MessageHandler is called when a new message is pushed from the server.
type MessageHandler func(msg *Message)

// DisconnectHandler is called when the client disconnects (either actively or due to error).
type DisconnectHandler func(reason error)

// ConnectHandler is called when the client successfully connects and authenticates.
type ConnectHandler func()

// MessageStatus represents the delivery status of a message.
type MessageStatus int

const (
	MessageStatusSending MessageStatus = iota // 发送中（转圈）
	MessageStatusSent                         // 服务器已确认（收到 ACK）
	MessageStatusDelivered                    // 对方已收到（推送到达其设备）
	MessageStatusRead                         // 对方已读
	MessageStatusFailed                       // 发送失败
)

// String returns a human-readable status string.
func (s MessageStatus) String() string {
	switch s {
	case MessageStatusSending:
		return "sending"
	case MessageStatusSent:
		return "sent"
	case MessageStatusDelivered:
		return "delivered"
	case MessageStatusRead:
		return "read"
	case MessageStatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// ConversationType represents the type of a conversation.
type ConversationType int

const (
	ConversationTypeP2P ConversationType = iota
	ConversationTypeGroup
)

// Message represents an IM message.
type Message struct {
	MsgID       int64
	Topic       string
	SenderID    int64
	MsgType     v1.MsgType
	Content     []byte
	Timestamp   int64
	TopicSeq    uint64
	Status      MessageStatus
	ClientMsgID string
}

// SendResult is returned after a message is successfully sent.
type SendResult struct {
	ClientMsgID string
	MsgID       int64
	Timestamp   int64
	TopicSeq    uint64
}

// PullResult is returned after pulling offline messages.
type PullResult struct {
	Messages []*Message
	HasMore  bool
	NextSeq  uint64
}

// toSDKMessage converts a protobuf MessagePush to SDK Message.
func toSDKMessage(push *v1.MessagePush) *Message {
	if push == nil {
		return nil
	}
	return &Message{
		MsgID:     push.MsgId,
		Topic:     push.Topic,
		SenderID:  push.SenderId,
		MsgType:   v1.MsgType(push.MsgType),
		Content:   push.Content,
		Timestamp: push.Timestamp,
		TopicSeq:  push.TopicSeq,
	}
}

// toSDKMessages converts protobuf PullMessages to SDK Messages.
func toSDKMessages(msgs []*v1.PullMessage) []*Message {
	result := make([]*Message, len(msgs))
	for i, m := range msgs {
		result[i] = &Message{
			MsgID:     m.MsgId,
			Topic:     m.Topic,
			SenderID:  m.SenderId,
			MsgType:   v1.MsgType(m.MsgType),
			Content:   m.Content,
			Timestamp: m.Timestamp,
			TopicSeq:  m.TopicSeq,
		}
	}
	return result
}

// ServerError represents an error response from the server.
type ServerError struct {
	Code       int32
	Message    string
	Retryable  bool
	RetryAfter time.Duration
}

// Error implements the error interface.
func (e *ServerError) Error() string {
	return e.Message
}

// IsServerError checks if an error is a ServerError.
func IsServerError(err error) (*ServerError, bool) {
	var se *ServerError
	if errors.As(err, &se) {
		return se, true
	}
	return nil, false
}
