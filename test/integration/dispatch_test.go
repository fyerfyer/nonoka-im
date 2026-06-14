package integration

import (
	"context"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/conf"
	"nonoka-im/internal/gateway"
	"nonoka-im/internal/service"
)

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
// gateway nodes registered via real heartbeats, the same user_id always gets
// the same gateway under consistent hashing.
func TestDispatch_MultipleNodes_ConsistentHash(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	ctx := context.Background()
	ts.redis.FlushDB(ctx)

	nodes := []struct {
		nodeID string
		url    string
	}{
		{"gw-1", "ws://10.0.0.1:8080/ws"},
		{"gw-2", "ws://10.0.0.2:8080/ws"},
		{"gw-3", "ws://10.0.0.3:8080/ws"},
	}

	registries := make([]*gateway.GatewayRegistry, 0, len(nodes))
	for _, n := range nodes {
		mgr := gateway.NewManager(testLogger)
		registry := gateway.NewGatewayRegistry(
			ts.redis,
			n.nodeID,
			n.url,
			"10.0.0.0:9000",
			2*time.Second,
			10*time.Second,
			mgr,
			testLogger,
		)
		registry.StartHeartbeat()
		registries = append(registries, registry)
	}
	defer func() {
		for _, r := range registries {
			r.Stop(ctx)
		}
	}()

	// Wait for heartbeats to propagate
	time.Sleep(300 * time.Millisecond)

	dispatchConf := &conf.Dispatch{
		Strategy: "consistent_hash",
	}
	svc := service.NewDispatchService(ts.redis, dispatchConf, testLogger)

	// Same user_id should always get the same gateway
	var firstURL string
	for i := range 10 {
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

// TestDispatch_HTTP_Endpoint verifies the /v1/dispatch/gateway HTTP endpoint.
func TestDispatch_HTTP_Endpoint(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	// Call the dispatch endpoint via HTTP
	resp := httpGet(t, testBaseURL+"/v1/dispatch/gateway", "")
	assertStatusCode(t, resp, 200)

	var reply v1.GetGatewayReply
	decodeProtoJSON(t, resp.Body, &reply)
	resp.Body.Close()

	if reply.GatewayUrl == "" {
		t.Fatalf("expected non-empty gateway URL in HTTP response")
	}
	t.Logf("HTTP dispatch returned: %s", reply.GatewayUrl)
}
