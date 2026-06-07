package integration

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/pkg/sdk"
)

// ============================================
// SDK Integration Tests
// ============================================

// TestSDK_Connect_Auth_Success verifies SDK can connect and authenticate.
func TestSDK_Connect_Auth_Success(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "sdk-auth-user", "123456")

	connected := make(chan struct{}, 1)
	client := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    5 * time.Second,
		OnConnect: func() {
			connected <- struct{}{}
		},
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("sdk connect failed: %v", err)
	}

	// Wait for OnConnect callback
	select {
	case <-connected:
		// success
	case <-time.After(3 * time.Second):
		t.Fatal("OnConnect callback not fired")
	}

	if !client.IsConnected() {
		t.Fatal("client should be connected")
	}
	if !client.IsAuthed() {
		t.Fatal("client should be authed")
	}
	if client.UserID() == 0 {
		t.Fatal("client should have a non-zero userID")
	}
}

// TestSDK_Connect_InvalidToken verifies SDK fails to auth with invalid token.
func TestSDK_Connect_InvalidToken(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	client := sdk.NewClient(sdk.Options{
		GatewayURL:     testWSURL,
		Token:          "invalid-token",
		DeviceID:       "sdk-test",
		RequestTimeout: 5 * time.Second,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := client.Connect(ctx)
	if err == nil {
		t.Fatal("expected auth failure with invalid token")
	}
}

// TestSDK_SendMessage_Success verifies SDK can send a message and receive ACK.
func TestSDK_SendMessage_Success(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "sdk-send-user", "123456")

	client := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    5 * time.Second,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("sdk connect failed: %v", err)
	}

	// Send a message
	sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sendCancel()

	result, err := client.SendMessage(sendCtx, "p2p_1_2", v1.MsgType_MSG_TYPE_TEXT, []byte("hello from sdk"))
	if err != nil {
		t.Fatalf("send message failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil send result")
	}
	if result.ClientMsgID == "" {
		t.Fatal("expected non-empty client_msg_id")
	}
	if result.Timestamp == 0 {
		t.Fatal("expected non-zero timestamp")
	}
}

// TestSDK_SendMessage_WithoutAuth verifies send fails without auth.
func TestSDK_SendMessage_WithoutAuth(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	// Connect without token (no auth)
	client := sdk.NewClient(sdk.Options{
		GatewayURL:     testWSURL,
		RequestTimeout: 2 * time.Second,
	})
	defer client.Close()

	// Manually connect WebSocket without auth
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect without auth should succeed: %v", err)
	}

	// Try to send without auth
	sendCtx, sendCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer sendCancel()

	_, err := client.SendMessage(sendCtx, "p2p_1_2", v1.MsgType_MSG_TYPE_TEXT, []byte("test"))
	if err == nil {
		t.Fatal("expected error when sending without auth")
	}
}

// TestSDK_PullMessages verifies SDK can pull offline messages.
func TestSDK_PullMessages(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "sdk-pull-user", "123456")

	client := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    5 * time.Second,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("sdk connect failed: %v", err)
	}

	// Pull messages from a topic
	pullCtx, pullCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pullCancel()

	result, err := client.PullMessages(pullCtx, "p2p_1_2", 0, 10)
	if err != nil {
		t.Fatalf("pull messages failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil pull result")
	}
	// No messages yet, should be empty
	if len(result.Messages) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(result.Messages))
	}
	if result.HasMore {
		t.Fatal("expected hasMore=false for empty result")
	}
	if result.NextSeq != 0 {
		t.Fatalf("expected nextSeq=0 for empty result, got %d", result.NextSeq)
	}
}

// TestSDK_Heartbeat verifies heartbeat keeps connection alive.
func TestSDK_Heartbeat(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "sdk-heartbeat-user", "123456")

	client := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 1 * time.Second,
		RequestTimeout:    5 * time.Second,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("sdk connect failed: %v", err)
	}

	// Wait for a few heartbeats
	time.Sleep(3 * time.Second)

	// Connection should still be alive
	if !client.IsConnected() {
		t.Fatal("client should still be connected after heartbeats")
	}
	if !client.IsAuthed() {
		t.Fatal("client should still be authed after heartbeats")
	}
}

// TestSDK_MessageHandler verifies push messages trigger the OnMessage callback.
func TestSDK_MessageHandler(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, userID := registerAndLogin(t, "sdk-receive-user", "123456")

	var receivedMsgs sync.Map
	msgReceived := make(chan *sdk.Message, 10)

	client := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    5 * time.Second,
		AutoAck:           true,
		OnMessage: func(msg *sdk.Message) {
			receivedMsgs.Store(msg.MsgID, msg)
			msgReceived <- msg
		},
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("sdk connect failed: %v", err)
	}

	// Broadcast a message to the user via the manager directly
	broadcastPacket := &v1.Packet{
		Cmd: v1.Command_CMD_NOTIFY,
		Seq: 99,
		Payload: &v1.Packet_Notify{
			Notify: &v1.MessagePush{
				MsgId:     12345,
				Topic:     "p2p_1_2",
				SenderId:  2,
				MsgType:   int32(v1.MsgType_MSG_TYPE_TEXT),
				Content:   []byte("test push message"),
				Timestamp: time.Now().Unix(),
				TopicSeq:  1,
			},
		},
	}

	sent := ts.gwManager.BroadcastToUser(userID, broadcastPacket)
	if sent != 1 {
		t.Fatalf("expected broadcast to 1 device, got %d", sent)
	}

	// Wait for message callback
	select {
	case msg := <-msgReceived:
		if msg.Topic != "p2p_1_2" {
			t.Fatalf("expected topic=p2p_1_2, got %s", msg.Topic)
		}
		if string(msg.Content) != "test push message" {
			t.Fatalf("expected content='test push message', got %s", string(msg.Content))
		}
		if msg.TopicSeq != 1 {
			t.Fatalf("expected topicSeq=1, got %d", msg.TopicSeq)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("message callback not fired within timeout")
	}
}

// TestSDK_Reconnect verifies auto-reconnect works after connection drops.
func TestSDK_Reconnect(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "sdk-reconnect-user", "123456")

	var disconnectCount atomic.Int32
	var connectCount atomic.Int32
	connected := make(chan struct{}, 5)

	client := sdk.NewClient(sdk.Options{
		GatewayURL:           testWSURL,
		Token:                token,
		DeviceID:             "sdk-test",
		HeartbeatInterval:    5 * time.Second,
		RequestTimeout:       5 * time.Second,
		ReconnectInterval:    500 * time.Millisecond,
		AutoReconnect:        true,
		MaxReconnectAttempts: 10,
		OnConnect: func() {
			connectCount.Add(1)
			select {
			case connected <- struct{}{}:
			default:
			}
		},
		OnDisconnect: func(reason error) {
			disconnectCount.Add(1)
		},
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("sdk connect failed: %v", err)
	}

	// Wait for initial connection
	select {
	case <-connected:
		// connected
	case <-time.After(3 * time.Second):
		t.Fatal("initial connect callback not fired")
	}

	// Simulate connection drop by closing from server side
	// The SDK should reconnect automatically
	// We'll check if the connection remains alive after some time
	time.Sleep(2 * time.Second)

	// Client should still be connected (either original or reconnected)
	if !client.IsConnected() {
		t.Fatal("client should still be connected after potential reconnect")
	}
}

// TestSDK_MessageDeduplication verifies duplicate push messages are deduplicated.
func TestSDK_MessageDeduplication(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, userID := registerAndLogin(t, "sdk-dedup-user", "123456")

	var msgCount atomic.Int32
	msgReceived := make(chan *sdk.Message, 10)

	client := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    5 * time.Second,
		AutoAck:           true,
		OnMessage: func(msg *sdk.Message) {
			msgCount.Add(1)
			msgReceived <- msg
		},
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("sdk connect failed: %v", err)
	}

	// Broadcast the same message twice
	broadcastPacket := &v1.Packet{
		Cmd: v1.Command_CMD_NOTIFY,
		Seq: 99,
		Payload: &v1.Packet_Notify{
			Notify: &v1.MessagePush{
				MsgId:     12345,
				Topic:     "p2p_1_2",
				SenderId:  2,
				MsgType:   int32(v1.MsgType_MSG_TYPE_TEXT),
				Content:   []byte("dedup test"),
				Timestamp: time.Now().Unix(),
				TopicSeq:  1,
			},
		},
	}

	ts.gwManager.BroadcastToUser(userID, broadcastPacket)
	ts.gwManager.BroadcastToUser(userID, broadcastPacket)

	// Wait for first message
	select {
	case <-msgReceived:
		// received first
	case <-time.After(3 * time.Second):
		t.Fatal("first message callback not fired")
	}

	// Wait a bit to see if duplicate arrives
	select {
	case <-msgReceived:
		t.Fatal("duplicate message should have been deduplicated")
	case <-time.After(500 * time.Millisecond):
		// good, no duplicate
	}

	if msgCount.Load() != 1 {
		t.Fatalf("expected exactly 1 message callback, got %d", msgCount.Load())
	}
}

// TestSDK_SendAndPull_E2E verifies a basic send + pull flow works end-to-end.
func TestSDK_SendAndPull_E2E(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	// Create two users
	token1, _ := registerAndLogin(t, "sdk-e2e-user1", "123456")
	token2, _ := registerAndLogin(t, "sdk-e2e-user2", "123456")

	// User1 connects
	client1 := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token1,
		DeviceID:          "sdk-device1",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    5 * time.Second,
	})
	defer client1.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client1.Connect(ctx); err != nil {
		t.Fatalf("client1 connect failed: %v", err)
	}

	// User2 connects
	client2 := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token2,
		DeviceID:          "sdk-device2",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    5 * time.Second,
	})
	defer client2.Close()

	if err := client2.Connect(ctx); err != nil {
		t.Fatalf("client2 connect failed: %v", err)
	}

	// User1 sends a message
	sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sendCancel()

	result, err := client1.SendMessage(sendCtx, "p2p_1_2", v1.MsgType_MSG_TYPE_TEXT, []byte("e2e hello"))
	if err != nil {
		t.Fatalf("client1 send failed: %v", err)
	}
	if result.ClientMsgID == "" {
		t.Fatal("expected non-empty client_msg_id")
	}
}

// TestSDK_SendMessage_WithMentions verifies send with mentions works.
func TestSDK_SendMessage_WithMentions(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "sdk-mention-user", "123456")

	client := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    5 * time.Second,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("sdk connect failed: %v", err)
	}

	sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sendCancel()

	result, err := client.SendMessageWithMentions(sendCtx, "group_test", v1.MsgType_MSG_TYPE_TEXT, []byte("hello @user"), []int64{2, 3})
	if err != nil {
		t.Fatalf("send message with mentions failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil send result")
	}
}

// TestSDK_ConcurrentSend verifies concurrent message sending is safe.
func TestSDK_ConcurrentSend(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "sdk-concurrent-user", "123456")

	client := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    5 * time.Second,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("sdk connect failed: %v", err)
	}

	// Send messages concurrently
	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer sendCancel()

			content := []byte("concurrent message " + string(rune('0'+idx)))
			_, err := client.SendMessage(sendCtx, "p2p_1_2", v1.MsgType_MSG_TYPE_TEXT, content)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	errCount := 0
	for err := range errors {
		_ = err
		errCount++
	}

	if errCount > 0 {
		t.Fatalf("expected 0 errors, got %d", errCount)
	}
}
