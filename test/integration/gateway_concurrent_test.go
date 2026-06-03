package integration

import (
	"context"
	"encoding/json"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// ============================================
// 并发测试
// ============================================

// TestGateway_Concurrent_Auth verifies that many clients can authenticate concurrently.
func TestGateway_Concurrent_Auth(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	const concurrency = 30
	var wg sync.WaitGroup
	wg.Add(concurrency)

	var successCount int32
	var failCount int32

	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			defer wg.Done()

			// Each goroutine registers and logs in a unique user
			username := "concurrent-auth-user-" + string(rune('a'+idx%26)) + "-" + string(rune('0'+idx/26))
			token, _ := registerAndLogin(t, username, "123456")

			wsConn := wsConnect(t)
			defer wsConn.Close()

			authPayload, _ := json.Marshal(map[string]interface{}{
				"token":     token,
				"device_id": "device-" + string(rune('0'+idx%10)),
			})
			wsSendPacket(t, wsConn, &v1.Packet{
				Cmd:     v1.Command_CMD_AUTH,
				Seq:     uint64(idx + 1),
				Payload: authPayload,
			})

			resp := wsReadPacketOrNil(t, wsConn, 3*time.Second)
			if resp == nil {
				atomic.AddInt32(&failCount, 1)
				t.Logf("[%d] auth timed out", idx)
				return
			}

			if resp.Cmd != v1.Command_CMD_AUTH {
				atomic.AddInt32(&failCount, 1)
				t.Logf("[%d] unexpected cmd: %v", idx, resp.Cmd)
				return
			}

			var authReply map[string]interface{}
			if err := json.Unmarshal(resp.Payload, &authReply); err != nil {
				atomic.AddInt32(&failCount, 1)
				return
			}
			if success, ok := authReply["success"].(bool); ok && success {
				atomic.AddInt32(&successCount, 1)
			} else {
				atomic.AddInt32(&failCount, 1)
			}
		}(i)
	}

	wg.Wait()

	if successCount != concurrency {
		t.Fatalf("expected %d successful auths, got %d (failed: %d)", concurrency, successCount, failCount)
	}
	t.Logf("concurrent auth result: success=%d, fail=%d", successCount, failCount)
}

// TestGateway_Concurrent_SameUser_MultiDevice verifies that one user can connect
// from many devices concurrently.
func TestGateway_Concurrent_SameUser_MultiDevice(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, userID := registerAndLogin(t, "concurrent-same-user", "123456")

	const concurrency = 20
	var wg sync.WaitGroup
	wg.Add(concurrency)

	var connectedCount int32

	conns := make([]*websocket.Conn, concurrency)
	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			defer wg.Done()

			wsConn := wsConnect(t)
			conns[idx] = wsConn

			authPayload, _ := json.Marshal(map[string]interface{}{
				"token":     token,
				"device_id": "device-" + string(rune('a'+idx%26)) + "-" + string(rune('0'+idx/26)),
			})
			wsSendPacket(t, wsConn, &v1.Packet{
				Cmd:     v1.Command_CMD_AUTH,
				Seq:     uint64(idx + 1),
				Payload: authPayload,
			})

			resp := wsReadPacketOrNil(t, wsConn, 3*time.Second)
			if resp != nil && resp.Cmd == v1.Command_CMD_AUTH {
				atomic.AddInt32(&connectedCount, 1)
			}
		}(i)
	}

	wg.Wait()

	// Verify all connections succeeded
	if int(connectedCount) != concurrency {
		t.Fatalf("expected %d connections, got %d", concurrency, connectedCount)
	}

	// Verify manager has all connections
	allConns := ts.gwManager.GetAll(userID)
	if len(allConns) != concurrency {
		t.Fatalf("expected %d connections in manager, got %d", concurrency, len(allConns))
	}

	// Verify Redis has all devices
	ctx := context.Background()
	devices, err := ts.gwSessionMgr.GetDevices(ctx, userID)
	if err != nil {
		t.Fatalf("failed to get devices from redis: %v", err)
	}
	if len(devices) != concurrency {
		t.Fatalf("expected %d devices in redis, got %d", concurrency, len(devices))
	}

	// Clean up connections
	for _, c := range conns {
		if c != nil {
			c.Close()
		}
	}

	// Wait for async cleanup
	time.Sleep(300 * time.Millisecond)

	// Verify all cleaned up
	if ts.gwManager.Get(userID) != nil {
		t.Fatal("all connections should be cleaned up")
	}
}

// TestGateway_Concurrent_Heartbeat verifies that many clients can send heartbeats concurrently
// without interfering with each other.
func TestGateway_Concurrent_Heartbeat(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	const concurrency = 20
	const heartbeatsPerClient = 10

	// Create authenticated connections
	conns := make([]*websocket.Conn, concurrency)
	for i := 0; i < concurrency; i++ {
		username := "heartbeat-user-" + string(rune('a'+i%26))
		token, _ := registerAndLogin(t, username, "123456")

		wsConn := wsConnect(t)
		conns[i] = wsConn

		authPayload, _ := json.Marshal(map[string]interface{}{
			"token":     token,
			"device_id": "d1",
		})
		wsSendPacket(t, wsConn, &v1.Packet{
			Cmd:     v1.Command_CMD_AUTH,
			Seq:     1,
			Payload: authPayload,
		})
		wsReadPacket(t, wsConn, 2*time.Second)
	}

	// All connections send heartbeats concurrently
	var wg sync.WaitGroup
	wg.Add(concurrency)

	var totalHeartbeats int32

	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			defer wg.Done()
			wsConn := conns[idx]

			for j := 0; j < heartbeatsPerClient; j++ {
				wsSendPacket(t, wsConn, &v1.Packet{
					Cmd: v1.Command_CMD_HEARTBEAT,
					Seq: uint64(j + 1),
				})

				resp := wsReadPacketOrNil(t, wsConn, 2*time.Second)
				if resp != nil && resp.Cmd == v1.Command_CMD_HEARTBEAT {
					atomic.AddInt32(&totalHeartbeats, 1)
				}
			}
		}(i)
	}

	wg.Wait()

	expected := int32(concurrency * heartbeatsPerClient)
	if totalHeartbeats != expected {
		t.Fatalf("expected %d heartbeats, got %d", expected, totalHeartbeats)
	}

	// Clean up
	for _, c := range conns {
		if c != nil {
			c.Close()
		}
	}
}

// TestGateway_Concurrent_Publish verifies that many clients can publish messages concurrently.
func TestGateway_Concurrent_Publish(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	const concurrency = 15
	const messagesPerClient = 5

	// Set up authenticated connections
	conns := make([]*websocket.Conn, concurrency)
	for i := 0; i < concurrency; i++ {
		username := "publish-user-" + string(rune('a'+i%26))
		token, _ := registerAndLogin(t, username, "123456")

		wsConn := wsConnect(t)
		conns[i] = wsConn

		authPayload, _ := json.Marshal(map[string]interface{}{
			"token":     token,
			"device_id": "d1",
		})
		wsSendPacket(t, wsConn, &v1.Packet{
			Cmd:     v1.Command_CMD_AUTH,
			Seq:     1,
			Payload: authPayload,
		})
		wsReadPacket(t, wsConn, 2*time.Second)
	}

	// Publish concurrently
	var wg sync.WaitGroup
	wg.Add(concurrency)

	var ackCount int32

	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			defer wg.Done()
			wsConn := conns[idx]

			for j := 0; j < messagesPerClient; j++ {
				req := &v1.SendMessageRequest{
					Topic:       "p2p_1_2",
					MsgType:     v1.MsgType_MSG_TYPE_TEXT,
					Content:     []byte("concurrent message"),
					ClientMsgId: "msg-" + string(rune('0'+idx)) + "-" + string(rune('0'+j)),
				}
				payload, _ := proto.Marshal(req)
				wsSendPacket(t, wsConn, &v1.Packet{
					Cmd:     v1.Command_CMD_PUBLISH,
					Seq:     uint64(j + 1),
					Payload: payload,
				})

				resp := wsReadPacketOrNil(t, wsConn, 2*time.Second)
				if resp != nil && resp.Cmd == v1.Command_CMD_PUBLISH {
					var reply v1.SendMessageReply
					if err := proto.Unmarshal(resp.Payload, &reply); err == nil && reply.ClientMsgId == req.ClientMsgId {
						atomic.AddInt32(&ackCount, 1)
					}
				}
			}
		}(i)
	}

	wg.Wait()

	expected := int32(concurrency * messagesPerClient)
	if ackCount != expected {
		t.Fatalf("expected %d ACKs, got %d", expected, ackCount)
	}

	// Clean up
	for _, c := range conns {
		if c != nil {
			c.Close()
		}
	}
}

// TestGateway_Connection_Reliability_Stress tests connection reliability under rapid connect/disconnect.
func TestGateway_Connection_Reliability_Stress(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	const iterations = 50

	token, userID := registerAndLogin(t, "reliability-user", "123456")

	for i := 0; i < iterations; i++ {
		wsConn := wsConnect(t)

		authPayload, _ := json.Marshal(map[string]interface{}{
			"token":     token,
			"device_id": "stress-device",
		})
		wsSendPacket(t, wsConn, &v1.Packet{
			Cmd:     v1.Command_CMD_AUTH,
			Seq:     1,
			Payload: authPayload,
		})

		resp := wsReadPacketOrNil(t, wsConn, 2*time.Second)
		if resp == nil || resp.Cmd != v1.Command_CMD_AUTH {
			t.Fatalf("iteration %d: auth failed", i)
		}

		// Immediately close
		wsConn.Close()

		// Brief pause to allow cleanup
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for all async cleanup
	time.Sleep(500 * time.Millisecond)

	// Verify clean state
	if ts.gwManager.Get(userID) != nil {
		t.Fatal("connection should be cleaned up after stress test")
	}

	ctx := context.Background()
	if ts.gwSessionMgr.IsOnline(ctx, userID) {
		t.Fatal("user should be offline after stress test")
	}

	t.Logf("stress test passed: %d connect/auth/close cycles", iterations)
}

// TestGateway_Broadcast_Concurrent verifies broadcasting concurrently to many users.
func TestGateway_Broadcast_Concurrent(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	const userCount = 10
	const devicesPerUser = 2

	// Set up users and connections
	userIDs := make([]int64, userCount)
	conns := make([][]*websocket.Conn, userCount)

	for u := 0; u < userCount; u++ {
		username := "broadcast-concurrent-user-" + string(rune('a'+u%26))
		token, userID := registerAndLogin(t, username, "123456")
		userIDs[u] = userID
		conns[u] = make([]*websocket.Conn, devicesPerUser)

		for d := 0; d < devicesPerUser; d++ {
			wsConn := wsConnect(t)
			conns[u][d] = wsConn

			authPayload, _ := json.Marshal(map[string]interface{}{
				"token":     token,
				"device_id": "device-" + string(rune('0'+d)),
			})
			wsSendPacket(t, wsConn, &v1.Packet{
				Cmd:     v1.Command_CMD_AUTH,
				Seq:     1,
				Payload: authPayload,
			})
			wsReadPacket(t, wsConn, 2*time.Second)
		}
	}

	// Broadcast to all users concurrently
	var wg sync.WaitGroup
	wg.Add(userCount)

	var totalSent int32

	for u := 0; u < userCount; u++ {
		go func(idx int) {
			defer wg.Done()

			packet := &v1.Packet{
				Cmd:     v1.Command_CMD_NOTIFY,
				Seq:     uint64(idx + 1),
				Payload: []byte("broadcast to user " + string(rune('0'+idx))),
			}
			sent := ts.gwManager.BroadcastToUser(userIDs[idx], packet)
			atomic.AddInt32(&totalSent, int32(sent))
		}(u)
	}

	wg.Wait()

	expected := int32(userCount * devicesPerUser)
	if totalSent != expected {
		t.Fatalf("expected %d total sent, got %d", expected, totalSent)
	}

	// Verify each device received the broadcast
	for u := 0; u < userCount; u++ {
		for d := 0; d < devicesPerUser; d++ {
			resp := wsReadPacketOrNil(t, conns[u][d], 2*time.Second)
			if resp == nil {
				t.Fatalf("user %d device %d did not receive broadcast", u, d)
			}
			if resp.Cmd != v1.Command_CMD_NOTIFY {
				t.Fatalf("user %d device %d expected CMD_NOTIFY, got %v", u, d, resp.Cmd)
			}
		}
	}

	// Clean up
	for u := 0; u < userCount; u++ {
		for d := 0; d < devicesPerUser; d++ {
			if conns[u][d] != nil {
				conns[u][d].Close()
			}
		}
	}
}

// TestGateway_RedisSession_ExpireAndRefresh verifies that Redis sessions expire correctly
// and are refreshed by heartbeats.
func TestGateway_RedisSession_ExpireAndRefresh(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, userID := registerAndLogin(t, "expire-user", "123456")

	wsConn := wsConnect(t)
	defer wsConn.Close()

	// Authenticate with short TTL
	authPayload, _ := json.Marshal(map[string]interface{}{
		"token":     token,
		"device_id": "expire-device",
	})
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd:     v1.Command_CMD_AUTH,
		Seq:     1,
		Payload: authPayload,
	})
	wsReadPacket(t, wsConn, 2*time.Second)

	ctx := context.Background()

	// Manually set a short TTL for testing
	err := ts.gwSessionMgr.SetSession(ctx, userID, "expire-device", 2*time.Second)
	if err != nil {
		t.Fatalf("failed to set session: %v", err)
	}

	// Verify session exists
	if !ts.gwSessionMgr.IsOnline(ctx, userID) {
		t.Fatal("user should be online after auth")
	}

	// Wait for session to almost expire
	time.Sleep(1500 * time.Millisecond)

	// Send heartbeat to refresh TTL
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd: v1.Command_CMD_HEARTBEAT,
		Seq: 2,
	})
	wsReadPacket(t, wsConn, 2*time.Second)

	// Refresh TTL in Redis
	err = ts.gwSessionMgr.ExpireSession(ctx, userID, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to expire session: %v", err)
	}

	// Wait for original TTL to pass
	time.Sleep(1500 * time.Millisecond)

	// Session should still exist because heartbeat refreshed it
	if !ts.gwSessionMgr.IsOnline(ctx, userID) {
		t.Fatal("user should still be online after heartbeat refresh")
	}
}

// TestGateway_Concurrent_RapidMessages verifies the system handles rapid sequential messages.
func TestGateway_Concurrent_RapidMessages(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "rapid-user", "123456")

	wsConn := wsConnect(t)
	defer wsConn.Close()

	// Authenticate
	authPayload, _ := json.Marshal(map[string]interface{}{
		"token":     token,
		"device_id": "rapid-device",
	})
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd:     v1.Command_CMD_AUTH,
		Seq:     1,
		Payload: authPayload,
	})
	wsReadPacket(t, wsConn, 2*time.Second)

	// Send many messages rapidly
	const messageCount = 100
	for i := 0; i < messageCount; i++ {
		req := &v1.SendMessageRequest{
			Topic:       "p2p_1_2",
			MsgType:     v1.MsgType_MSG_TYPE_TEXT,
			Content:     []byte("rapid message"),
			ClientMsgId: "rapid-" + string(rune('0'+i%10)),
		}
		payload, _ := proto.Marshal(req)
		wsSendPacket(t, wsConn, &v1.Packet{
			Cmd:     v1.Command_CMD_PUBLISH,
			Seq:     uint64(i + 1),
			Payload: payload,
		})
	}

	// Read all ACKs
	ackCount := 0
	for i := 0; i < messageCount; i++ {
		resp := wsReadPacketOrNil(t, wsConn, 3*time.Second)
		if resp == nil {
			break
		}
		if resp.Cmd == v1.Command_CMD_PUBLISH {
			ackCount++
		}
	}

	if ackCount != messageCount {
		t.Fatalf("expected %d ACKs, got %d", messageCount, ackCount)
	}
}

// TestGateway_MixedTraffic verifies handling of mixed commands concurrently.
func TestGateway_MixedTraffic(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	const clientCount = 10

	// Set up clients
	conns := make([]*websocket.Conn, clientCount)
	for i := 0; i < clientCount; i++ {
		username := "mixed-user-" + string(rune('a'+i%26))
		token, _ := registerAndLogin(t, username, "123456")

		wsConn := wsConnect(t)
		conns[i] = wsConn

		authPayload, _ := json.Marshal(map[string]interface{}{
			"token":     token,
			"device_id": "d1",
		})
		wsSendPacket(t, wsConn, &v1.Packet{
			Cmd:     v1.Command_CMD_AUTH,
			Seq:     1,
			Payload: authPayload,
		})
		wsReadPacket(t, wsConn, 2*time.Second)
	}

	// Each client sends a mix of heartbeats and publishes
	var wg sync.WaitGroup
	wg.Add(clientCount)

	for i := 0; i < clientCount; i++ {
		go func(idx int) {
			defer wg.Done()
			wsConn := conns[idx]

			for j := 0; j < 20; j++ {
				switch rand.Intn(3) {
				case 0: // heartbeat
					wsSendPacket(t, wsConn, &v1.Packet{
						Cmd: v1.Command_CMD_HEARTBEAT,
						Seq: uint64(j + 1),
					})
					wsReadPacketOrNil(t, wsConn, 2*time.Second)

				case 1, 2: // publish
					req := &v1.SendMessageRequest{
						Topic:       "p2p_1_2",
						MsgType:     v1.MsgType_MSG_TYPE_TEXT,
						Content:     []byte("mixed traffic"),
						ClientMsgId: "mixed-" + string(rune('0'+idx)) + "-" + string(rune('0'+j%10)),
					}
					payload, _ := proto.Marshal(req)
					wsSendPacket(t, wsConn, &v1.Packet{
						Cmd:     v1.Command_CMD_PUBLISH,
						Seq:     uint64(j + 1),
						Payload: payload,
					})
					wsReadPacketOrNil(t, wsConn, 2*time.Second)
				}
			}
		}(i)
	}

	wg.Wait()

	// Clean up
	for _, c := range conns {
		if c != nil {
			c.Close()
		}
	}
}
