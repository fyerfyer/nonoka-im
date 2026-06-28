package data

import (
  "context"
  "fmt"
  "time"

  pb "nonoka-im/api/im/v1"

  "github.com/go-kratos/kratos/v2/errors"
  "github.com/go-kratos/kratos/v2/log"
  "github.com/oklog/ulid/v2"
  "gorm.io/gorm"
)

// Group is the persistent representation of a group conversation.
type Group struct {
  GroupID string `gorm:"primaryKey;size:26"`
  Name    string `gorm:"size:100;not null"`
  OwnerID int64  `gorm:"not null"`
  Topic   string `gorm:"size:64;not null"`
  CreatedAt int64 `gorm:"not null"`
}

// GroupRepo provides persistent group operations.
type GroupRepo interface {
  CreateGroup(ctx context.Context, name string, ownerID int64, memberIDs []int64) (*Group, error)
  ListMyGroups(ctx context.Context, userID int64) ([]*Group, error)
  GetGroup(ctx context.Context, groupID string) (*Group, error)
  AddMember(ctx context.Context, groupID string, userID int64) error
  RemoveMember(ctx context.Context, groupID string, userID int64) error
  ListMembers(ctx context.Context, groupID string) ([]*pb.GroupMember, error)
}

type groupRepo struct {
  data *Data
  log  *log.Helper
}

// NewGroupRepo creates a new GroupRepo backed by PostgreSQL.
func NewGroupRepo(data *Data, logger log.Logger) GroupRepo {
  return &groupRepo{
    data: data,
    log:  log.NewHelper(logger),
  }
}

// CreateGroup creates a new group, adds the owner and optional members.
func (r *groupRepo) CreateGroup(ctx context.Context, name string, ownerID int64, memberIDs []int64) (*Group, error) {
  if name = trim(name); name == "" {
    return nil, errors.BadRequest("INVALID_NAME", "group name is required")
  }

  groupID := ulid.Make().String()
  group := &Group{
    GroupID:   groupID,
    Name:      name,
    OwnerID:   ownerID,
    Topic:     fmt.Sprintf("grp_%s", groupID),
    CreatedAt: time.Now().Unix(),
  }

  err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
    if err := tx.Create(group).Error; err != nil {
      return fmt.Errorf("create group: %w", err)
    }

    // Owner is always a member.
    members := []GroupMember{
      {GroupID: groupID, UserID: ownerID},
    }
    memberSet := map[int64]struct{}{ownerID: {}}
    for _, id := range memberIDs {
      if id == 0 {
        continue
      }
      if _, ok := memberSet[id]; ok {
        continue
      }
      memberSet[id] = struct{}{}
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

  return group, nil
}

// ListMyGroups returns all groups the user is a member of.
func (r *groupRepo) ListMyGroups(ctx context.Context, userID int64) ([]*Group, error) {
  var rows []*Group
  result := r.data.db.WithContext(ctx).
    Model(&Group{}).
    Select("groups.*").
    Joins("INNER JOIN group_members ON group_members.group_id = groups.group_id").
    Where("group_members.user_id = ?", userID).
    Order("groups.created_at DESC").
    Find(&rows)
  if result.Error != nil {
    r.log.Errorf("ListMyGroups failed: %v", result.Error)
    return nil, errors.InternalServer("DB_ERROR", "failed to list groups")
  }
  return rows, nil
}

// GetGroup returns a group by ID.
func (r *groupRepo) GetGroup(ctx context.Context, groupID string) (*Group, error) {
  var group Group
  result := r.data.db.WithContext(ctx).Where("group_id = ?", groupID).First(&group)
  if result.Error != nil {
    if result.Error == gorm.ErrRecordNotFound {
      return nil, errors.NotFound("GROUP_NOT_FOUND", "group not found")
    }
    r.log.Errorf("GetGroup failed: %v", result.Error)
    return nil, errors.InternalServer("DB_ERROR", "failed to get group")
  }
  return &group, nil
}

// AddMember adds a user to a group.
func (r *groupRepo) AddMember(ctx context.Context, groupID string, userID int64) error {
  member := &GroupMember{GroupID: groupID, UserID: userID}
  if err := r.data.db.WithContext(ctx).Create(member).Error; err != nil {
    if errors.Is(err, gorm.ErrDuplicatedKey) || containsDuplicateKey(err) {
      return nil
    }
    r.log.Errorf("AddMember failed: %v", err)
    return errors.InternalServer("DB_ERROR", "failed to add member")
  }
  return nil
}

// RemoveMember removes a user from a group.
func (r *groupRepo) RemoveMember(ctx context.Context, groupID string, userID int64) error {
  result := r.data.db.WithContext(ctx).
    Where("group_id = ? AND user_id = ?", groupID, userID).
    Delete(&GroupMember{})
  if result.Error != nil {
    r.log.Errorf("RemoveMember failed: %v", result.Error)
    return errors.InternalServer("DB_ERROR", "failed to remove member")
  }
  return nil
}

// ListMembers returns group members with usernames.
func (r *groupRepo) ListMembers(ctx context.Context, groupID string) ([]*pb.GroupMember, error) {
  var rows []struct {
    UserID   int64
    Username string
  }
  result := r.data.db.WithContext(ctx).
    Model(&GroupMember{}).
    Select("group_members.user_id, users.username").
    Joins("INNER JOIN users ON users.id = group_members.user_id").
    Where("group_members.group_id = ?", groupID).
    Order("users.username ASC").
    Scan(&rows)
  if result.Error != nil {
    r.log.Errorf("ListMembers failed: %v", result.Error)
    return nil, errors.InternalServer("DB_ERROR", "failed to list members")
  }

  members := make([]*pb.GroupMember, len(rows))
  for i, r := range rows {
    members[i] = &pb.GroupMember{
      UserId:   r.UserID,
      Username: r.Username,
    }
  }
  return members, nil
}

func trim(s string) string {
  for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n' || s[0] == '\r') {
    s = s[1:]
  }
  for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
    s = s[:len(s)-1]
  }
  return s
}

func containsDuplicateKey(err error) bool {
  if err == nil {
    return false
  }
  msg := err.Error()
  return contains(msg, "duplicate key") || contains(msg, "unique constraint")
}

func contains(s, substr string) bool {
  return len(s) >= len(substr) && (s == substr || len(substr) == 0 || containsInternal(s, substr))
}

func containsInternal(s, substr string) bool {
  for i := 0; i+len(substr) <= len(s); i++ {
    if s[i:i+len(substr)] == substr {
      return true
    }
  }
  return false
}

// ensure interface compliance
var _ GroupRepo = (*groupRepo)(nil)
