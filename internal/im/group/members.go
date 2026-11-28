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

type memberCacheEntry struct {
	ids     []int64
	expires time.Time
}

// Service resolves group membership through an in-process L1 cache backed by
// a Redis L2 cache and PostgreSQL. Group messages hit this path once per
// message; avoiding a Redis round-trip here matters for large groups.
type Service struct {
	repo  MemberRepository
	redis redis.UniversalClient
	log   *log.Helper

	cacheMu   sync.RWMutex
	cache     map[string]memberCacheEntry
	cacheTTL  time.Duration
	lastPrune time.Time
}

func NewService(repo MemberRepository, redis redis.UniversalClient, logger log.Logger) *Service {
	return &Service{
		repo: repo, redis: redis, log: log.NewHelper(logger),
		cache: make(map[string]memberCacheEntry), cacheTTL: 5 * time.Second,
	}
}

// SetCacheTTL overrides the local membership cache TTL. Must be called before
// the service starts serving traffic.
func (s *Service) SetCacheTTL(ttl time.Duration) {
	if ttl > 0 {
		s.cacheTTL = ttl
	}
}

func (s *Service) key(groupID string) string { return "im:group:members:" + groupID }

func (s *Service) cached(groupID string) ([]int64, bool) {
	s.cacheMu.RLock()
	entry, ok := s.cache[groupID]
	s.cacheMu.RUnlock()
	if !ok || time.Now().After(entry.expires) {
		if ok {
			s.cacheMu.Lock()
			delete(s.cache, groupID)
			s.cacheMu.Unlock()
		}
		return nil, false
	}
	out := make([]int64, len(entry.ids))
	copy(out, entry.ids)
	return out, true
}

func (s *Service) remember(groupID string, ids []int64) {
	if len(ids) == 0 {
		return
	}
	copied := make([]int64, len(ids))
	copy(copied, ids)
	ttl := s.cacheTTL
	if ttl <= 0 {
		ttl = 5 * time.Second
	}
	s.cacheMu.Lock()
	now := time.Now()
	if s.lastPrune.IsZero() || now.Sub(s.lastPrune) > ttl {
		for key, entry := range s.cache {
			if now.After(entry.expires) {
				delete(s.cache, key)
			}
		}
		s.lastPrune = now
	}
	s.cache[groupID] = memberCacheEntry{ids: copied, expires: now.Add(ttl)}
	s.cacheMu.Unlock()
}

func (s *Service) forget(groupID string) {
	s.cacheMu.Lock()
	delete(s.cache, groupID)
	s.cacheMu.Unlock()
}

func (s *Service) GetGroupMembers(ctx context.Context, groupID string) ([]int64, error) {
	if ids, ok := s.cached(groupID); ok {
		return ids, nil
	}
	if s.redis != nil {
		members, err := s.redis.SMembers(ctx, s.key(groupID)).Result()
		if err == nil && len(members) > 0 {
			ids := parseMemberIDs(members, groupID, s.log)
			s.remember(groupID, ids)
			return ids, nil
		}
	}
	ids, err := s.repo.GetMembers(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("get group members: %w", err)
	}
	if len(ids) > 0 {
		s.remember(groupID, ids)
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
	s.forget(groupID)
	if s.redis != nil {
		if err := s.redis.SAdd(ctx, s.key(groupID), userID).Err(); err != nil {
			// A partial cache is more dangerous than a miss: drop the whole
			// set so the next reader repopulates from PostgreSQL.
			s.log.Warnf("add group member to cache failed, invalidating cache: group=%s user=%d err=%v", groupID, userID, err)
			_ = s.redis.Del(ctx, s.key(groupID)).Err()
		}
	}
	return nil
}

func (s *Service) RemoveGroupMember(ctx context.Context, groupID string, userID int64) error {
	// Persist the removal first. Invalidating the cache before the database
	// write would hide the member from fan-out for the Redis cache TTL if the
	// write failed.
	if err := s.repo.RemoveMember(ctx, groupID, userID); err != nil {
		return fmt.Errorf("remove group member: %w", err)
	}
	s.forget(groupID)
	if s.redis != nil {
		if err := s.redis.SRem(ctx, s.key(groupID), userID).Err(); err != nil {
			// Never leave a removed member in the shared cache. The DB call
			// above remains the source of truth; a cache miss will refetch it.
			s.log.Warnf("remove group member from cache failed, invalidating cache: group=%s user=%d err=%v", groupID, userID, err)
			_ = s.redis.Del(ctx, s.key(groupID)).Err()
		}
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
