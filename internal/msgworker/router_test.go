package msgworker

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
)

// TestGatewayRouter_SessionCache_HitAndMiss verifies that the local session
// route cache is used on subsequent lookups and that negative results are also
// cached briefly.
func TestGatewayRouter_SessionCache_HitAndMiss(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping redis-dependent test in short mode")
	}

	ctx := context.Background()
	redisCli := newTestRedis(t)
	defer redisCli.Close()

	router := NewGatewayRouter(redisCli, 30*time.Second, log.NewStdLogger(io.Discard))
	router.SetSessionCacheTTL(5 * time.Second)

	// Seed Redis session index for user 1 on node-a.
	if err := redisCli.HSet(ctx, "im:session:1:devices", "d1", "node-a").Err(); err != nil {
		t.Fatalf("hset failed: %v", err)
	}

	alive := []*GatewayNodeInfo{{NodeID: "node-a", GrpcAddr: "127.0.0.1:9000"}}

	// First lookup should hit Redis and populate the cache.
	mapping, err := router.ResolveUserNodesWithNodes(ctx, []int64{1}, alive)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if len(mapping["node-a"]) != 1 || mapping["node-a"][0] != 1 {
		t.Fatalf("unexpected mapping: %v", mapping)
	}

	// Delete Redis entry; subsequent lookup should still succeed via cache.
	if err := redisCli.Del(ctx, "im:session:1:devices").Err(); err != nil {
		t.Fatalf("del failed: %v", err)
	}
	mapping, err = router.ResolveUserNodesWithNodes(ctx, []int64{1}, alive)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if len(mapping["node-a"]) != 1 {
		t.Fatalf("expected cache hit for node-a, got: %v", mapping)
	}

	// Invalidate cache entry and verify Redis miss (user offline).
	router.invalidateSessionCache(1)
	mapping, err = router.ResolveUserNodesWithNodes(ctx, []int64{1}, alive)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if len(mapping) != 0 {
		t.Fatalf("expected empty mapping after invalidation, got: %v", mapping)
	}
}

// TestGatewayRouter_SessionCache_PubSubInvalidation verifies that a Pub/Sub
// session change event invalidates the local cache.
func TestGatewayRouter_SessionCache_PubSubInvalidation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping redis-dependent test in short mode")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	redisCli := newTestRedis(t)
	defer redisCli.Close()

	router := NewGatewayRouter(redisCli, 30*time.Second, log.NewStdLogger(io.Discard))
	router.SetSessionCacheTTL(5 * time.Second)
	if err := router.Start(ctx); err != nil {
		t.Fatalf("router start failed: %v", err)
	}
	defer router.Stop()

	// Seed session and warm cache.
	if err := redisCli.HSet(ctx, "im:session:2:devices", "d1", "node-b").Err(); err != nil {
		t.Fatalf("hset failed: %v", err)
	}
	alive := []*GatewayNodeInfo{{NodeID: "node-b", GrpcAddr: "127.0.0.1:9001"}}
	if _, err := router.ResolveUserNodesWithNodes(ctx, []int64{2}, alive); err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	// Delete Redis entry and publish a session change event.
	if err := redisCli.Del(ctx, "im:session:2:devices").Err(); err != nil {
		t.Fatalf("del failed: %v", err)
	}
	event := `{"user_id":2,"device_id":"d1","action":"del","node_id":"node-b","at":0}`
	if err := redisCli.Publish(ctx, sessionChangeChannel, event).Err(); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	// Wait for the Pub/Sub message to arrive.
	time.Sleep(200 * time.Millisecond)

	mapping, err := router.ResolveUserNodesWithNodes(ctx, []int64{2}, alive)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if len(mapping) != 0 {
		t.Fatalf("expected cache invalidation after pubsub event, got: %v", mapping)
	}
}

func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	cli := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
	if err := cli.Ping(context.Background()).Err(); err != nil {
		t.Skipf("redis not available: %v", err)
	}
	return cli
}
