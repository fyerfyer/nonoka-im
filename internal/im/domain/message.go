package domain

import "time"

// MessageType mirrors the wire MsgType enum without importing the generated
// protobuf package into the domain layer.
type MessageType int32

const (
	MessageTypeUnspecified MessageType = 0
	MessageTypeText        MessageType = 1
	MessageTypeImage       MessageType = 2
	MessageTypeFile        MessageType = 3
	MessageTypeVoice       MessageType = 4
	MessageTypeAgentCard   MessageType = 5
)

// Message is one persisted IM message. It is used for both group messages
// (read fan-out) and inbox entries; InboxMessage adds the per-user fields.
type Message struct {
	ID               int64
	Topic            string
	SenderID         int64
	Type             MessageType
	Content          []byte
	Timestamp        int64
	TopicSeq         uint64
	ClientMsgID      string
	MentionedUserIDs []int64
	Recalled         bool
	CreatedAt        time.Time
}

// Getter methods keep the persistence adapter readable while still exposing a
// plain domain struct to the application layer.
func (m *Message) GetTopic() string             { return m.Topic }
func (m *Message) GetSenderId() int64           { return m.SenderID }
func (m *Message) GetMsgType() int32            { return int32(m.Type) }
func (m *Message) GetContent() []byte           { return m.Content }
func (m *Message) GetTimestamp() int64          { return m.Timestamp }
func (m *Message) GetClientMsgId() string       { return m.ClientMsgID }
func (m *Message) GetMentionedUserIds() []int64 { return m.MentionedUserIDs }

// InboxMessage is a per-user copy of a message (write fan-out for P2P/system
// messages and @mention notifications in groups).
type InboxMessage struct {
	Message
	UserID      int64
	Read        bool
	DeliveredAt time.Time
}

// MentionMessage is an @mention inbox entry. It has the same shape as an inbox
// message but lives in a separate MongoDB collection.
type MentionMessage struct {
	Message
	UserID      int64
	Read        bool
	DeliveredAt time.Time
}

// DeliveryStatus records that a message reached a specific user.
type DeliveryStatus struct {
	UserID      int64
	MsgID       int64
	Topic       string
	TopicSeq    uint64
	DeliveredAt time.Time
}

// TopicSeqBackup is the durable fallback for the Redis topic sequence.
type TopicSeqBackup struct {
	Topic     string
	MaxSeq    uint64
	UpdatedAt time.Time
}

// PushMessage is the domain-level payload sent to realtime pushers. Transport
// adapters map it to the protobuf push packet.
type PushMessage struct {
	MsgID            int64
	Topic            string
	SenderID         int64
	MsgType          int32
	Content          []byte
	Timestamp        int64
	TopicSeq         uint64
	ClientMsgID      string
	MentionedUserIDs []int64
	Recalled         bool
}

// SendReceipt confirms to the sender that a message was persisted.
type SendReceipt struct {
	ClientMsgID string
	MsgID       int64
	Topic       string
	TopicSeq    uint64
	Timestamp   int64
}

// ReceiptBatchItem is one recipient receipt in a batched push.
type ReceiptBatchItem struct {
	UserID  int64
	Receipt SendReceipt
}

// ReceiptFailure reports one receipt that could not be delivered.
type ReceiptFailure struct {
	UserID  int64
	Receipt SendReceipt
}
