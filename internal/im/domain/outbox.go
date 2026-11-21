package domain

import (
	"errors"
	"time"
)

// Outbox row status values. Published rows are retained briefly for audit and
// duplicate detection; failed rows are parked for manual replay.
const (
	OutboxPending    = "pending"
	OutboxProcessing = "processing"
	OutboxPublished  = "published"
	OutboxFailed     = "failed"
)

var (
	ErrRecallNotFound = errors.New("message not found or already recalled")
	ErrRecallExpired  = errors.New("message recall window expired")
	ErrNotFound       = errors.New("not found")
)

// OutboxEvent is written in the same MongoDB transaction as the IM message.
// Payload is an opaque integration-event body (currently messaging.MessageEvent)
// so the IM bounded context does not import the agent package.
type OutboxEvent struct {
	ID            string
	EventID       string
	MsgID         int64
	Topic         string
	TopicSeq      uint64
	Payload       []byte
	Status        string
	Attempts      int
	NextAttemptAt time.Time
	LockedUntil   time.Time
	LastError     string
	CreatedAt     time.Time
	PublishedAt   time.Time
}
