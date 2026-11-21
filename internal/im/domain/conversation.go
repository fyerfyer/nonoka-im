package domain

import "time"

// Conversation is the per-user conversation summary stored in PostgreSQL.
type Conversation struct {
	ID             int64
	UserID         int64
	Topic          string
	Type           string
	PeerID         int64
	PeerUsername   string
	LastMsgPreview string
	LastMsgAt      int64
	LastSeq        uint64
	LastReadSeq    uint64
	UnreadCount    int32
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
