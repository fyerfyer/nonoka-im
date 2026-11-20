// Package messaging is the shared IM read/write surface for services that are
// not the IM worker itself. Phase 0 provides adapters over the existing
// msgworker storage; a later phase can move the storage implementation here
// without changing the consumers.
package messaging

import (
	"context"
	"errors"
	"fmt"

	"nonoka-im/internal/msgworker"

	"go.mongodb.org/mongo-driver/v2/mongo"
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

// Reader wraps the authorized msgworker read methods.
type Reader struct {
	storage *msgworker.MessageStorage
}

func NewReader(storage *msgworker.MessageStorage) *Reader {
	return &Reader{storage: storage}
}

func (r *Reader) RecentMessages(ctx context.Context, userID int64, topic string, limit int) ([]Message, error) {
	if r == nil || r.storage == nil {
		return nil, fmt.Errorf("message reader unavailable")
	}
	rows, err := r.storage.GetRecentMessages(ctx, userID, topic, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Message, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		out = append(out, Message{ID: row.MsgID, SenderID: row.SenderID, Timestamp: row.Timestamp, Content: row.Content})
	}
	return out, nil
}

func (r *Reader) MessageForUser(ctx context.Context, userID int64, topic string, msgID int64) (Message, error) {
	if r == nil || r.storage == nil {
		return Message{}, fmt.Errorf("message reader unavailable")
	}
	row, err := r.storage.GetMessageForUser(ctx, userID, topic, msgID)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return Message{}, ErrNotFound
		}
		return Message{}, err
	}
	return Message{ID: row.MsgID, SenderID: row.SenderID, Timestamp: row.Timestamp, Content: row.Content}, nil
}
