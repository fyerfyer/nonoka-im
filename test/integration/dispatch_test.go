package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/conf"
	"nonoka-im/internal/gateway"
	"nonoka-im/internal/service"

	durationpb "google.golang.org/protobuf/types/known/durationpb"
)

// TestDispatch_Fallback_LocalNode verifies that when no gateway nodes are
// registered in Redis, the dispatch service falls back to the local node URL.
func TestDispatch_Fallback_LocalNode(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	ctx := context.Background()

	// Ensure no gateway nodes are registered
	ts.redis.FlushDB(ctx)

	// Create dispatch service with local node config
	dispatchConf := &conf.Dispatch{
		Strategy: "consistent_hash",
	}
	svc := service.NewDispatchService(ts.redis, dispatchConf, testLogger)

	// Request gateway without user_id
	resp, err := svc.Gateway(ctx, &v1.GetGatewayRequest{})
	if err != nil {
		t.Fatalf("gateway dispatch failed: %v", err)
	}
	if resp.GatewayUrl == "" {
		t.Fatalf("expected non-empty gateway URL")
	}
	// Should fall back to localhost default
	if resp.GatewayUrl != "ws://localhost:8000/ws" {
		t.Fatalf("expected fallback URL ws://localhost:8000/ws, got %s", resp.GatewayUrl)
	}
}

// TestDispatch_SingleNode_Heartbeat verifies that after a gateway node
// registers itself via heartbeat, dispatch returns that node's URL.
func TestDispatch_SingleNode_Heartbeat(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	ctx := context.Background()
	ts.redis.FlushDB(ctx)

	// Create a manager and registry to simulate a gateway node
	mgr := gateway.NewManager(testLogger)
	registry := gateway.NewGatewayRegistry(
		ts.redis,
		"gateway-alpha",
		"ws://10.0.0.1:8080/ws",
		"10.0.0.1:9000",
		2*time.Second,
		10*time.Second,
		mgr,
		testLogger,
	)

	// Start heartbeat and report once
	registry.StartHeartbeat()
	defer registry.Stop(ctx)

	// Wait for heartbeat to propagate
	time.Sleep(200 * time.Millisecond)

	// Create dispatch service
	dispatchConf := &conf.Dispatch{
		Strategy: "consistent_hash",
	}
	svc := service.NewDispatchService(ts.redis, dispatchConf, testLogger)

	// Dispatch should return the registered node
	resp, err := svc.Gateway(ctx, &v1.GetGatewayRequest{})
	if err != nil {
		t.Fatalf("gateway dispatch failed: %v", err)
	}
	if resp.GatewayUrl != "ws://10.0.0.1:8080/ws" {
		t.Fatalf("expected ws://10.0.0.1:8080/ws, got %s", resp.GatewayUrl)
	}
}

// TestDispatch_MultipleNodes_ConsistentHash verifies that with multiple
// registered nodes, the same user_id always gets the same gateway.
func TestDispatch_MultipleNodes_ConsistentHash(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	ctx := context.Background()
	ts.redis.FlushDB(ctx)

	// Register 3 gateway nodes with different connection counts
	nodes := []struct {
		nodeID string
		url    string
		conns  int
	}{
		{"gw-1", "ws://10.0.0.1:8080/ws", 10},
		{"gw-2", "ws://10.0.0.2:8080/ws", 20},
		{"gw-3", "ws://10.0.0.3:8080/ws", 5},
	}

	for _, n := range nodes {
		// Manually register nodes in Redis.
		nodeKey := fmt.Sprintf("im:gateway:%s", n.nodeID)
		pipe := ts.redis.Pipeline()
		pipe.SAdd(ctx, "im:gateway:nodes", n.nodeID)
		pipe.HSet(ctx, nodeKey, map[string]interface{}{
			"url":       n.url,
			"conns":     n.conns,
			"heartbeat": time.Now().Unix(),
		})
		pipe.Expire(ctx, nodeKey, 30*time.Second)
		if _, err := pipe.Exec(ctx); err != nil {
			t.Fatalf("failed to register node %s: %v", n.nodeID, err)
		}
	}

	dispatchConf := &conf.Dispatch{
		Strategy: "consistent_hash",
	}
	svc := service.NewDispatchService(ts.redis, dispatchConf, testLogger)

	// Same user_id should always get the same gateway
	var firstURL string
	for i := 0; i < 10; i++ {
		resp, err := svc.Gateway(ctx, &v1.GetGatewayRequest{UserId: 42})
		if err != nil {
			t.Fatalf("gateway dispatch failed: %v", err)
		}
		if i == 0 {
			firstURL = resp.GatewayUrl
			t.Logf("user_id=42 -> %s", firstURL)
		} else if resp.GatewayUrl != firstURL {
			t.Fatalf("consistent hash violated: expected %s, got %s", firstURL, resp.GatewayUrl)
		}
	}

	// Different user_id may get a different gateway (not guaranteed, but likely)
	resp2, _ := svc.Gateway(ctx, &v1.GetGatewayRequest{UserId: 999})
	t.Logf("user_id=999 -> %s", resp2.GatewayUrl)
}

// TestDispatch_MultipleNodes_LeastConnections verifies that the
// least_connections strategy picks the node with fewest connections.
func TestDispatch_MultipleNodes_LeastConnections(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	ctx := context.Background()
	ts.redis.FlushDB(ctx)

	// Register 3 nodes with different connection counts
	nodes := []struct {
		nodeID string
		url    string
		conns  int
	}{
		{"gw-heavy", "ws://10.0.0.1:8080/ws", 100},
		{"gw-light", "ws://10.0.0.2:8080/ws", 5},
		{"gw-mid", "ws://10.0.0.3:8080/ws", 50},
	}

	for _, n := range nodes {
		nodeKey := fmt.Sprintf("im:gateway:%s", n.nodeID)
		pipe := ts.redis.Pipeline()
		pipe.SAdd(ctx, "im:gateway:nodes", n.nodeID)
		pipe.HSet(ctx, nodeKey, map[string]interface{}{
			"url":       n.url,
			"conns":     n.conns,
			"heartbeat": time.Now().Unix(),
		})
		pipe.Expire(ctx, nodeKey, 30*time.Second)
		if _, err := pipe.Exec(ctx); err != nil {
			t.Fatalf("failed to register node %s: %v", n.nodeID, err)
		}
	}

	dispatchConf := &conf.Dispatch{
		Strategy: "least_connections",
	}
	svc := service.NewDispatchService(ts.redis, dispatchConf, testLogger)

	// Should always pick the lightest node
	for i := 0; i < 5; i++ {
		resp, err := svc.Gateway(ctx, &v1.GetGatewayRequest{UserId: int64(i)})
		if err != nil {
			t.Fatalf("gateway dispatch failed: %v", err)
		}
		if resp.GatewayUrl != "ws://10.0.0.2:8080/ws" {
			t.Fatalf("expected least-loaded gateway ws://10.0.0.2:8080/ws, got %s", resp.GatewayUrl)
		}
	}
}

// TestDispatch_StaleNode_Excluded verifies that nodes with expired
// heartbeats are excluded from dispatch.
func TestDispatch_StaleNode_Excluded(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	ctx := context.Background()
	ts.redis.FlushDB(ctx)

	// Register a fresh node and a stale node
	freshNode := struct {
		nodeID string
		url    string
	}{"gw-fresh", "ws://10.0.0.1:8080/ws"}

	staleNode := struct {
		nodeID string
		url    string
	}{"gw-stale", "ws://10.0.0.2:8080/ws"}

	// Fresh node
	pipe := ts.redis.Pipeline()
	pipe.SAdd(ctx, "im:gateway:nodes", freshNode.nodeID)
	pipe.HSet(ctx, fmt.Sprintf("im:gateway:%s", freshNode.nodeID), map[string]interface{}{
		"url":       freshNode.url,
		"conns":     10,
		"heartbeat": time.Now().Unix(),
	})

	// Stale node (heartbeat 2 minutes ago)
	pipe.SAdd(ctx, "im:gateway:nodes", staleNode.nodeID)
	pipe.HSet(ctx, fmt.Sprintf("im:gateway:%s", staleNode.nodeID), map[string]interface{}{
		"url":       staleNode.url,
		"conns":     5,
		"heartbeat": time.Now().Add(-2 * time.Minute).Unix(),
	})
	if _, err := pipe.Exec(ctx); err != nil {
		t.Fatalf("failed to register nodes: %v", err)
	}

	// Use a short TTL so the stale node gets excluded
	dispatchConf := &conf.Dispatch{
		Strategy: "least_connections",
		NodeTtl:  durationpb.New(30 * time.Second),
	}
	svc := service.NewDispatchService(ts.redis, dispatchConf, testLogger)

	// Should only return the fresh node
	resp, err := svc.Gateway(ctx, &v1.GetGatewayRequest{})
	if err != nil {
		t.Fatalf("gateway dispatch failed: %v", err)
	}
	if resp.GatewayUrl != freshNode.url {
		t.Fatalf("expected fresh node %s, got %s (stale node was not excluded)", freshNode.url, resp.GatewayUrl)
	}
}

// TestDispatch_HTTP_Endpoint verifies the /v1/dispatch/gateway HTTP endpoint.
func TestDispatch_HTTP_Endpoint(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	// Call the dispatch endpoint via HTTP
	resp := httpGet(t, testBaseURL+"/v1/dispatch/gateway", "")
	assertStatusCode(t, resp, http.StatusOK)

	var reply v1.GetGatewayReply
	decodeProtoJSON(t, resp.Body, &reply)
	resp.Body.Close()

	if reply.GatewayUrl == "" {
		t.Fatalf("expected non-empty gateway URL in HTTP response")
	}
	t.Logf("HTTP dispatch returned: %s", reply.GatewayUrl)
}
