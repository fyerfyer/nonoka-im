// Package messaging also exposes the outbox operations needed by the admin
// DLQ view. The relay itself stays in msgworker; this adapter is read/replay
// only and safe to host from the gateway process.
package messaging

import (
	"context"
	"time"

	"nonoka-im/internal/msgworker"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type FailedEvent struct {
	ID        string    `json:"id"`
	EventID   string    `json:"eventId"`
	Topic     string    `json:"topic"`
	TopicSeq  uint64    `json:"topicSeq"`
	Attempts  int       `json:"attempts"`
	LastError string    `json:"lastError,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type OutboxAdmin struct {
	storage *msgworker.MessageStorage
}

func NewOutboxAdmin(storage *msgworker.MessageStorage) *OutboxAdmin {
	return &OutboxAdmin{storage: storage}
}

func (o *OutboxAdmin) ListFailed(ctx context.Context, limit int) ([]FailedEvent, error) {
	rows, err := o.storage.ListFailedAgentEvents(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]FailedEvent, 0, len(rows))
	for i := range rows {
		out = append(out, FailedEvent{
			ID: rows[i].ID.Hex(), EventID: rows[i].EventID, Topic: rows[i].Topic,
			TopicSeq: rows[i].TopicSeq, Attempts: rows[i].Attempts, LastError: rows[i].LastError,
			CreatedAt: rows[i].CreatedAt,
		})
	}
	return out, nil
}

// Replay moves one failed row back to pending. The agent consumer deduplicates
// by event_id, so a replay cannot create a second run.
func (o *OutboxAdmin) Replay(ctx context.Context, id string) error {
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	return o.storage.ReplayAgentEvent(ctx, objectID)
}
