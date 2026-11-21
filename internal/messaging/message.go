// Package messaging is the shared integration surface used by services that
// are not the IM worker itself. It contains only transport-neutral DTOs and
// ports; concrete MongoDB/HTTP adapters live in subpackages.
package messaging

import (
	"context"
	"errors"
)

// ErrNotFound marks a message that does not exist or is not visible to the
// requesting user.
var ErrNotFound = errors.New("message not found")

// Message is the transport-neutral read model of one IM message.
type Message struct {
	ID        int64
	SenderID  int64
	Timestamp int64
	Content   []byte
}

// Reader is the authorized IM read surface.
type Reader interface {
	RecentMessages(ctx context.Context, userID int64, topic string, limit int) ([]Message, error)
	MessageForUser(ctx context.Context, userID int64, topic string, msgID int64) (Message, error)
}

// File is the read model of one attachment.
type File struct {
	Name string
	Mime string
	Data []byte
}

// FileReader loads attachments after the caller has checked authorization.
type FileReader interface {
	ReadFile(ctx context.Context, fileID string) (File, error)
}

// FailedEvent is one terminal outbox row exposed by the admin API.
type FailedEvent struct {
	ID        string
	EventID   string
	Topic     string
	TopicSeq  uint64
	Attempts  int
	LastError string
	CreatedAt int64
}

// OutboxAdmin lists and replays terminal outbox rows.
type OutboxAdmin interface {
	ListFailed(ctx context.Context, limit int) ([]FailedEvent, error)
	Replay(ctx context.Context, id string) error
}
