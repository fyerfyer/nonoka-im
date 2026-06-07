package integration

import (
	"context"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/pkg/sdk"
)

// TestMessageStatusStringReturnsCorrectLabels verifies each MessageStatus maps to the right human-readable string.
func TestMessageStatusStringReturnsCorrectLabels(t *testing.T) {
	tests := []struct {
		status sdk.MessageStatus
		want   string
	}{
		{sdk.MessageStatusSending, "sending"},
		{sdk.MessageStatusSent, "sent"},
		{sdk.MessageStatusDelivered, "delivered"},
		{sdk.MessageStatusRead, "read"},
		{sdk.MessageStatusFailed, "failed"},
		{sdk.MessageStatus(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("MessageStatus(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}

// TestSendMessageProducesClientMsgID verifies every outbound message gets a unique client-side identifier.
func TestSendMessageProducesClientMsgID(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "state-send-user", "123456")

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

	result, err := client.SendMessage(sendCtx, "p2p_1_2", v1.MsgType_MSG_TYPE_TEXT, []byte("state test"))
	if err != nil {
		t.Fatalf("send message failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ClientMsgID == "" {
		t.Fatal("expected non-empty client_msg_id")
	}
	if result.MsgID == 0 {
		t.Fatal("expected non-zero msg_id")
	}
}

// TestSendWithoutAuthFailsGracefully verifies sending is rejected when the client is not authenticated.
func TestSendWithoutAuthFailsGracefully(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	client := sdk.NewClient(sdk.Options{
		BaseURL:           testBaseURL,
		GatewayURL:        testWSURL,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    2 * time.Second,
		AutoReconnect:     false,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Connect without auth (no token)
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect without auth should succeed for ws: %v", err)
	}

	sendCtx, sendCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer sendCancel()

	_, err := client.SendMessage(sendCtx, "p2p_1_2", v1.MsgType_MSG_TYPE_TEXT, []byte("should fail"))
	if err == nil {
		t.Fatal("expected error when sending without auth")
	}
}

// TestClientCanCloseIdempotently verifies Close can be invoked multiple times without panic.
func TestClientCanCloseIdempotently(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "close-idempotent-user", "123456")

	client := sdk.NewClient(sdk.Options{
		BaseURL:           testBaseURL,
		GatewayURL:        testWSURL,
		Token:             token,
		DeviceID:          "sdk-test",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    5 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	if err := client.Close(); err != nil {
		t.Fatalf("first close failed: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("second close failed: %v", err)
	}
}
