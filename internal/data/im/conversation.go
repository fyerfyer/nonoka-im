package imdata

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nonoka-im/internal/im/domain"

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
	UpsertConversations(ctx context.Context, userIDs []int64, msg *domain.Message, topicSeq uint64) error
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

// UpsertConversations updates every recipient's summary in one statement.
// It replaces the old SELECT + UPDATE + INSERT loop (which was O(recipients)
// round trips) with a single multi-row INSERT ... ON CONFLICT.
func (r *conversationRepo) UpsertConversations(ctx context.Context, userIDs []int64, msg *domain.Message, topicSeq uint64) error {
	if msg == nil || msg.Topic == "" || len(userIDs) == 0 {
		return nil
	}
	topicType := domain.ParseTopicType(msg.Topic)
	if topicType != domain.TopicTypeP2P && topicType != domain.TopicTypeGroup {
		return nil
	}

	preview := previewForMessage(msg)
	now := time.Now()
	convType := topicTypeString(topicType)

	unique := make([]int64, 0, len(userIDs))
	seen := make(map[int64]struct{}, len(userIDs))
	for _, userID := range userIDs {
		if userID <= 0 {
			continue
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		unique = append(unique, userID)
	}
	if len(unique) == 0 {
		return nil
	}

	peerIDs := make(map[int64]int64, len(unique))
	peerNames := make(map[int64]string, len(unique))
	if topicType == domain.TopicTypeP2P {
		uid1, uid2, err := domain.ExtractUserIDsFromP2PTopic(msg.Topic)
		if err != nil {
			return fmt.Errorf("extract p2p user ids: %w", err)
		}
		for _, userID := range unique {
			if userID == uid1 {
				peerIDs[userID] = uid2
			} else {
				peerIDs[userID] = uid1
			}
		}
		// Peer usernames are display-only. Loading them on every message adds
		// a PostgreSQL query to the hot path; ListConversations backfills them
		// lazily from one IN query when a row is actually rendered.
	}

	const chunkSize = 500
	for start := 0; start < len(unique); start += chunkSize {
		end := start + chunkSize
		if end > len(unique) {
			end = len(unique)
		}
		chunk := unique[start:end]
		values := make([]string, 0, len(chunk))
		args := make([]any, 0, len(chunk)*12+2)
		for _, userID := range chunk {
			lastReadSeq := uint64(0)
			unread := int32(1)
			if userID == msg.SenderID {
				lastReadSeq = topicSeq
				unread = 0
			}
			values = append(values, "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
			args = append(args,
				userID, msg.Topic, convType, peerIDs[userID], peerNames[userID],
				preview, msg.Timestamp, topicSeq, lastReadSeq, unread, now, now,
			)
		}
		args = append(args, msg.SenderID, msg.SenderID)
		query := fmt.Sprintf(`
			INSERT INTO conversations
				(user_id, topic, type, peer_id, peer_username, last_msg_preview, last_msg_at, last_seq, last_read_seq, unread_count, created_at, updated_at)
			VALUES %s
			ON CONFLICT (user_id, topic) DO UPDATE SET
				type = EXCLUDED.type,
				peer_id = EXCLUDED.peer_id,
				peer_username = CASE WHEN EXCLUDED.peer_username <> '' THEN EXCLUDED.peer_username ELSE conversations.peer_username END,
				last_msg_preview = EXCLUDED.last_msg_preview,
				last_msg_at = EXCLUDED.last_msg_at,
				last_seq = EXCLUDED.last_seq,
				last_read_seq = CASE WHEN EXCLUDED.user_id = ? THEN EXCLUDED.last_read_seq ELSE conversations.last_read_seq END,
				unread_count = CASE WHEN EXCLUDED.user_id = ? THEN 0 ELSE conversations.unread_count + 1 END,
				updated_at = EXCLUDED.updated_at
		`, strings.Join(values, ", "))
		if err := r.db.WithContext(ctx).Exec(query, args...).Error; err != nil {
			return fmt.Errorf("upsert conversations: %w", err)
		}
	}
	return nil
}

// usernames loads all requested usernames in one query.
func (r *conversationRepo) usernames(ctx context.Context, ids []int64) (map[int64]string, error) {
	out := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []User
	if err := r.db.WithContext(ctx).Select("id", "username").Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row.Username
	}
	return out, nil
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
	// Backfill peer usernames for rows written before the batch upsert stored
	// them, in one query instead of one query per row.
	missing := make([]int64, 0)
	seen := make(map[int64]struct{})
	for _, row := range rows {
		if row.Type == "p2p" && row.PeerUsername == "" && row.PeerID > 0 {
			if _, ok := seen[row.PeerID]; ok {
				continue
			}
			seen[row.PeerID] = struct{}{}
			missing = append(missing, row.PeerID)
		}
	}
	if len(missing) > 0 {
		if names, err := r.usernames(ctx, missing); err == nil {
			for _, row := range rows {
				if row.PeerUsername == "" {
					row.PeerUsername = names[row.PeerID]
				}
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
	update := map[string]interface{}{}
	if upToSeq > 0 {
		// Read receipts can arrive out of order after reconnects. Never move
		// the read watermark backwards.
		update["last_read_seq"] = gorm.Expr("GREATEST(last_read_seq, ?)", upToSeq)
		update["unread_count"] = gorm.Expr("GREATEST(0, last_seq - GREATEST(last_read_seq, ?))", upToSeq)
	} else {
		update["last_read_seq"] = gorm.Expr("last_seq")
		update["unread_count"] = 0
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
