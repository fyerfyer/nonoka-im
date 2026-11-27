package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"nonoka-im/internal/im/seq"
)

// TestSeq_NodeLeaseIsExclusive verifies two msgworker replicas cannot acquire
// the same Snowflake node id.
func TestSeq_NodeLeaseIsExclusive(t *testing.T) {
	redisClient := setupTestRedis(t)
	ctx := context.Background()
	leaseA, err := seq.AcquireNodeID(ctx, redisClient, "test-holder-a", testLogger)
	if err != nil {
		t.Fatalf("lease A: %v", err)
	}
	defer leaseA.Release(ctx)
	leaseB, err := seq.AcquireNodeID(ctx, redisClient, "test-holder-b", testLogger)
	if err != nil {
		t.Fatalf("lease B: %v", err)
	}
	defer leaseB.Release(ctx)
	if leaseA.ID == leaseB.ID {
		t.Fatalf("two holders received the same node id %d", leaseA.ID)
	}
}

func TestSeq_NodeLeaseLossIsReported(t *testing.T) {
	redisClient := setupTestRedis(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	lease, err := seq.AcquireNodeID(ctx, redisClient, "test-holder-lost", testLogger)
	if err != nil {
		t.Fatalf("lease: %v", err)
	}
	defer lease.Release(context.Background())
	key := fmt.Sprintf("im:msgid:node:%d", lease.ID)
	if err := redisClient.Set(ctx, key, "another-process", time.Minute).Err(); err != nil {
		t.Fatalf("steal lease: %v", err)
	}
	go lease.KeepAlive(ctx, 10*time.Millisecond)
	select {
	case <-lease.Lost():
	case <-time.After(2 * time.Second):
		t.Fatal("lease loss was not reported; worker would keep generating colliding ids")
	}
}

func TestSeq_MongoAllocatorRangesAreUnique(t *testing.T) {
	worker, _, cleanup := setupMsgWorker(t)
	defer cleanup()
	ctx := context.Background()
	storage := worker.Storage()
	topic := "p2p_1_2-seqallocator"
	first, last, err := storage.AllocateRange(ctx, topic, 2, 5)
	if err != nil {
		t.Fatal(err)
	}
	if first != 6 || last != 7 {
		t.Fatalf("first range = [%d,%d], want [6,7]", first, last)
	}
	first, last, err = storage.AllocateRange(ctx, topic, 2, 5)
	if err != nil {
		t.Fatal(err)
	}
	if first != 8 || last != 9 {
		t.Fatalf("second range = [%d,%d], want [8,9]", first, last)
	}
}

// TestSeq_RecoversFromDurableFloorAfterRedisLoss simulates an evicted Redis
// sequence key. The first allocation after the loss must start above MongoDB's
// durable high-water mark instead of restarting at 1.
func TestSeq_RecoversFromDurableFloorAfterRedisLoss(t *testing.T) {
	redisClient := setupTestRedis(t)
	ctx := context.Background()
	topic := fmt.Sprintf("p2p_1_2-seqrecovery-%d", time.Now().UnixNano())
	key := "im:seq:" + topic
	_ = redisClient.Del(ctx, key).Err()
	defer redisClient.Del(ctx, key)

	first := seq.New(redisClient, testLogger)
	first.SetFloor(func(context.Context, string) (uint64, error) { return 5, nil })
	value, err := first.NextSeq(ctx, topic)
	if err != nil {
		t.Fatalf("first NextSeq: %v", err)
	}
	if value != 6 {
		t.Fatalf("first seq = %d, want 6 (durable floor 5 + 1)", value)
	}

	// Losing the Redis key is the failure this floor is designed to survive.
	if err := redisClient.Del(ctx, key).Err(); err != nil {
		t.Fatal(err)
	}
	second := seq.New(redisClient, testLogger)
	second.SetFloor(func(context.Context, string) (uint64, error) { return 20, nil })
	value, err = second.NextSeq(ctx, topic)
	if err != nil {
		t.Fatalf("NextSeq after recovery: %v", err)
	}
	if value != 21 {
		t.Fatalf("recovered seq = %d, want 21 (durable floor 20 + 1)", value)
	}
}
