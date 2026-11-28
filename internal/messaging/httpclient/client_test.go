package httpclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestLookupMessageIDUsesBotReceiptEndpoint(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/bot/messages/by-client-id/client-1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("missing bot token")
		}
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"status":"pending"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"messageId": "42", "topic": "p2p_1_2", "client_msg_id": "client-1",
		})
	}))
	defer server.Close()

	client := &Client{BaseURL: server.URL, HTTP: server.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if got := client.lookupMessageID(ctx, "p2p_1_2", "client-1", "token"); got != 42 {
		t.Fatalf("message id = %d, want 42", got)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2", calls.Load())
	}
}
