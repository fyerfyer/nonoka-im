package sdk

type RealtimeEventType string

const (
	RealtimeConnected       RealtimeEventType = "connected"
	RealtimeAuthed          RealtimeEventType = "authed"
	RealtimeDisconnected    RealtimeEventType = "disconnected"
	RealtimeReconnecting    RealtimeEventType = "reconnecting"
	RealtimeMessage         RealtimeEventType = "message"
	RealtimeSendReceipt     RealtimeEventType = "send_receipt"
	RealtimeDeliveryReceipt RealtimeEventType = "delivery_receipt"
	RealtimeReadReceipt     RealtimeEventType = "read_receipt"
	RealtimeRecall          RealtimeEventType = "recall"
	RealtimeAgentRunEvent   RealtimeEventType = "agent_run_event"
	RealtimeError           RealtimeEventType = "error"
)

type SendReceipt struct {
	ClientMessageID string
	MessageID       int64
	Topic           string
	TopicSeq        uint64
	Timestamp       int64
}

type DeliveryReceipt struct {
	Topic     string
	TopicSeq  uint64
	MessageID int64
}

type ReadReceipt struct {
	Topic    string
	UpToSeq  uint64
	ReaderID int64
}

type RecallNotice struct {
	Topic      string
	TopicSeq   uint64
	MessageID  int64
	SenderID   int64
	RecalledAt int64
}

// RealtimeEvent is the typed event emitted by the WebSocket client. Exactly
// one payload field is set.
type RealtimeEvent struct {
	Type            RealtimeEventType
	Message         *Message
	SendReceipt     *SendReceipt
	DeliveryReceipt *DeliveryReceipt
	ReadReceipt     *ReadReceipt
	Recall          *RecallNotice
	RunEvent        *RunEvent
	Err             error
}
