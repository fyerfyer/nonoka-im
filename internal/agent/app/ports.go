package app

import (
	"context"
	"errors"
)

// ErrMessageNotFound marks a trigger message that no longer exists or is not
// visible to the actor. The executor treats it as a permanent configuration
// error instead of a transient storage outage.
var ErrMessageNotFound = errors.New("message not found")

// Message is the read-model view of one persisted IM message. The agent layer
// never sees MongoDB documents or the msgworker storage type.
type Message struct {
	ID        int64
	SenderID  int64
	Timestamp int64
	Content   []byte
}

// MessageReader is the authorized IM read surface used by the agent tools.
type MessageReader interface {
	RecentMessages(ctx context.Context, userID int64, topic string, limit int) ([]Message, error)
	MessageForUser(ctx context.Context, userID int64, topic string, msgID int64) (Message, error)
}

// File is the read-model view of one attachment.
type File struct {
	Name string
	Mime string
	Data []byte
}

// FileReader loads attachments after the caller has checked authorization.
type FileReader interface {
	ReadFile(ctx context.Context, fileID string) (File, error)
}

// ReplySender posts agent output and approval cards into an IM topic. It is
// implemented over the internal HTTP API so the harness has no transport
// dependency.
type ReplySender interface {
	Send(ctx context.Context, topic, content, clientID string) error
	SendWithReceipt(ctx context.Context, topic, content, clientID string) (int64, error)
	SendCard(ctx context.Context, topic, content, clientID string) (int64, error)
}
