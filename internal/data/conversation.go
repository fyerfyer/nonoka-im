package data

import (
	"context"
	"fmt"
	"strings"
	"time"

	pb "nonoka-im/api/im/v1"
	"nonoka-im/internal/topic"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"gorm.io/gorm"
)

// Conversation is the per-user conversation summary stored in PostgreSQL.
type Conversation struct {
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
	UpsertConversation(ctx context.Context, userID int64, msg *pb.UpstreamMessage, topicSeq uint64) error
	ListConversations(ctx context.Context, userID int64, limit int32, offset int64) ([]*Conversation, error)
	MarkRead(ctx context.Context, userID int64, topic string, upToSeq uint64) error
}

type conversationRepo struct {
	data *Data
	log  *log.Helper
}

// NewConversationRepo creates a new ConversationRepo backed by PostgreSQL.
func NewConversationRepo(data *Data, logger log.Logger) ConversationRepo {
	return &conversationRepo{
		data: data,
		log:  log.NewHelper(logger),
	}
}

// UpsertConversation updates or creates a conversation summary for a user
// when a new message is persisted. The sender's copy is considered read,
// while the recipient's unread count is incremented.
func (r *conversationRepo) UpsertConversation(ctx context.Context, userID int64, msg *pb.UpstreamMessage, topicSeq uint64) error {
	if msg == nil || msg.Topic == "" {
		return nil
	}

	topicType := topic.ParseType(msg.Topic)
	if topicType != topic.TypeP2P && topicType != topic.TypeGroup {
		return nil
	}

	preview := truncatePreview(string(msg.Content), 120)
	now := time.Now()

	var peerID int64
	var peerUsername string
	if topicType == topic.TypeP2P {
		uid1, uid2, err := topic.ExtractUserIDsFromP2PTopic(msg.Topic)
		if err != nil {
			return fmt.Errorf("extract p2p user ids: %w", err)
		}
		if userID == uid1 {
			peerID = uid2
		} else {
			peerID = uid1
		}
		var peer User
		if err := r.data.db.WithContext(ctx).First(&peer, peerID).Error; err == nil {
			peerUsername = peer.Username
		} else {
			r.log.Warnw("failed to load peer username", "peer_id", peerID, "error", err)
		}
	}

	convType := topicTypeString(topicType)

	// Update existing row first to avoid double-incrementing unread_count on insert+update races.
	var rowsAffected int64
	if userID == msg.SenderId {
		res := r.data.db.WithContext(ctx).Model(&Conversation{}).
			Where("user_id = ? AND topic = ?", userID, msg.Topic).
			Updates(map[string]interface{}{
				"last_msg_preview": preview,
				"last_msg_at":      msg.Timestamp,
				"last_seq":         topicSeq,
				"last_read_seq":    topicSeq,
				"unread_count":     0,
				"updated_at":       now,
				"type":             convType,
				"peer_id":          peerID,
				"peer_username":    peerUsername,
			})
		rowsAffected = res.RowsAffected
		if res.Error != nil {
			return fmt.Errorf("update sender conversation: %w", res.Error)
		}
	} else {
		res := r.data.db.WithContext(ctx).Model(&Conversation{}).
			Where("user_id = ? AND topic = ?", userID, msg.Topic).
			Updates(map[string]interface{}{
				"last_msg_preview": preview,
				"last_msg_at":      msg.Timestamp,
				"last_seq":         topicSeq,
				"unread_count":     gorm.Expr("unread_count + 1"),
				"updated_at":       now,
				"type":             convType,
				"peer_id":          peerID,
				"peer_username":    peerUsername,
			})
		rowsAffected = res.RowsAffected
		if res.Error != nil {
			return fmt.Errorf("update recipient conversation: %w", res.Error)
		}
	}

	// No existing row: insert a new one.
	if rowsAffected == 0 {
		conv := &Conversation{
			UserID:         userID,
			Topic:          msg.Topic,
			Type:           convType,
			PeerID:         peerID,
			PeerUsername:   peerUsername,
			LastMsgPreview: preview,
			LastMsgAt:      msg.Timestamp,
			LastSeq:        topicSeq,
			UpdatedAt:      now,
		}
		if userID == msg.SenderId {
			conv.LastReadSeq = topicSeq
			conv.UnreadCount = 0
		} else {
			conv.UnreadCount = 1
		}
		if err := r.data.db.WithContext(ctx).Create(conv).Error; err != nil {
			// Ignore duplicate key races: another insert won.
			if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "duplicate key") {
				return nil
			}
			return fmt.Errorf("insert conversation: %w", err)
		}
	}

	return nil
}

// ListConversations returns conversation summaries for a user ordered by updated_at DESC.
func (r *conversationRepo) ListConversations(ctx context.Context, userID int64, limit int32, offset int64) ([]*Conversation, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var rows []*Conversation
	result := r.data.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("updated_at DESC").
		Limit(int(limit)).
		Offset(int(offset)).
		Find(&rows)
	if result.Error != nil {
		return nil, fmt.Errorf("list conversations: %w", result.Error)
	}

	// Backfill missing peer usernames for P2P conversations.
	for _, row := range rows {
		if row.Type == "p2p" && row.PeerUsername == "" && row.PeerID > 0 {
			var peer User
			if err := r.data.db.WithContext(ctx).First(&peer, row.PeerID).Error; err == nil {
				row.PeerUsername = peer.Username
			}
		}
	}
	return rows, nil
}

// MarkRead marks all messages up to the current last_seq as read for the user's conversation.
func (r *conversationRepo) MarkRead(ctx context.Context, userID int64, topic string, upToSeq uint64) error {
	update := map[string]interface{}{
		"unread_count": 0,
	}
	if upToSeq > 0 {
		update["last_read_seq"] = upToSeq
	} else {
		update["last_read_seq"] = gorm.Expr("last_seq")
	}
	result := r.data.db.WithContext(ctx).Model(&Conversation{}).
		Where("user_id = ? AND topic = ?", userID, topic).
		Updates(update)
	if result.Error != nil {
		return fmt.Errorf("mark conversation read: %w", result.Error)
	}
	return nil
}

func topicTypeString(t topic.Type) string {
	switch t {
	case topic.TypeP2P:
		return "p2p"
	case topic.TypeGroup:
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

// ensure interface compliance
var _ ConversationRepo = (*conversationRepo)(nil)
