package msgworker

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
)

// GroupMemberService provides group membership information.
// Implementations can be backed by Redis, a database, or an external service.
type GroupMemberService interface {
	// GetGroupMembers returns all member IDs of a group.
	GetGroupMembers(ctx context.Context, groupID string) ([]int64, error)
}

// RedisGroupMemberService is a simple Redis-backed implementation of
// GroupMemberService. Members are stored in a Redis Set per group.
type RedisGroupMemberService struct {
	redis redis.UniversalClient
	log   *log.Helper
}

// NewRedisGroupMemberService creates a new RedisGroupMemberService.
func NewRedisGroupMemberService(redis redis.UniversalClient, logger log.Logger) *RedisGroupMemberService {
	return &RedisGroupMemberService{
		redis: redis,
		log:   log.NewHelper(logger),
	}
}

// GetGroupMembers returns all member IDs of a group from Redis.
func (s *RedisGroupMemberService) GetGroupMembers(ctx context.Context, groupID string) ([]int64, error) {
	key := fmt.Sprintf("im:group:members:%s", groupID)
	members, err := s.redis.SMembers(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("smembers failed for group %s: %w", groupID, err)
	}
	result := make([]int64, 0, len(members))
	for _, m := range members {
		id, err := strconv.ParseInt(m, 10, 64)
		if err != nil {
			s.log.Warnf("invalid member id in group %s: %s", groupID, m)
			continue
		}
		result = append(result, id)
	}
	return result, nil
}

// AddGroupMember adds a user to a group.
func (s *RedisGroupMemberService) AddGroupMember(ctx context.Context, groupID string, userID int64) error {
	key := fmt.Sprintf("im:group:members:%s", groupID)
	return s.redis.SAdd(ctx, key, userID).Err()
}

// RemoveGroupMember removes a user from a group.
func (s *RedisGroupMemberService) RemoveGroupMember(ctx context.Context, groupID string, userID int64) error {
	key := fmt.Sprintf("im:group:members:%s", groupID)
	return s.redis.SRem(ctx, key, userID).Err()
}

// InMemoryGroupMemberService is a simple in-memory implementation for testing.
type InMemoryGroupMemberService struct {
	mu      sync.RWMutex
	members map[string]map[int64]struct{} // groupID -> set of userIDs
}

// NewInMemoryGroupMemberService creates a new InMemoryGroupMemberService.
func NewInMemoryGroupMemberService() *InMemoryGroupMemberService {
	return &InMemoryGroupMemberService{
		members: make(map[string]map[int64]struct{}),
	}
}

// GetGroupMembers returns all member IDs of a group.
func (s *InMemoryGroupMemberService) GetGroupMembers(ctx context.Context, groupID string) ([]int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	group, ok := s.members[groupID]
	if !ok {
		return nil, nil
	}
	result := make([]int64, 0, len(group))
	for id := range group {
		result = append(result, id)
	}
	return result, nil
}

// AddGroupMember adds a user to a group.
func (s *InMemoryGroupMemberService) AddGroupMember(ctx context.Context, groupID string, userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.members[groupID] == nil {
		s.members[groupID] = make(map[int64]struct{})
	}
	s.members[groupID][userID] = struct{}{}
}

// RemoveGroupMember removes a user from a group.
func (s *InMemoryGroupMemberService) RemoveGroupMember(ctx context.Context, groupID string, userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if group, ok := s.members[groupID]; ok {
		delete(group, userID)
	}
}

// ExtractGroupID extracts the group ID from a group topic string.
// Topic format: grp_<groupID>
func ExtractGroupID(topic string) (string, error) {
	if !strings.HasPrefix(topic, "grp_") {
		return "", fmt.Errorf("invalid group topic format: %s", topic)
	}
	return strings.TrimPrefix(topic, "grp_"), nil
}
