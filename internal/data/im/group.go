package imdata

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nonoka-im/internal/im/domain"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

// GroupRow is the persistent representation of a group conversation.
func (GroupRow) TableName() string { return "groups" }

type GroupRow struct {
	GroupID   string `gorm:"primaryKey;size:26"`
	Name      string `gorm:"size:100;not null"`
	OwnerID   int64  `gorm:"not null"`
	Topic     string `gorm:"size:64;not null"`
	CreatedAt int64  `gorm:"not null"`
}

// GroupRepo provides persistent group operations.
type GroupRepo interface {
	CreateGroup(ctx context.Context, name string, ownerID int64, memberIDs []int64) (*domain.Group, error)
	DeleteGroup(ctx context.Context, groupID string) error
	ListMyGroups(ctx context.Context, userID int64) ([]*domain.Group, error)
	GetGroup(ctx context.Context, groupID string) (*domain.Group, error)
	AddMember(ctx context.Context, groupID string, userID int64) error
	RemoveMember(ctx context.Context, groupID string, userID int64) error
	ListMembers(ctx context.Context, groupID string) ([]domain.GroupMember, error)
	IsGroupMember(ctx context.Context, groupID string, userID int64) (bool, error)
}

type groupRepo struct {
	db  *gorm.DB
	log *log.Helper
}

func NewGroupRepo(db *gorm.DB, logger log.Logger) GroupRepo {
	return &groupRepo{db: db, log: log.NewHelper(logger)}
}

func (r *groupRepo) CreateGroup(ctx context.Context, name string, ownerID int64, memberIDs []int64) (*domain.Group, error) {
	if name = strings.TrimSpace(name); name == "" {
		return nil, errors.BadRequest("INVALID_NAME", "group name is required")
	}
	groupID := ulid.Make().String()
	row := &GroupRow{GroupID: groupID, Name: name, OwnerID: ownerID, Topic: "grp_" + groupID, CreatedAt: time.Now().Unix()}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(row).Error; err != nil {
			return fmt.Errorf("create group: %w", err)
		}
		members := []GroupMember{{GroupID: groupID, UserID: ownerID}}
		seen := map[int64]struct{}{ownerID: {}}
		for _, id := range memberIDs {
			if id == 0 {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			members = append(members, GroupMember{GroupID: groupID, UserID: id})
		}
		if err := tx.CreateInBatches(members, 100).Error; err != nil {
			return fmt.Errorf("create group members: %w", err)
		}
		return nil
	})
	if err != nil {
		r.log.Errorf("CreateGroup failed: %v", err)
		return nil, errors.InternalServer("DB_ERROR", "failed to create group")
	}
	return toDomainGroup(row), nil
}

// DeleteGroup removes a group and its members. It is used to compensate a
// failed authorization write during group creation; there is no soft delete.
func (r *groupRepo) DeleteGroup(ctx context.Context, groupID string) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ?", groupID).Delete(&GroupMember{}).Error; err != nil {
			return err
		}
		return tx.Where("group_id = ?", groupID).Delete(&GroupRow{}).Error
	})
	if err != nil {
		r.log.Errorf("DeleteGroup failed: %v", err)
		return errors.InternalServer("DB_ERROR", "failed to delete group")
	}
	return nil
}

func (r *groupRepo) ListMyGroups(ctx context.Context, userID int64) ([]*domain.Group, error) {
	var rows []*GroupRow
	result := r.db.WithContext(ctx).Model(&GroupRow{}).Select("groups.*").
		Joins("INNER JOIN group_members ON group_members.group_id = groups.group_id").
		Where("group_members.user_id = ?", userID).Order("groups.created_at DESC").Find(&rows)
	if result.Error != nil {
		r.log.Errorf("ListMyGroups failed: %v", result.Error)
		return nil, errors.InternalServer("DB_ERROR", "failed to list groups")
	}
	out := make([]*domain.Group, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomainGroup(row))
	}
	return out, nil
}

func (r *groupRepo) GetGroup(ctx context.Context, groupID string) (*domain.Group, error) {
	var row GroupRow
	result := r.db.WithContext(ctx).Where("group_id = ?", groupID).First(&row)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, errors.NotFound("GROUP_NOT_FOUND", "group not found")
		}
		r.log.Errorf("GetGroup failed: %v", result.Error)
		return nil, errors.InternalServer("DB_ERROR", "failed to get group")
	}
	return toDomainGroup(&row), nil
}

func (r *groupRepo) AddMember(ctx context.Context, groupID string, userID int64) error {
	member := &GroupMember{GroupID: groupID, UserID: userID}
	if err := r.db.WithContext(ctx).Create(member).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || containsDuplicateKey(err) {
			return nil
		}
		r.log.Errorf("AddMember failed: %v", err)
		return errors.InternalServer("DB_ERROR", "failed to add member")
	}
	return nil
}

func (r *groupRepo) RemoveMember(ctx context.Context, groupID string, userID int64) error {
	if err := r.db.WithContext(ctx).Where("group_id = ? AND user_id = ?", groupID, userID).Delete(&GroupMember{}).Error; err != nil {
		r.log.Errorf("RemoveMember failed: %v", err)
		return errors.InternalServer("DB_ERROR", "failed to remove member")
	}
	return nil
}

func (r *groupRepo) ListMembers(ctx context.Context, groupID string) ([]domain.GroupMember, error) {
	var rows []struct {
		UserID   int64
		Username string
	}
	result := r.db.WithContext(ctx).Model(&GroupMember{}).
		Select("group_members.user_id, users.username").
		Joins("INNER JOIN users ON users.id = group_members.user_id").
		Where("group_members.group_id = ?", groupID).Order("users.username ASC").Scan(&rows)
	if result.Error != nil {
		r.log.Errorf("ListMembers failed: %v", result.Error)
		return nil, errors.InternalServer("DB_ERROR", "failed to list members")
	}
	members := make([]domain.GroupMember, len(rows))
	for i, row := range rows {
		members[i] = domain.GroupMember{UserID: row.UserID, Username: row.Username}
	}
	return members, nil
}

func (r *groupRepo) IsGroupMember(ctx context.Context, groupID string, userID int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&GroupMember{}).Where("group_id = ? AND user_id = ?", groupID, userID).Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check group membership: %w", err)
	}
	return count > 0, nil
}

func toDomainGroup(row *GroupRow) *domain.Group {
	return &domain.Group{GroupID: row.GroupID, Name: row.Name, OwnerID: row.OwnerID, Topic: row.Topic, CreatedAt: row.CreatedAt}
}

func containsDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key") || strings.Contains(msg, "unique constraint")
}

var _ GroupRepo = (*groupRepo)(nil)
