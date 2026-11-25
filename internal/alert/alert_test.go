package alert

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWebhookPostsGenericJSON(t *testing.T) {
	var got Event
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode alert: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	webhook := NewWebhook(server.URL)
	if err := webhook.Notify(context.Background(), Event{
		Type: "agent_outbox", Pending: 2, Failed: 1, OldestAgeSeconds: 420, At: time.Unix(1, 0),
	}); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if got.Failed != 1 || got.Pending != 2 || got.OldestAgeSeconds != 420 {
		t.Fatalf("unexpected event: %+v", got)
	}
}
