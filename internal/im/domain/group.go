package domain

import "time"

// Group is the persistent representation of a group conversation.
type Group struct {
	GroupID   string
	Name      string
	OwnerID   int64
	Topic     string
	CreatedAt int64
}

// GroupMember is one user in a group with display metadata.
type GroupMember struct {
	UserID   int64
	Username string
}

// GroupMemberRow is the storage row for the group_members join table.
type GroupMemberRow struct {
	GroupID string
	UserID  int64
}

// Ensure timestamps are available to adapters without importing time there.
var _ = time.Time{}
