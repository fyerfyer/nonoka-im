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
		RequestTimeout:    15 * time.Second,
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

	select {
	case <-connected:
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

func TestSDK_Connect_InvalidToken(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	client := sdk.NewClient(sdk.Options{
		GatewayURL:     testWSURL,
		Token:          "invalid-token",
		DeviceID:       "sdk-test",
		RequestTimeout: 15 * time.Second,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := client.Connect(ctx)
	if err == nil {
		t.Fatal("expected auth failure with invalid token")
	}
}

func TestSDK_SendMessage_Success(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "sdk-send-user", "123456")

	client := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("sdk connect failed: %v", err)
	}

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

func TestSDK_SendMessage_WithoutAuth(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	client := sdk.NewClient(sdk.Options{
		GatewayURL:     testWSURL,
		RequestTimeout: 2 * time.Second,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect without auth should succeed: %v", err)
	}

	sendCtx, sendCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer sendCancel()

	_, err := client.SendMessage(sendCtx, "p2p_1_2", v1.MsgType_MSG_TYPE_TEXT, []byte("test"))
	if err == nil {
		t.Fatal("expected error when sending without auth")
	}
}

func TestSDK_PullMessages(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "sdk-pull-user", "123456")

	client := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("sdk connect failed: %v", err)
	}

	pullCtx, pullCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pullCancel()

	result, err := client.PullMessages(pullCtx, "p2p_1_2", 0, 10)
	if err != nil {
		t.Fatalf("pull messages failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil pull result")
	}
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

func TestSDK_Heartbeat(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "sdk-heartbeat-user", "123456")

	client := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 1 * time.Second,
		RequestTimeout:    15 * time.Second,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("sdk connect failed: %v", err)
	}

	time.Sleep(3 * time.Second)

	if !client.IsConnected() {
		t.Fatal("client should still be connected after heartbeats")
	}
	if !client.IsAuthed() {
		t.Fatal("client should still be authed after heartbeats")
	}
}

func TestSDK_MessageHandler(t *testing.T) {
	ts := setupTestServer(t, true)
	defer ts.stop()

	token1, userID1 := registerAndLogin(t, "sdk-msg-sender", "123456")
	token2, userID2 := registerAndLogin(t, "sdk-msg-receiver", "123456")
	topic := sdk.P2PTopic(userID1, userID2)

	msgReceived := make(chan *sdk.Message, 1)
	receiver := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token2,
		DeviceID:          "sdk-receiver",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second,
		AutoAck:           true,
		OnMessage: func(msg *sdk.Message) {
			select {
			case msgReceived <- msg:
			default:
			}
		},
	})
	defer receiver.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := receiver.Connect(ctx); err != nil {
		t.Fatalf("receiver connect failed: %v", err)
	}

	sender := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token1,
		DeviceID:          "sdk-sender",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second,
	})
	defer sender.Close()

	if err := sender.Connect(ctx); err != nil {
		t.Fatalf("sender connect failed: %v", err)
	}

	sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sendCancel()
	if _, err := sender.SendText(sendCtx, topic, "hello via sdk"); err != nil {
		t.Fatalf("sender send failed: %v", err)
	}

	select {
	case msg := <-msgReceived:
		if msg.Topic != topic {
			t.Fatalf("expected topic=%s, got %s", topic, msg.Topic)
		}
		if string(msg.Content) != "hello via sdk" {
			t.Fatalf("expected content='hello via sdk', got %s", string(msg.Content))
		}
		if msg.TopicSeq == 0 {
			t.Fatal("expected non-zero topicSeq")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("message callback not fired within timeout")
	}
}

func TestSDK_MessageDeduplication(t *testing.T) {
	ts := setupTestServer(t, true)
	defer ts.stop()

	token1, userID1 := registerAndLogin(t, "sdk-dedup-sender", "123456")
	token2, userID2 := registerAndLogin(t, "sdk-dedup-receiver", "123456")
	topic := sdk.P2PTopic(userID1, userID2)

	var msgCount atomic.Int32
	msgReceived := make(chan *sdk.Message, 2)

	receiver := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token2,
		DeviceID:          "sdk-receiver",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second,
		AutoAck:           true,
		OnMessage: func(msg *sdk.Message) {
			msgCount.Add(1)
			select {
			case msgReceived <- msg:
			default:
			}
		},
	})
	defer receiver.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := receiver.Connect(ctx); err != nil {
		t.Fatalf("receiver connect failed: %v", err)
	}

	sender := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token1,
		DeviceID:          "sdk-sender",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second,
	})
	defer sender.Close()

	if err := sender.Connect(ctx); err != nil {
		t.Fatalf("sender connect failed: %v", err)
	}

	clientMsgID := "dedup-sdk-001"
	sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sendCancel()

	if _, err := sender.Realtime.SendMessage(sendCtx, topic, v1.MsgType_MSG_TYPE_TEXT, []byte("dedup test"), clientMsgID); err != nil {
		t.Fatalf("first send failed: %v", err)
	}
	if _, err := sender.Realtime.SendMessage(sendCtx, topic, v1.MsgType_MSG_TYPE_TEXT, []byte("dedup test"), clientMsgID); err != nil {
		t.Fatalf("second send failed: %v", err)
	}

	select {
	case <-msgReceived:
	case <-time.After(15 * time.Second):
		t.Fatal("first message callback not fired")
	}

	select {
	case <-msgReceived:
		t.Fatal("duplicate message should have been deduplicated")
	case <-time.After(500 * time.Millisecond):
	}

	if msgCount.Load() != 1 {
		t.Fatalf("expected exactly 1 message callback, got %d", msgCount.Load())
	}
}

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

	select {
	case <-connected:
	case <-time.After(3 * time.Second):
		t.Fatal("initial connect callback not fired")
	}

	time.Sleep(2 * time.Second)

	if !client.IsConnected() {
		t.Fatal("client should still be connected after potential reconnect")
	}
}

func TestSDK_SendMessage_Ack(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token1, _ := registerAndLogin(t, "sdk-e2e-user1", "123456")
	token2, _ := registerAndLogin(t, "sdk-e2e-user2", "123456")

	client1 := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token1,
		DeviceID:          "sdk-device1",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second,
	})
	defer client1.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client1.Connect(ctx); err != nil {
		t.Fatalf("client1 connect failed: %v", err)
	}

	client2 := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token2,
		DeviceID:          "sdk-device2",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second,
	})
	defer client2.Close()

	if err := client2.Connect(ctx); err != nil {
		t.Fatalf("client2 connect failed: %v", err)
	}

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

func TestSDK_SendMessage_WithMentions(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "sdk-mention-user", "123456")

	client := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("sdk connect failed: %v", err)
	}

	sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sendCancel()

	result, err := client.SendMessageWithMentions(sendCtx, "grp_test", v1.MsgType_MSG_TYPE_TEXT, []byte("hello @user"), []int64{2, 3})
	if err != nil {
		t.Fatalf("send message with mentions failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil send result")
	}
}

func TestSDK_ConcurrentSend(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "sdk-concurrent-user", "123456")

	client := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("sdk connect failed: %v", err)
	}

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
