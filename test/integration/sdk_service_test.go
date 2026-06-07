package integration

import (
	"context"
	"testing"
	"time"

	"nonoka-im/pkg/sdk"
)

// TestAuthServiceCanRegisterAndLogin verifies the HTTP auth service can register a new user and log them in.
func TestAuthServiceCanRegisterAndLogin(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	client := sdk.NewClient(sdk.Options{
		BaseURL: testBaseURL,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	regResult, err := client.Auth.Register(ctx, "svc-auth-user", "123456")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if regResult.UserID == 0 {
		t.Fatal("expected non-zero userID after register")
	}

	loginResult, err := client.Auth.Login(ctx, "svc-auth-user", "123456")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if loginResult.Token == "" {
		t.Fatal("expected non-empty token after login")
	}
	if loginResult.UserID == 0 {
		t.Fatal("expected non-zero userID after login")
	}
}

// TestDispatchServiceReturnsGatewayURL verifies the dispatch service resolves a gateway WebSocket URL.
func TestDispatchServiceReturnsGatewayURL(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	client := sdk.NewClient(sdk.Options{
		BaseURL: testBaseURL,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	url, err := client.Dispatch.GetGateway(ctx, 1)
	if err != nil {
		t.Fatalf("get gateway failed: %v", err)
	}
	if url == "" {
		t.Fatal("expected non-empty gateway URL")
	}
}

// TestMessageServiceCanSendViaHTTP verifies messages can be sent through the HTTP message service when WebSocket is unavailable.
func TestMessageServiceCanSendViaHTTP(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "http-msg-user", "123456")

	client := sdk.NewClient(sdk.Options{
		BaseURL:        testBaseURL,
		Token:          token,
		RequestTimeout: 5 * time.Second,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	// Close WebSocket to force HTTP fallback path
	if client.Realtime != nil {
		client.Realtime.Close()
	}

	sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sendCancel()

	result, err := client.SendMessage(sendCtx, "p2p_1_2", 1, []byte("http fallback test"))
	if err != nil {
		t.Fatalf("send message via HTTP fallback failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result from HTTP fallback")
	}
}
