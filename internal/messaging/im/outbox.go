package im

import (
	"context"

	"nonoka-im/internal/im/domain"
	"nonoka-im/internal/messaging"
)

// Outbox is the narrow IM application outbox surface used by the admin API.
type Outbox interface {
	ListFailed(ctx context.Context, limit int) ([]domain.OutboxEvent, error)
	Replay(ctx context.Context, id string) error
	ReplayMany(ctx context.Context, ids []string) (int, error)
}

type OutboxAdmin struct{ Service Outbox }

func NewOutboxAdmin(service Outbox) *OutboxAdmin { return &OutboxAdmin{Service: service} }

func (o *OutboxAdmin) ListFailed(ctx context.Context, limit int) ([]messaging.FailedEvent, error) {
	rows, err := o.Service.ListFailed(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]messaging.FailedEvent, 0, len(rows))
	for _, row := range rows {
		out = append(out, messaging.FailedEvent{
			ID: row.ID, EventID: row.EventID, Topic: row.Topic, TopicSeq: row.TopicSeq,
			Attempts: row.Attempts, LastError: row.LastError, CreatedAt: row.CreatedAt.Unix(),
		})
	}
	return out, nil
}

func (o *OutboxAdmin) Replay(ctx context.Context, id string) error {
	return o.Service.Replay(ctx, id)
}

func (o *OutboxAdmin) ReplayMany(ctx context.Context, ids []string) (int, error) {
	return o.Service.ReplayMany(ctx, ids)
}
