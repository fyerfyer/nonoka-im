package app

import (
	"context"
	"strings"

	"nonoka-im/internal/im/domain"
)

// GroupService provides group conversation usecases.
type GroupService struct {
	Repo       GroupRepository
	Authorizer GroupAuthorizer
}

func (s *GroupService) Create(ctx context.Context, userID int64, name string, memberIDs []int64) (*domain.Group, error) {
	if userID <= 0 {
		return nil, ErrAuthRequired
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrInvalidGroupName
	}
	group, err := s.Repo.CreateGroup(ctx, name, userID, memberIDs)
	if err != nil {
		return nil, err
	}
	if s.Authorizer != nil {
		if err := s.Authorizer.SetGroupOwner(ctx, group.GroupID, userID); err != nil {
			return nil, err
		}
		for _, memberID := range memberIDs {
			if err := s.Authorizer.SetGroupMember(ctx, group.GroupID, memberID, true); err != nil {
				return nil, err
			}
		}
		if err := s.Authorizer.SetTopicGroup(ctx, group.Topic, group.GroupID); err != nil {
			return nil, err
		}
	}
	return group, nil
}

func (s *GroupService) ListMine(ctx context.Context, userID int64) ([]*domain.Group, error) {
	if userID <= 0 {
		return nil, ErrAuthRequired
	}
	groups, err := s.Repo.ListMyGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	if s.Authorizer == nil {
		return groups, nil
	}
	visible := make([]*domain.Group, 0, len(groups))
	for _, group := range groups {
		if s.Authorizer.CanAccessTopic(ctx, userID, group.Topic) == nil {
			visible = append(visible, group)
		}
	}
	return visible, nil
}

func (s *GroupService) Get(ctx context.Context, userID int64, groupID string) (*domain.Group, error) {
	if userID <= 0 {
		return nil, ErrAuthRequired
	}
	group, err := s.Repo.GetGroup(ctx, groupID)
	if err != nil {
		return nil, ErrGroupNotFound
	}
	if err := s.requireMember(ctx, userID, group); err != nil {
		return nil, err
	}
	return group, nil
}

func (s *GroupService) AddMember(ctx context.Context, userID int64, groupID string, memberID int64) error {
	if userID <= 0 {
		return ErrAuthRequired
	}
	group, err := s.Repo.GetGroup(ctx, groupID)
	if err != nil {
		return ErrGroupNotFound
	}
	if err := s.requireMember(ctx, userID, group); err != nil {
		return err
	}
	if err := s.Repo.AddMember(ctx, groupID, memberID); err != nil {
		return err
	}
	if s.Authorizer != nil {
		return s.Authorizer.SetGroupMember(ctx, groupID, memberID, true)
	}
	return nil
}

func (s *GroupService) RemoveMember(ctx context.Context, userID int64, groupID string, memberID int64) error {
	if userID <= 0 {
		return ErrAuthRequired
	}
	group, err := s.Repo.GetGroup(ctx, groupID)
	if err != nil {
		return ErrGroupNotFound
	}
	if s.Authorizer != nil {
		if memberID != userID && s.Authorizer.CanManageGroup(ctx, userID, groupID) != nil {
			return ErrGroupForbidden
		}
	} else if group.OwnerID != userID && memberID != userID {
		return ErrGroupForbidden
	}
	if memberID == group.OwnerID && userID != group.OwnerID {
		return ErrGroupForbidden
	}
	if s.Authorizer != nil {
		if err := s.Authorizer.SetGroupMember(ctx, groupID, memberID, false); err != nil {
			return err
		}
	}
	return s.Repo.RemoveMember(ctx, groupID, memberID)
}

func (s *GroupService) ListMembers(ctx context.Context, userID int64, groupID string) ([]domain.GroupMember, error) {
	if userID <= 0 {
		return nil, ErrAuthRequired
	}
	group, err := s.Repo.GetGroup(ctx, groupID)
	if err != nil {
		return nil, ErrGroupNotFound
	}
	if err := s.requireMember(ctx, userID, group); err != nil {
		return nil, err
	}
	return s.Repo.ListMembers(ctx, groupID)
}

func (s *GroupService) requireMember(ctx context.Context, userID int64, group *domain.Group) error {
	if s.Authorizer != nil {
		if s.Authorizer.CanAccessTopic(ctx, userID, group.Topic) != nil {
			return ErrGroupForbidden
		}
		return nil
	}
	member, err := s.Repo.IsGroupMember(ctx, group.GroupID, userID)
	if err != nil || !member {
		return ErrGroupForbidden
	}
	return nil
}
