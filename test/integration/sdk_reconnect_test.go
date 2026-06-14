package integration

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/pkg/sdk"
)

// TestAutoReconnectTriggersOnConnectCallback verifies the OnConnect callback fires after a successful reconnect.
func TestAutoReconnectTriggersOnConnectCallback(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "reconnect-callback-user", "123456")

	var connectCount atomic.Int32
	connected := make(chan struct{}, 2)

	client := sdk.NewClient(sdk.Options{
		BaseURL:           testBaseURL,
		GatewayURL:        testWSURL,
		Token:             token,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    5 * time.Second,
		ReconnectInterval: 200 * time.Millisecond,
		AutoReconnect:     true,
		MaxReconnectAttempts: 10,
		OnConnect: func() {
			connectCount.Add(1)
			select {
			case connected <- struct{}{}:
			default:
			}
		},
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	// Wait for initial OnConnect
	select {
	case <-connected:
		// initial connect
	case <-time.After(3 * time.Second):
		t.Fatal("initial OnConnect not fired")
	}

	// Disconnect realtime to simulate network drop without stopping background goroutines
	if client.Realtime != nil {
		client.Realtime.Disconnect()
	}

	// Wait for reconnect OnConnect
	select {
	case <-connected:
		// reconnected
	case <-time.After(5 * time.Second):
		t.Fatal("reconnect OnConnect not fired")
	}

	if connectCount.Load() < 2 {
		t.Fatalf("expected at least 2 connects, got %d", connectCount.Load())
	}
}

// TestSendMessageWithMentionsWorks verifies mentions can be attached to outgoing messages.
func TestSendMessageWithMentionsWorks(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "mention-user", "123456")

	client := sdk.NewClient(sdk.Options{
		BaseURL:           testBaseURL,
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
		t.Fatalf("connect failed: %v", err)
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
