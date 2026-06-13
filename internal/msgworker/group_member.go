package msgworker

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"nonoka-im/internal/data"
	"github.com/redis/go-redis/v9"
)

// GroupMemberService provides group membership information.
// The authoritative membership data lives in persistent storage (PostgreSQL);
// implementations may optionally use Redis as a read-through cache.
type GroupMemberService interface {
	// GetGroupMembers returns all member IDs of a group.
	GetGroupMembers(ctx context.Context, groupID string) ([]int64, error)
}

// PersistentGroupMemberService is backed by PostgreSQL and optionally uses
// Redis as a read-through cache. Membership changes are written through to
// both layers when Redis is available; reads prefer the cache and fall back
// to the database on cache miss.
type PersistentGroupMemberService struct {
	repo  data.GroupMemberRepo
	redis redis.UniversalClient
	log   *log.Helper
}

// NewPersistentGroupMemberService creates a new PersistentGroupMemberService.
func NewPersistentGroupMemberService(repo data.GroupMemberRepo, redis redis.UniversalClient, logger log.Logger) *PersistentGroupMemberService {
	return &PersistentGroupMemberService{
		repo:  repo,
		redis: redis,
		log:   log.NewHelper(logger),
	}
}

func (s *PersistentGroupMemberService) groupMembersKey(groupID string) string {
	return fmt.Sprintf("im:group:members:%s", groupID)
}

// GetGroupMembers returns all member IDs of a group.
func (s *PersistentGroupMemberService) GetGroupMembers(ctx context.Context, groupID string) ([]int64, error) {
	key := s.groupMembersKey(groupID)

	// Try cache first.
	if s.redis != nil {
		members, err := s.redis.SMembers(ctx, key).Result()
		if err == nil && len(members) > 0 {
			return parseMemberIDs(members, groupID, s.log), nil
		}
	}

	// Cache miss or unavailable: load from PostgreSQL.
	ids, err := s.repo.GetMembers(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("get group members: %w", err)
	}

	// Warm the cache.
	if s.redis != nil && len(ids) > 0 {
		members := make([]interface{}, len(ids))
		for i, id := range ids {
			members[i] = id
		}
		if err := s.redis.SAdd(ctx, key, members...).Err(); err != nil {
			s.log.Warnf("warm group member cache failed: group=%s err=%v", groupID, err)
		} else {
			_ = s.redis.Expire(ctx, key, 24*time.Hour)
		}
	}

	return ids, nil
}

// AddGroupMember adds a user to a group.
func (s *PersistentGroupMemberService) AddGroupMember(ctx context.Context, groupID string, userID int64) error {
	if err := s.repo.AddMember(ctx, groupID, userID); err != nil {
		return fmt.Errorf("add group member: %w", err)
	}
	if s.redis != nil {
		if err := s.redis.SAdd(ctx, s.groupMembersKey(groupID), userID).Err(); err != nil {
			s.log.Warnf("add group member to cache failed: group=%s user=%d err=%v", groupID, userID, err)
		}
	}
	return nil
}

// RemoveGroupMember removes a user from a group.
func (s *PersistentGroupMemberService) RemoveGroupMember(ctx context.Context, groupID string, userID int64) error {
	// Invalidate cache first so concurrent readers don't see stale data even if
	// the database removal fails.
	if s.redis != nil {
		if err := s.redis.SRem(ctx, s.groupMembersKey(groupID), userID).Err(); err != nil {
			s.log.Warnf("remove group member from cache failed: group=%s user=%d err=%v", groupID, userID, err)
		}
	}
	if err := s.repo.RemoveMember(ctx, groupID, userID); err != nil {
		return fmt.Errorf("remove group member: %w", err)
	}
	return nil
}

func parseMemberIDs(members []string, groupID string, log *log.Helper) []int64 {
	result := make([]int64, 0, len(members))
	for _, m := range members {
		id, err := strconv.ParseInt(m, 10, 64)
		if err != nil {
			log.Warnf("invalid member id in group %s: %s", groupID, m)
			continue
		}
		result = append(result, id)
	}
	return result
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
