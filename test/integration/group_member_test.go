package integration

import (
	"context"
	"sort"
	"testing"
	"time"

	"nonoka-im/internal/conf"
	"nonoka-im/internal/data"
	"nonoka-im/internal/msgworker"
)

func setupGroupMemberTest(t *testing.T) (*data.Data, func()) {
	confData := &conf.Data{
		Database: &conf.Data_Database{
			Driver: "postgres",
			Source: "host=127.0.0.1 user=postgres password=root dbname=nonoka_im_test port=5433 sslmode=disable TimeZone=Asia/Shanghai",
		},
		Redis: &conf.Data_Redis{
			Addr: "127.0.0.1:6380",
		},
	}
	d, cleanup, err := data.NewData(confData)
	if err != nil {
		t.Fatalf("failed to create data layer: %v", err)
	}
	if err := d.CleanTestData(); err != nil {
		cleanup()
		t.Fatalf("failed to clean test data: %v", err)
	}
	if d.Redis != nil {
		if err := d.Redis.FlushDB(context.Background()).Err(); err != nil {
			cleanup()
			t.Fatalf("failed to flush redis: %v", err)
		}
	}
	return d, cleanup
}

func sortedInts(ids []int64) []int64 {
	out := make([]int64, len(ids))
	copy(out, ids)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func intSlicesEqual(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	aa := sortedInts(a)
	bb := sortedInts(b)
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
}

func TestPersistentGroupMemberService_BasicCRUDAndCache(t *testing.T) {
	d, cleanup := setupGroupMemberTest(t)
	defer cleanup()
	ctx := context.Background()

	repo := data.NewGroupMemberRepo(d, testLogger)
	svc := msgworker.NewPersistentGroupMemberService(repo, d.Redis, testLogger)

	groupID := "persistent_group_test"
	cacheKey := "im:group:members:" + groupID

	// 1. Add members through the service (writes to PG + Redis cache).
	for _, uid := range []int64{100, 200, 300} {
		if err := svc.AddGroupMember(ctx, groupID, uid); err != nil {
			t.Fatalf("AddGroupMember failed: %v", err)
		}
	}

	// 2. First read: cache may or may not be warm; result must come from PG + cache.
	members1, err := svc.GetGroupMembers(ctx, groupID)
	if err != nil {
		t.Fatalf("GetGroupMembers first call failed: %v", err)
	}
	if !intSlicesEqual(members1, []int64{100, 200, 300}) {
		t.Fatalf("expected members [100 200 300], got %v", members1)
	}

	// 3. Verify Redis cache was warmed.
	cached, err := d.Redis.SMembers(ctx, cacheKey).Result()
	if err != nil {
		t.Fatalf("SMembers failed: %v", err)
	}
	if len(cached) != 3 {
		t.Fatalf("expected 3 cached members, got %d", len(cached))
	}

	// 4. Second read should hit cache and return the same members.
	members2, err := svc.GetGroupMembers(ctx, groupID)
	if err != nil {
		t.Fatalf("GetGroupMembers second call failed: %v", err)
	}
	if !intSlicesEqual(members2, []int64{100, 200, 300}) {
		t.Fatalf("expected cached members [100 200 300], got %v", members2)
	}

	// 5. Add another member through the service; cache should be updated.
	if err := svc.AddGroupMember(ctx, groupID, 400); err != nil {
		t.Fatalf("AddGroupMember 400 failed: %v", err)
	}
	members3, err := svc.GetGroupMembers(ctx, groupID)
	if err != nil {
		t.Fatalf("GetGroupMembers after add failed: %v", err)
	}
	if !intSlicesEqual(members3, []int64{100, 200, 300, 400}) {
		t.Fatalf("expected members [100 200 300 400], got %v", members3)
	}

	// 6. Remove a member through the service; cache should be invalidated/updated.
	if err := svc.RemoveGroupMember(ctx, groupID, 200); err != nil {
		t.Fatalf("RemoveGroupMember failed: %v", err)
	}
	members4, err := svc.GetGroupMembers(ctx, groupID)
	if err != nil {
		t.Fatalf("GetGroupMembers after remove failed: %v", err)
	}
	if !intSlicesEqual(members4, []int64{100, 300, 400}) {
		t.Fatalf("expected members [100 300 400], got %v", members4)
	}
}

func TestPersistentGroupMemberService_CacheMissFallsBackToPostgres(t *testing.T) {
	d, cleanup := setupGroupMemberTest(t)
	defer cleanup()
	ctx := context.Background()

	repo := data.NewGroupMemberRepo(d, testLogger)
	svc := msgworker.NewPersistentGroupMemberService(repo, d.Redis, testLogger)

	groupID := "cache_miss_test"
	cacheKey := "im:group:members:" + groupID

	// Seed members directly in PostgreSQL (bypassing cache).
	for _, uid := range []int64{10, 20} {
		if err := repo.AddMember(ctx, groupID, uid); err != nil {
			t.Fatalf("repo.AddMember failed: %v", err)
		}
	}

	// Ensure cache is empty.
	if err := d.Redis.Del(ctx, cacheKey).Err(); err != nil {
		t.Fatalf("delete cache key failed: %v", err)
	}

	// Read through service: cache miss, must fall back to PG and warm cache.
	members, err := svc.GetGroupMembers(ctx, groupID)
	if err != nil {
		t.Fatalf("GetGroupMembers failed: %v", err)
	}
	if !intSlicesEqual(members, []int64{10, 20}) {
		t.Fatalf("expected members [10 20], got %v", members)
	}

	// Verify cache was warmed.
	cached, err := d.Redis.SMembers(ctx, cacheKey).Result()
	if err != nil {
		t.Fatalf("SMembers failed: %v", err)
	}
	if len(cached) != 2 {
		t.Fatalf("expected 2 cached members after warm, got %d", len(cached))
	}
}

func TestPersistentGroupMemberService_NoRedisFallsBackToPostgres(t *testing.T) {
	d, cleanup := setupGroupMemberTest(t)
	defer cleanup()
	ctx := context.Background()

	repo := data.NewGroupMemberRepo(d, testLogger)
	// Pass nil redis: service must still work by reading from PostgreSQL.
	svc := msgworker.NewPersistentGroupMemberService(repo, nil, testLogger)

	groupID := "no_redis_test"
	for _, uid := range []int64{1, 2, 3} {
		if err := svc.AddGroupMember(ctx, groupID, uid); err != nil {
			t.Fatalf("AddGroupMember failed: %v", err)
		}
	}

	members, err := svc.GetGroupMembers(ctx, groupID)
	if err != nil {
		t.Fatalf("GetGroupMembers failed: %v", err)
	}
	if !intSlicesEqual(members, []int64{1, 2, 3}) {
		t.Fatalf("expected members [1 2 3], got %v", members)
	}
}

func TestPersistentGroupMemberService_ConcurrentReadsAreConsistent(t *testing.T) {
	d, cleanup := setupGroupMemberTest(t)
	defer cleanup()
	ctx := context.Background()

	repo := data.NewGroupMemberRepo(d, testLogger)
	svc := msgworker.NewPersistentGroupMemberService(repo, d.Redis, testLogger)

	groupID := "concurrent_test"
	for _, uid := range []int64{1000, 2000, 3000} {
		if err := svc.AddGroupMember(ctx, groupID, uid); err != nil {
			t.Fatalf("AddGroupMember failed: %v", err)
		}
	}

	// Concurrent reads should all see the same set of members.
	done := make(chan []int64, 10)
	for i := 0; i < 10; i++ {
		go func() {
			ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			members, err := svc.GetGroupMembers(ctx2, groupID)
			if err != nil {
				done <- nil
				return
			}
			done <- members
		}()
	}

	for i := 0; i < 10; i++ {
		members := <-done
		if members == nil {
			t.Fatalf("concurrent GetGroupMembers failed")
		}
		if !intSlicesEqual(members, []int64{1000, 2000, 3000}) {
			t.Fatalf("expected members [1000 2000 3000], got %v", members)
		}
	}
}
