// Package messaging is the shared integration surface used by services that
// are not the IM worker itself. It contains only transport-neutral DTOs and
// ports; concrete MongoDB/HTTP adapters live in subpackages.
package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// ErrNotFound marks a message that does not exist or is not visible to the
// requesting user.
var ErrNotFound = errors.New("message not found")

// Message type values mirror the IM wire enum without importing the proto
// package into messaging/agent code.
const (
	MessageTypeText      int32 = 1
	MessageTypeImage     int32 = 2
	MessageTypeFile      int32 = 3
	MessageTypeVoice     int32 = 4
	MessageTypeAgentCard int32 = 5
)

// Message is the transport-neutral read model of one IM message.
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

// MediaMeta is the JSON metadata stored in IMAGE/FILE message content.
type MediaMeta struct {
	FileID string `json:"file_id,omitempty"`
	URL    string `json:"url,omitempty"`
	Name   string `json:"name,omitempty"`
	Mime   string `json:"mime,omitempty"`
	Size   int64  `json:"size,omitempty"`
}

// ParseMediaContent decodes attachment metadata and derives the file ID from
// either the explicit field or the legacy /v1/files/<id> URL.
func ParseMediaContent(content []byte) (MediaMeta, bool) {
	var meta MediaMeta
	if len(content) == 0 || json.Unmarshal(content, &meta) != nil {
		return MediaMeta{}, false
	}
	if meta.FileID == "" {
		if id := strings.TrimPrefix(meta.URL, "/v1/files/"); id != meta.URL && !strings.ContainsAny(id, "/?#") {
			meta.FileID = id
		}
	}
	return meta, meta.FileID != ""
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
