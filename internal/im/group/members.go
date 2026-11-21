// Package group provides group membership caching and bot-user indexing for
// the IM delivery pipeline.
package group

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
)

// MemberRepository is the persistent membership source.
type MemberRepository interface {
	GetMembers(ctx context.Context, groupID string) ([]int64, error)
	AddMember(ctx context.Context, groupID string, userID int64) error
	RemoveMember(ctx context.Context, groupID string, userID int64) error
}

// Service resolves group membership, optionally through a Redis read cache.
type Service struct {
	repo  MemberRepository
	redis redis.UniversalClient
	log   *log.Helper
}

func NewService(repo MemberRepository, redis redis.UniversalClient, logger log.Logger) *Service {
	return &Service{repo: repo, redis: redis, log: log.NewHelper(logger)}
}

func (s *Service) key(groupID string) string { return "im:group:members:" + groupID }

func (s *Service) GetGroupMembers(ctx context.Context, groupID string) ([]int64, error) {
	if s.redis != nil {
		members, err := s.redis.SMembers(ctx, s.key(groupID)).Result()
		if err == nil && len(members) > 0 {
			return parseMemberIDs(members, groupID, s.log), nil
		}
	}
	ids, err := s.repo.GetMembers(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("get group members: %w", err)
	}
	if s.redis != nil && len(ids) > 0 {
		values := make([]interface{}, len(ids))
		for i, id := range ids {
			values[i] = id
		}
		if err := s.redis.SAdd(ctx, s.key(groupID), values...).Err(); err != nil {
			s.log.Warnf("warm group member cache failed: group=%s err=%v", groupID, err)
		} else {
			_ = s.redis.Expire(ctx, s.key(groupID), 24*time.Hour)
		}
	}
	return ids, nil
}

func (s *Service) AddGroupMember(ctx context.Context, groupID string, userID int64) error {
	if err := s.repo.AddMember(ctx, groupID, userID); err != nil {
		return fmt.Errorf("add group member: %w", err)
	}
	if s.redis != nil {
		if err := s.redis.SAdd(ctx, s.key(groupID), userID).Err(); err != nil {
			s.log.Warnf("add group member to cache failed: group=%s user=%d err=%v", groupID, userID, err)
		}
	}
	return nil
}

func (s *Service) RemoveGroupMember(ctx context.Context, groupID string, userID int64) error {
	if s.redis != nil {
		if err := s.redis.SRem(ctx, s.key(groupID), userID).Err(); err != nil {
			s.log.Warnf("remove group member from cache failed: group=%s user=%d err=%v", groupID, userID, err)
		}
	}
	if err := s.repo.RemoveMember(ctx, groupID, userID); err != nil {
		return fmt.Errorf("remove group member: %w", err)
	}
	return nil
}

func parseMemberIDs(members []string, groupID string, logger *log.Helper) []int64 {
	result := make([]int64, 0, len(members))
	for _, member := range members {
		id, err := strconv.ParseInt(member, 10, 64)
		if err != nil {
			logger.Warnf("invalid member id in group %s: %s", groupID, member)
			continue
		}
		result = append(result, id)
	}
	return result
}

// InMemoryService is a small implementation used by tests.
type InMemoryService struct {
	mu      sync.RWMutex
	members map[string]map[int64]struct{}
}

func NewInMemoryService() *InMemoryService {
	return &InMemoryService{members: make(map[string]map[int64]struct{})}
}

func (s *InMemoryService) GetGroupMembers(_ context.Context, groupID string) ([]int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	group := s.members[groupID]
	out := make([]int64, 0, len(group))
	for id := range group {
		out = append(out, id)
	}
	return out, nil
}

func (s *InMemoryService) AddGroupMember(_ context.Context, groupID string, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.members[groupID] == nil {
		s.members[groupID] = make(map[int64]struct{})
	}
	s.members[groupID][userID] = struct{}{}
	return nil
}

func (s *InMemoryService) RemoveGroupMember(_ context.Context, groupID string, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.members[groupID], userID)
	return nil
}

var _ = context.Background
