// Package im adapts the IM application read surface to the messaging ports.
package im

import (
	"context"

	"nonoka-im/internal/im/domain"
	"nonoka-im/internal/messaging"
)

// Query is the narrow IM application read surface required by messaging.
type Query interface {
	RecentMessages(ctx context.Context, userID int64, topic string, limit int) ([]domain.InboxMessage, error)
	MessageForUser(ctx context.Context, userID int64, topic string, msgID int64) (domain.InboxMessage, error)
}

// Reader converts IM application rows into the shared messaging read model.
type Reader struct{ Query Query }

func NewReader(q Query) *Reader { return &Reader{Query: q} }

func (r *Reader) RecentMessages(ctx context.Context, userID int64, topic string, limit int) ([]messaging.Message, error) {
	rows, err := r.Query.RecentMessages(ctx, userID, topic, limit)
	if err != nil {
		return nil, err
	}
	out := make([]messaging.Message, 0, len(rows))
	for i := range rows {
		out = append(out, messaging.Message{
			ID: rows[i].ID, SenderID: rows[i].SenderID, Timestamp: rows[i].Timestamp,
			Type: int32(rows[i].Type), Content: rows[i].Content,
		})
	}
	return out, nil
}

func (r *Reader) MessageForUser(ctx context.Context, userID int64, topic string, msgID int64) (messaging.Message, error) {
	row, err := r.Query.MessageForUser(ctx, userID, topic, msgID)
	if err != nil {
		return messaging.Message{}, messaging.ErrNotFound
	}
	msg := messaging.Message{
		ID: row.ID, SenderID: row.SenderID, Timestamp: row.Timestamp,
		Type: int32(row.Type), Content: row.Content,
	}
	if row.Type == domain.MessageTypeImage || row.Type == domain.MessageTypeFile {
		if meta, ok := messaging.ParseMediaContent(row.Content); ok {
			msg.FileID, msg.FileName, msg.Mime, msg.Size = meta.FileID, meta.Name, meta.Mime, meta.Size
		}
	}
	return msg, nil
}
