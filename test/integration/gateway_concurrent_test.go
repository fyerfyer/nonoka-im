package integration

import (
	"context"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	v1 "nonoka-im/api/im/v1"
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

	for i := range concurrency {
		go func(idx int) {
			defer wg.Done()

			// Each goroutine registers and logs in a unique user
			username := "concurrent-auth-user-" + string(rune('a'+idx%26)) + "-" + string(rune('0'+idx/26))
			token, _ := registerAndLogin(t, username, "123456")

			wsConn := wsConnect(t)
			defer wsConn.Close()

			wsSendPacket(t, wsConn, &v1.Packet{
				Cmd: v1.Command_CMD_AUTH,
				Seq: uint64(idx + 1),
				Payload: &v1.Packet_AuthReq{
					AuthReq: &v1.AuthRequest{
						Token:    token,
						DeviceId: "device-" + string(rune('0'+idx%10)),
					},
				},
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

			authResp := resp.GetAuthResp()
			if authResp == nil {
				atomic.AddInt32(&failCount, 1)
				return
			}
			if authResp.Success {
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
	for i := range concurrency {
		go func(idx int) {
			defer wg.Done()

			wsConn := wsConnect(t)
			conns[idx] = wsConn

			wsSendPacket(t, wsConn, &v1.Packet{
				Cmd: v1.Command_CMD_AUTH,
				Seq: uint64(idx + 1),
				Payload: &v1.Packet_AuthReq{
					AuthReq: &v1.AuthRequest{
						Token:    token,
						DeviceId: "device-" + string(rune('a'+idx%26)) + "-" + string(rune('0'+idx/26)),
					},
				},
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
	for i := range concurrency {
		username := "heartbeat-user-" + string(rune('a'+i%26))
		token, _ := registerAndLogin(t, username, "123456")

		wsConn := wsConnect(t)
		conns[i] = wsConn

		wsSendPacket(t, wsConn, &v1.Packet{
			Cmd: v1.Command_CMD_AUTH,
			Seq: 1,
			Payload: &v1.Packet_AuthReq{
				AuthReq: &v1.AuthRequest{
					Token:    token,
					DeviceId: "d1",
				},
			},
		})
		wsReadPacket(t, wsConn, 2*time.Second)
	}

	// All connections send heartbeats concurrently
	var wg sync.WaitGroup
	wg.Add(concurrency)

	var totalHeartbeats int32

	for i := range concurrency {
		go func(idx int) {
			defer wg.Done()
			wsConn := conns[idx]

			for j := range heartbeatsPerClient {
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
	for i := range concurrency {
		username := "publish-user-" + string(rune('a'+i%26))
		token, _ := registerAndLogin(t, username, "123456")

		wsConn := wsConnect(t)
		conns[i] = wsConn

		wsSendPacket(t, wsConn, &v1.Packet{
			Cmd: v1.Command_CMD_AUTH,
			Seq: 1,
			Payload: &v1.Packet_AuthReq{
				AuthReq: &v1.AuthRequest{
					Token:    token,
					DeviceId: "d1",
				},
			},
		})
		wsReadPacket(t, wsConn, 2*time.Second)
	}

	// Publish concurrently
	var wg sync.WaitGroup
	wg.Add(concurrency)

	var ackCount int32

	for i := range concurrency {
		go func(idx int) {
			defer wg.Done()
			wsConn := conns[idx]

			for j := range messagesPerClient {
				wsSendPacket(t, wsConn, &v1.Packet{
					Cmd: v1.Command_CMD_PUBLISH,
					Seq: uint64(j + 1),
					Payload: &v1.Packet_SendReq{
						SendReq: &v1.SendMessageRequest{
							Topic:       "p2p_1_2",
							MsgType:     v1.MsgType_MSG_TYPE_TEXT,
							Content:     []byte("concurrent message"),
							ClientMsgId: "msg-" + string(rune('0'+idx)) + "-" + string(rune('0'+j)),
						},
					},
				})

				resp := wsReadPacketOrNil(t, wsConn, 2*time.Second)
				if resp != nil && resp.Cmd == v1.Command_CMD_PUBLISH {
					reply := resp.GetSendReply()
					if reply != nil && reply.ClientMsgId == "msg-"+string(rune('0'+idx))+"-"+string(rune('0'+j)) {
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

// TestGateway_Concurrent_RapidMessages verifies the system handles rapid sequential messages.
func TestGateway_Concurrent_RapidMessages(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "rapid-user", "123456")

	wsConn := wsConnect(t)
	defer wsConn.Close()

	// Authenticate
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd: v1.Command_CMD_AUTH,
		Seq: 1,
		Payload: &v1.Packet_AuthReq{
			AuthReq: &v1.AuthRequest{
				Token:    token,
				DeviceId: "rapid-device",
			},
		},
	})
	wsReadPacket(t, wsConn, 2*time.Second)

	// Send many messages rapidly
	const messageCount = 100
	for i := range messageCount {
		wsSendPacket(t, wsConn, &v1.Packet{
			Cmd: v1.Command_CMD_PUBLISH,
			Seq: uint64(i + 1),
			Payload: &v1.Packet_SendReq{
				SendReq: &v1.SendMessageRequest{
					Topic:       "p2p_1_2",
					MsgType:     v1.MsgType_MSG_TYPE_TEXT,
					Content:     []byte("rapid message"),
					ClientMsgId: "rapid-" + string(rune('0'+i%10)),
				},
			},
		})
	}

	// Read all ACKs
	ackCount := 0
	for range messageCount {
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
	for i := range clientCount {
		username := "mixed-user-" + string(rune('a'+i%26))
		token, _ := registerAndLogin(t, username, "123456")

		wsConn := wsConnect(t)
		conns[i] = wsConn

		wsSendPacket(t, wsConn, &v1.Packet{
			Cmd: v1.Command_CMD_AUTH,
			Seq: 1,
			Payload: &v1.Packet_AuthReq{
				AuthReq: &v1.AuthRequest{
					Token:    token,
					DeviceId: "d1",
				},
			},
		})
		wsReadPacket(t, wsConn, 2*time.Second)
	}

	// Each client sends a mix of heartbeats and publishes
	var wg sync.WaitGroup
	wg.Add(clientCount)

	for i := range clientCount {
		go func(idx int) {
			defer wg.Done()
			wsConn := conns[idx]

			for j := range 20 {
				switch rand.Intn(3) {
				case 0: // heartbeat
					wsSendPacket(t, wsConn, &v1.Packet{
						Cmd: v1.Command_CMD_HEARTBEAT,
						Seq: uint64(j + 1),
					})
					wsReadPacketOrNil(t, wsConn, 2*time.Second)

				case 1, 2: // publish
					wsSendPacket(t, wsConn, &v1.Packet{
						Cmd: v1.Command_CMD_PUBLISH,
						Seq: uint64(j + 1),
						Payload: &v1.Packet_SendReq{
							SendReq: &v1.SendMessageRequest{
								Topic:       "p2p_1_2",
								MsgType:     v1.MsgType_MSG_TYPE_TEXT,
								Content:     []byte("mixed traffic"),
								ClientMsgId: "mixed-" + string(rune('0'+idx)) + "-" + string(rune('0'+j%10)),
							},
						},
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
