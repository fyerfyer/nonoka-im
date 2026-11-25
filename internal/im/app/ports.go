package app

import (
	"context"
	"errors"
	"time"

	"nonoka-im/internal/im/domain"
	"nonoka-im/internal/messaging"
)

// Typed usecase errors. Transport adapters map these to protocol error codes
// instead of parsing strings.
var (
	ErrAuthRequired       = errors.New("authentication required")
	ErrInvalidTopic       = errors.New("invalid topic")
	ErrTopicDenied        = errors.New("topic access denied")
	ErrMessageTooLarge    = errors.New("message too large")
	ErrClientMsgIDReq     = errors.New("client_msg_id is required")
	ErrDeliveryFailed     = errors.New("message delivery failed")
	ErrInvalidCursor      = errors.New("invalid cursor")
	ErrGroupNotFound      = errors.New("group not found")
	ErrGroupForbidden     = errors.New("group access denied")
	ErrInvalidGroupName   = errors.New("group name is required")
	ErrSearchUnavailable  = errors.New("search is not configured")
	ErrInvalidSearchQuery = errors.New("search query or has_file filter is required")
)

// IDGenerator generates globally unique message IDs.
type IDGenerator interface {
	NextID() int64
}

// SeqGenerator allocates per-topic sequence numbers.
type SeqGenerator interface {
	NextSeq(ctx context.Context, topic string) (uint64, error)
	NextSeqBatch(ctx context.Context, topic string, count int) ([]uint64, error)
}

// MessageWriter persists messages and, when provided, the agent outbox row in
// the same transaction.
type MessageWriter interface {
	SaveP2P(ctx context.Context, msg *domain.Message, outbox *domain.OutboxEvent) (recipients []int64, duplicate bool, err error)
	SaveGroup(ctx context.Context, msg *domain.Message, outbox *domain.OutboxEvent) (duplicate bool, err error)
	SaveGroupBatch(ctx context.Context, msgs []*domain.Message) (inserted int, duplicate bool, err error)
	SaveSystem(ctx context.Context, msg *domain.Message) (recipients []int64, duplicate bool, err error)
	SaveMentionInbox(ctx context.Context, msg *domain.Message, mentionedUserIDs []int64) error
	BackupTopicSeq(ctx context.Context, topic string, seq uint64) error
}

// SearchIndex is the optional keyword-search backend. Implementations receive
// an already-authorized topic and must never widen the topic filter.
type SearchIndex interface {
	Search(ctx context.Context, topic, query string, filter domain.SearchFilter, limit, offset int) ([]domain.SearchHit, bool, error)
	Healthy(ctx context.Context) error
}

// SearchIndexRepository stores the durable index outbox and relay state.
type SearchIndexRepository interface {
	ClaimSearchIndexTask(ctx context.Context) (*domain.SearchIndexTask, error)
	CompleteSearchIndexTask(ctx context.Context, id string) error
	FailSearchIndexTask(ctx context.Context, id string, taskErr error, backoff time.Duration) error
	AbandonSearchIndexTask(ctx context.Context, id string) error
	SearchIndexStats(ctx context.Context) (pending, failed int64, oldestAge time.Duration, err error)
}

// MessageReader is the authorized read surface used by pull/recall/receipts.
type MessageReader interface {
	GetOfflineMessages(ctx context.Context, userID int64, topic string, lastSeq uint64, limit int) ([]domain.InboxMessage, error)
	GetGroupMessages(ctx context.Context, topic string, lastSeq uint64, limit int) ([]domain.Message, error)
	GetHistoryMessages(ctx context.Context, userID int64, topic string, endSeq uint64, limit int) ([]domain.InboxMessage, bool, error)
	GetRecentMessages(ctx context.Context, userID int64, topic string, limit int) ([]domain.InboxMessage, error)
	GetMessageForUser(ctx context.Context, userID int64, topic string, msgID int64) (domain.InboxMessage, error)
	GetMentionMessages(ctx context.Context, userID int64, topic string, lastSeq uint64, limit int) ([]domain.MentionMessage, error)
	GetTopicMaxSeq(ctx context.Context, topic string) (uint64, error)
	GetMessageSender(ctx context.Context, topic string, topicSeq uint64) (int64, error)
	UpdateDeliveryStatus(ctx context.Context, userID int64, topic string, topicSeq uint64) error
	UpdateReadStatus(ctx context.Context, userID int64, topic string, upToSeq uint64) error
	RecallMessage(ctx context.Context, topic string, topicSeq uint64, msgID int64, senderID int64, window time.Duration) error
}

// OutboxRepository contains the storage primitives used by OutboxService.
type OutboxRepository interface {
	ClaimOutbox(ctx context.Context) (*domain.OutboxEvent, error)
	MarkOutboxPublished(ctx context.Context, id string) error
	FailOutbox(ctx context.Context, id string, eventErr error, backoff time.Duration) error
	AbandonOutbox(ctx context.Context, id string) error
	PrunePublishedOutbox(ctx context.Context) error
	OutboxStats(ctx context.Context) (pending, failed int64, oldestAge time.Duration, err error)
	ListFailedOutbox(ctx context.Context, limit int) ([]domain.OutboxEvent, error)
	ReplayOutbox(ctx context.Context, id string) error
	ReplayOutboxMany(ctx context.Context, ids []string) (int, error)
}

// ConversationRepository persists per-user conversation summaries.
type ConversationRepository interface {
	UpsertConversation(ctx context.Context, userID int64, msg *domain.Message, topicSeq uint64) error
	ListConversations(ctx context.Context, userID int64, limit int32, offset int64) ([]*domain.Conversation, error)
	MarkRead(ctx context.Context, userID int64, topic string, upToSeq uint64) error
}

// GroupRepository persists groups and membership.
type GroupRepository interface {
	CreateGroup(ctx context.Context, name string, ownerID int64, memberIDs []int64) (*domain.Group, error)
	ListMyGroups(ctx context.Context, userID int64) ([]*domain.Group, error)
	GetGroup(ctx context.Context, groupID string) (*domain.Group, error)
	AddMember(ctx context.Context, groupID string, userID int64) error
	RemoveMember(ctx context.Context, groupID string, userID int64) error
	ListMembers(ctx context.Context, groupID string) ([]domain.GroupMember, error)
	IsGroupMember(ctx context.Context, groupID string, userID int64) (bool, error)
}

// GroupMemberReader resolves group members for delivery and conversation
// fan-out. Implementations may cache.
type GroupMemberReader interface {
	GetGroupMembers(ctx context.Context, groupID string) ([]int64, error)
}

// BotDirectory reports which users are bot accounts so push delivery and
// agent triggers do not treat bots as online humans.
type BotDirectory interface {
	Ready() bool
	IsBot(ctx context.Context, userID int64) (bool, error)
	AnyBot(ctx context.Context, userIDs []int64) (bool, error)
}

// MessageProducer forwards a new client message to Kafka.
type MessageProducer interface {
	Produce(ctx context.Context, msg *domain.Message) error
}

// AttachmentSharer grants topic-wide read access to attachments referenced by
// a message. Implementations are best-effort; failures must not block sending.
type AttachmentSharer interface {
	ShareToTopic(ctx context.Context, userID int64, msgType domain.MessageType, content []byte, topic string)
}

// Pusher delivers messages and receipts to realtime gateway sessions.
type Pusher interface {
	BatchPushToUsers(ctx context.Context, userIDs []int64, msg *domain.PushMessage) (delivered int32, failed []int64, err error)
	PushReceiptToUser(ctx context.Context, userID int64, receipt *domain.SendReceipt) (delivered int32, err error)
	BatchPushReceiptsToUsers(ctx context.Context, items []domain.ReceiptBatchItem) (delivered int32, failed []domain.ReceiptFailure, err error)
	Close() error
}

// RetryQueue schedules a failed push for later delivery.
type RetryQueue interface {
	ScheduleRetry(ctx context.Context, userID int64, msg *domain.PushMessage) error
}

// FileStore persists attachments.
type FileStore interface {
	Put(ctx context.Context, file FileObject) error
	Get(ctx context.Context, fileID string) (FileObject, error)
}

// FileObject is one attachment document.
type FileObject struct {
	ID        string
	Name      string
	Mime      string
	Size      int64
	OwnerID   int64
	Data      []byte
	CreatedAt time.Time
}

// EventPublisher is an alias of the shared messaging contract so im/app only
// depends on a DTO/port package.
type EventPublisher = messaging.EventPublisher
