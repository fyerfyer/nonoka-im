package app

import (
	"context"
	"errors"
	"time"

	"nonoka-im/internal/audit"
	"nonoka-im/internal/bot/domain"
)

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrForbidden    = errors.New("forbidden")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrRateLimited  = errors.New("rate limited")
	ErrStreamClosed = errors.New("stream closed")
)

// Store is the persistence surface for external bots.
type Store interface {
	CreateBot(ctx context.Context, ownerID int64, username, name, description string, events []string) (*domain.Bot, error)
	ListBots(ctx context.Context, ownerID int64, limit, offset int) ([]domain.Bot, int64, error)
	ListAllBots(ctx context.Context, limit, offset int) ([]domain.Bot, int64, error)
	GetBot(ctx context.Context, id int64) (*domain.Bot, error)
	GetBotByUserID(ctx context.Context, userID int64) (*domain.Bot, error)
	ListBotsByUserIDs(ctx context.Context, ids []int64) ([]domain.Bot, error)
	UpdateBot(ctx context.Context, id int64, name, description, status string, events []string) error
	DisableBot(ctx context.Context, id int64) error

	CreateToken(ctx context.Context, token domain.Token, hash []byte) error
	ListTokens(ctx context.Context, botID int64) ([]domain.Token, error)
	RevokeToken(ctx context.Context, botID int64, tokenID string) error
	FindTokenByPrefix(ctx context.Context, prefix string) (*domain.Token, *domain.Bot, []byte, error)
	TouchToken(ctx context.Context, id string) error

	CreateWebhook(ctx context.Context, webhook domain.Webhook, ciphertext []byte) error
	UpdateWebhook(ctx context.Context, botID int64, url string, events []string, enabled bool) error
	GetWebhook(ctx context.Context, botID int64) (*domain.Webhook, []byte, error)
	RotateWebhookSecret(ctx context.Context, botID int64, ciphertext []byte, prefix string) error
	DeleteWebhook(ctx context.Context, botID int64) error
	MarkWebhookSuccess(ctx context.Context, botID int64) error
	MarkWebhookFailure(ctx context.Context, botID int64, disable bool) error

	EnqueueDelivery(ctx context.Context, delivery *domain.Delivery) (bool, error)
	ClaimDelivery(ctx context.Context, lock time.Duration) (*domain.Delivery, error)
	MarkDeliverySucceeded(ctx context.Context, id int64, attempt, statusCode int, snippet string) error
	MarkDeliveryRetry(ctx context.Context, id int64, attempt int, next time.Time, statusCode int, errMsg, snippet string) error
	MarkDeliveryFailed(ctx context.Context, id int64, attempt, statusCode int, errMsg, snippet string) error
	ListDeliveries(ctx context.Context, botID int64, status string, limit, offset int) ([]domain.Delivery, int64, error)
	ReplayDelivery(ctx context.Context, botID, id int64) error
	ResetStaleDeliveries(ctx context.Context, older time.Duration) (int64, error)
	CleanupDeliveries(ctx context.Context, before time.Time, limit int) (int64, error)
	DeliveryStats(ctx context.Context, botID int64, since time.Time) (map[string]int64, error)
}

// Authorizer is the narrow OpenFGA surface used by bot usecases.
type Authorizer interface {
	CanAccessTopic(ctx context.Context, userID int64, topic string) error
	CanManagePlatform(ctx context.Context, userID int64) error
}

// Sealer encrypts webhook secrets at rest.
type Sealer interface {
	Seal(plain []byte) ([]byte, error)
	Open(ciphertext []byte) ([]byte, error)
}

// MessageSender is the IM write port used by bot replies.
type MessageSender interface {
	Send(ctx context.Context, in SendInput) (SendResult, error)
}

type SendInput struct {
	ActorID      int64
	Topic        string
	Content      []byte
	ClientMsgID  string
	MentionedIDs []int64
}

type SendResult struct {
	ClientMsgID string
	Timestamp   int64
}

// Message is the read-model of one persisted IM message.
type Message struct {
	ID        int64
	Topic     string
	TopicSeq  uint64
	SenderID  int64
	Type      int32
	Content   []byte
	Timestamp int64
	FileID    string
	FileName  string
	Mime      string
	Size      int64
}

// MessageReader is the authorized IM read surface for bot runtime APIs.
type MessageReader interface {
	MessageByClientID(ctx context.Context, userID int64, topic, clientMsgID string) (Message, error)
	RecentMessages(ctx context.Context, userID int64, topic string, limit int, beforeSeq uint64) ([]Message, error)
	MessageForUser(ctx context.Context, userID int64, topic string, msgID int64) (Message, error)
}

// MessageLoader loads a trigger message for webhook delivery. It is separate
// from MessageReader so the delivery worker only depends on what it needs.
type MessageLoader interface {
	LoadMessage(ctx context.Context, userID int64, topic string, msgID int64) (Message, error)
}

// UserDirectory resolves display data for event senders and mentions.
type UserDirectory interface {
	Username(ctx context.Context, userID int64) (string, error)
}

// AuditRecorder records sensitive bot actions. Implementations must redact
// tokens, secrets and message content.
type AuditRecorder interface {
	Record(ctx context.Context, event audit.Event) error
}

// RateLimiter is the shared fixed-window limiter for bot runtime requests.
type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}
