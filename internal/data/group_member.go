package data

import (
	"context"
	"fmt"

	"github.com/go-kratos/kratos/v2/log"
	"gorm.io/gorm/clause"
)

// GroupMember stores the many-to-many relationship between groups and users.
// It is persisted in PostgreSQL and cached in Redis by the service layer.
type GroupMember struct {
	GroupID string `gorm:"index:idx_group_members_group_user,unique;size:64;not null"`
	UserID  int64  `gorm:"index:idx_group_members_group_user,unique;not null"`
}

// GroupMemberRepo provides persistent group membership queries.
type GroupMemberRepo interface {
	// GetMembers returns all member user IDs of a group.
	GetMembers(ctx context.Context, groupID string) ([]int64, error)
	// AddMember adds a user to a group.
	AddMember(ctx context.Context, groupID string, userID int64) error
	// RemoveMember removes a user from a group.
	RemoveMember(ctx context.Context, groupID string, userID int64) error
}

type groupMemberRepo struct {
	data *Data
	log  *log.Helper
}

// NewGroupMemberRepo creates a new GroupMemberRepo backed by PostgreSQL.
func NewGroupMemberRepo(data *Data, logger log.Logger) GroupMemberRepo {
	return &groupMemberRepo{
		data: data,
		log:  log.NewHelper(logger),
	}
}

func (r *groupMemberRepo) GetMembers(ctx context.Context, groupID string) ([]int64, error) {
	var rows []GroupMember
	result := r.data.db.WithContext(ctx).
		Model(&GroupMember{}).
		Where("group_id = ?", groupID).
		Order("user_id asc").
		Find(&rows)
	if result.Error != nil {
		return nil, fmt.Errorf("query group members: %w", result.Error)
	}

	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.UserID)
	}
	return ids, nil
}

func (r *groupMemberRepo) AddMember(ctx context.Context, groupID string, userID int64) error {
	member := &GroupMember{GroupID: groupID, UserID: userID}
	// Use ON CONFLICT DO NOTHING to make duplicate inserts idempotent without
	// triggering database error logs.
	result := r.data.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(member)
	if result.Error != nil {
		return fmt.Errorf("add group member: %w", result.Error)
	}
	return nil
}

func (r *groupMemberRepo) RemoveMember(ctx context.Context, groupID string, userID int64) error {
	result := r.data.db.WithContext(ctx).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Delete(&GroupMember{})
	if result.Error != nil {
		return fmt.Errorf("remove group member: %w", result.Error)
	}
	return nil
}


