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
	Type      int32
	Content   []byte
	FileID    string
	FileName  string
	Mime      string
	Size      int64
}

// SearchFilter is the structured filter for a topic-scoped tool search.
type SearchFilter struct {
	SenderID int64
	Since    int64
	Until    int64
	HasFile  bool
}

// SearchHit is one authorized message search result.
type SearchHit struct {
	MsgID     int64
	Topic     string
	TopicSeq  uint64
	SenderID  int64
	Timestamp int64
	MsgType   int32
	HasFile   bool
	Snippet   string
}

// CitationRecorder persists the message refs consulted by a run. Implementations
// must deduplicate by (run_id, topic, msg_id).
type CitationRecorder interface {
	RecordRunCitations(ctx context.Context, runID uint64, hits []SearchHit) error
}

// MessageReader is the authorized IM read surface used by the agent tools.
type MessageReader interface {
	RecentMessages(ctx context.Context, userID int64, topic string, limit int) ([]Message, error)
	MessageForUser(ctx context.Context, userID int64, topic string, msgID int64) (Message, error)
	SearchMessages(ctx context.Context, userID int64, topic, query string, filter SearchFilter, limit, offset int) ([]SearchHit, bool, error)
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
