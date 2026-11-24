package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestMessagesSendReusesClientMessageID(t *testing.T) {
	var calls atomic.Int32
	var ids []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/message/send" {
			http.NotFound(w, r)
			return
		}
		var body struct {
			ClientMsgID string `json:"clientMsgId"`
			Topic       string `json:"topic"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		ids = append(ids, body.ClientMsgID)
		if calls.Add(1) == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"code":503,"reason":"UNAVAILABLE","message":"try again"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"clientMsgId":"` + body.ClientMsgID + `","msgId":"42","timestamp":"1700000000","topicSeq":"7"}`))
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL, TokenSource: StaticToken("token")})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	result, err := client.Messages.SendText(context.Background(), "p2p_1_2", "hello")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if result.MessageID != 42 || result.TopicSeq != 7 || result.ClientMessageID == "" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(ids) != 2 || ids[0] != ids[1] {
		t.Fatalf("client message ID was not reused: %v", ids)
	}
}

func TestAgentCRUDAndTypedErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"UNAUTHORIZED","message":"missing token","requestId":"req-1"}`))
			return
		}
		switch r.URL.Path {
		case "/v1/agents":
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`{"items":[{"id":1,"botUserId":9,"ownerUserId":1,"name":"A","description":"d","status":"active","currentVersion":1,"canManage":true,"canOwn":true,"draftChanged":false}],"hasMore":false}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":2,"botUserId":10,"ownerUserId":1,"name":"New","description":"","status":"draft","currentVersion":0,"canManage":true,"canOwn":true,"draftChanged":true,"draft":{"instruction":"hi","modelServiceId":1,"modelId":2,"tools":[],"capabilityPresets":[],"toolPolicies":{},"maxTokens":4096,"mcpBindings":[],"skills":[],"revision":1}}`))
		case "/v1/agents/1":
			_, _ = w.Write([]byte(`{"id":1,"botUserId":9,"ownerUserId":1,"name":"A","description":"d","status":"active","currentVersion":1,"canManage":true,"canOwn":true,"draftChanged":false}`))
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":"NOT_FOUND","message":"missing"}`))
		}
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL, TokenSource: StaticToken("token")})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	page, err := client.Agents.List(context.Background(), ListOptions{})
	if err != nil || len(page.Items) != 1 || page.Items[0].Name != "A" {
		t.Fatalf("list: %+v err=%v", page, err)
	}
	created, err := client.Agents.Create(context.Background(), AgentInput{Name: "New"})
	if err != nil || created.ID != 2 || created.Draft == nil {
		t.Fatalf("create: %+v err=%v", created, err)
	}
	_, err = client.Agents.Get(context.Background(), 999)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !IsNotFound(err) || apiErr.Code != "NOT_FOUND" {
		t.Fatalf("expected typed not-found error, got %v", err)
	}

	unauthorized, err := New(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer unauthorized.Close()
	_, err = unauthorized.Agents.Get(context.Background(), 1)
	if !IsUnauthorized(err) {
		t.Fatalf("expected unauthorized, got %v", err)
	}
}

func TestPasswordTokenSourceLoginAndCache(t *testing.T) {
	var logins atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/login":
			logins.Add(1)
			_, _ = w.Write([]byte(`{"userId":"7","token":"access","refreshToken":"refresh"}`))
		case "/v1/auth/refresh":
			_, _ = w.Write([]byte(`{"userId":"7","token":"access-2","refreshToken":"refresh-2"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	source := NewPasswordTokenSource(server.URL, "alice", "secret", "device")
	first, err := source.Token(context.Background())
	if err != nil || first != "access" {
		t.Fatalf("first token: %q err=%v", first, err)
	}
	second, err := source.Token(context.Background())
	if err != nil || second != "access" {
		t.Fatalf("cached token: %q err=%v", second, err)
	}
	if logins.Load() != 1 {
		t.Fatalf("expected one login, got %d", logins.Load())
	}
	refreshed, err := source.Refresh(context.Background())
	if err != nil || refreshed != "access-2" {
		t.Fatalf("refresh token: %q err=%v", refreshed, err)
	}
}
