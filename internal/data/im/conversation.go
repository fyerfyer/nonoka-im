package imdata

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nonoka-im/internal/im/domain"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"gorm.io/gorm"
)

// ConversationRow is the per-user conversation summary stored in PostgreSQL.
func (ConversationRow) TableName() string { return "conversations" }

type ConversationRow struct {
	ID             int64  `gorm:"primaryKey;autoIncrement"`
	UserID         int64  `gorm:"index:idx_conversations_user_updated,priority:1;index:idx_conversations_user_topic,unique,priority:1;not null"`
	Topic          string `gorm:"index:idx_conversations_user_topic,unique,priority:2;size:64;not null"`
	Type           string `gorm:"size:16;not null"`
	PeerID         int64  `gorm:"not null"`
	PeerUsername   string `gorm:"size:50"`
	LastMsgPreview string `gorm:"size:255"`
	LastMsgAt      int64
	LastSeq        uint64 `gorm:"default:0"`
	LastReadSeq    uint64 `gorm:"default:0"`
	UnreadCount    int32  `gorm:"default:0"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ConversationRepo provides persistent conversation summary operations.
type ConversationRepo interface {
	UpsertConversation(ctx context.Context, userID int64, msg *domain.Message, topicSeq uint64) error
	ListConversations(ctx context.Context, userID int64, limit int32, offset int64) ([]*domain.Conversation, error)
	MarkRead(ctx context.Context, userID int64, topic string, upToSeq uint64) error
}

type conversationRepo struct {
	db  *gorm.DB
	log *log.Helper
}

func NewConversationRepo(db *gorm.DB, logger log.Logger) ConversationRepo {
	return &conversationRepo{db: db, log: log.NewHelper(logger)}
}

func (r *conversationRepo) UpsertConversation(ctx context.Context, userID int64, msg *domain.Message, topicSeq uint64) error {
	if msg == nil || msg.Topic == "" {
		return nil
	}
	topicType := domain.ParseTopicType(msg.Topic)
	if topicType != domain.TopicTypeP2P && topicType != domain.TopicTypeGroup {
		return nil
	}
	preview := previewForMessage(msg)
	now := time.Now()

	var peerID int64
	var peerUsername string
	if topicType == domain.TopicTypeP2P {
		uid1, uid2, err := domain.ExtractUserIDsFromP2PTopic(msg.Topic)
		if err != nil {
			return fmt.Errorf("extract p2p user ids: %w", err)
		}
		if userID == uid1 {
			peerID = uid2
		} else {
			peerID = uid1
		}
		var peer User
		if err := r.db.WithContext(ctx).First(&peer, peerID).Error; err == nil {
			peerUsername = peer.Username
		} else {
			r.log.Warnf("failed to load peer username: peer_id=%d err=%v", peerID, err)
		}
	}
	convType := topicTypeString(topicType)

	var rowsAffected int64
	if userID == msg.SenderID {
		res := r.db.WithContext(ctx).Model(&ConversationRow{}).
			Where("user_id = ? AND topic = ?", userID, msg.Topic).
			Updates(map[string]interface{}{
				"last_msg_preview": preview, "last_msg_at": msg.Timestamp, "last_seq": topicSeq,
				"last_read_seq": topicSeq, "unread_count": 0, "updated_at": now,
				"type": convType, "peer_id": peerID, "peer_username": peerUsername,
			})
		rowsAffected = res.RowsAffected
		if res.Error != nil {
			return fmt.Errorf("update sender conversation: %w", res.Error)
		}
	} else {
		res := r.db.WithContext(ctx).Model(&ConversationRow{}).
			Where("user_id = ? AND topic = ?", userID, msg.Topic).
			Updates(map[string]interface{}{
				"last_msg_preview": preview, "last_msg_at": msg.Timestamp, "last_seq": topicSeq,
				"unread_count": gorm.Expr("unread_count + 1"), "updated_at": now,
				"type": convType, "peer_id": peerID, "peer_username": peerUsername,
			})
		rowsAffected = res.RowsAffected
		if res.Error != nil {
			return fmt.Errorf("update recipient conversation: %w", res.Error)
		}
	}
	if rowsAffected == 0 {
		conv := &ConversationRow{
			UserID: userID, Topic: msg.Topic, Type: convType, PeerID: peerID, PeerUsername: peerUsername,
			LastMsgPreview: preview, LastMsgAt: msg.Timestamp, LastSeq: topicSeq, UpdatedAt: now,
		}
		if userID == msg.SenderID {
			conv.LastReadSeq = topicSeq
		} else {
			conv.UnreadCount = 1
		}
		if err := r.db.WithContext(ctx).Create(conv).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "duplicate key") {
				return nil
			}
			return fmt.Errorf("insert conversation: %w", err)
		}
	}
	return nil
}

func (r *conversationRepo) ListConversations(ctx context.Context, userID int64, limit int32, offset int64) ([]*domain.Conversation, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var rows []*ConversationRow
	result := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("updated_at DESC").Limit(int(limit)).Offset(int(offset)).Find(&rows)
	if result.Error != nil {
		return nil, fmt.Errorf("list conversations: %w", result.Error)
	}
	for _, row := range rows {
		if row.Type == "p2p" && row.PeerUsername == "" && row.PeerID > 0 {
			var peer User
			if err := r.db.WithContext(ctx).First(&peer, row.PeerID).Error; err == nil {
				row.PeerUsername = peer.Username
			}
		}
	}
	out := make([]*domain.Conversation, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomainConversation(row))
	}
	return out, nil
}

func (r *conversationRepo) MarkRead(ctx context.Context, userID int64, topic string, upToSeq uint64) error {
	update := map[string]interface{}{"unread_count": 0}
	if upToSeq > 0 {
		update["last_read_seq"] = upToSeq
	} else {
		update["last_read_seq"] = gorm.Expr("last_seq")
	}
	if err := r.db.WithContext(ctx).Model(&ConversationRow{}).Where("user_id = ? AND topic = ?", userID, topic).Updates(update).Error; err != nil {
		return fmt.Errorf("mark conversation read: %w", err)
	}
	return nil
}

func toDomainConversation(row *ConversationRow) *domain.Conversation {
	return &domain.Conversation{
		ID: row.ID, UserID: row.UserID, Topic: row.Topic, Type: row.Type, PeerID: row.PeerID,
		PeerUsername: row.PeerUsername, LastMsgPreview: row.LastMsgPreview, LastMsgAt: row.LastMsgAt,
		LastSeq: row.LastSeq, LastReadSeq: row.LastReadSeq, UnreadCount: row.UnreadCount,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func topicTypeString(t domain.TopicType) string {
	switch t {
	case domain.TopicTypeP2P:
		return "p2p"
	case domain.TopicTypeGroup:
		return "group"
	default:
		return "unknown"
	}
}

func truncatePreview(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func previewForMessage(msg *domain.Message) string {
	switch msg.Type {
	case domain.MessageTypeImage:
		return "[图片]"
	case domain.MessageTypeFile:
		return "[文件]"
	default:
		return truncatePreview(string(msg.Content), 120)
	}
}

var _ ConversationRepo = (*conversationRepo)(nil)
