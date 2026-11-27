package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestGatewayAdminList(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	adminToken, adminID := registerAndLogin(t, "gateway-admin", "123456")
	if err := ts.openFGA.SetPlatformAdmin(context.Background(), adminID, true); err != nil {
		t.Fatalf("grant platform admin: %v", err)
	}
	userToken, _ := registerAndLogin(t, "gateway-viewer", "123456")

	resp := httpGet(t, testBaseURL+"/v1/admin/gateways", userToken)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("non-admin status = %d: %s", resp.StatusCode, body)
	}

	ctx := context.Background()
	nodeID := fmt.Sprintf("gateway-test-%d", time.Now().UnixNano())
	nodeKey := "im:gateway:" + nodeID
	if err := ts.redis.SAdd(ctx, "im:gateway:nodes", nodeID).Err(); err != nil {
		t.Fatalf("seed node index: %v", err)
	}
	if err := ts.redis.HSet(ctx, nodeKey, map[string]any{
		"url":       "ws://127.0.0.1:18000/ws",
		"grpc_addr": "127.0.0.1:19000",
		"conns":     7,
		"heartbeat": time.Now().Unix(),
	}).Err(); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	t.Cleanup(func() {
		_ = ts.redis.SRem(context.Background(), "im:gateway:nodes", nodeID).Err()
		_ = ts.redis.Del(context.Background(), nodeKey).Err()
	})

	resp = httpGet(t, testBaseURL+"/v1/admin/gateways", adminToken)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("admin status = %d: %s", resp.StatusCode, body)
	}
	var reply struct {
		Strategy       string `json:"strategy"`
		NodeTTLSeconds int64  `json:"nodeTtlSeconds"`
		Degraded       bool   `json:"degraded"`
		Gateways       []struct {
			NodeID      string `json:"nodeId"`
			URL         string `json:"url"`
			GrpcAddr    string `json:"grpcAddr"`
			Connections int    `json:"connections"`
			HeartbeatAt string `json:"heartbeatAt"`
			Alive       bool   `json:"alive"`
		} `json:"gateways"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	if reply.Strategy != "consistent_hash" || reply.NodeTTLSeconds != 30 || reply.Degraded {
		t.Fatalf("unexpected reply metadata: %+v", reply)
	}
	for _, node := range reply.Gateways {
		if node.NodeID != nodeID {
			continue
		}
		if node.URL != "ws://127.0.0.1:18000/ws" || node.GrpcAddr != "127.0.0.1:19000" ||
			node.Connections != 7 || !node.Alive || node.HeartbeatAt == "" {
			t.Fatalf("unexpected node: %+v", node)
		}
		return
	}
	t.Fatalf("seeded gateway %q not returned: %+v", nodeID, reply.Gateways)
}
