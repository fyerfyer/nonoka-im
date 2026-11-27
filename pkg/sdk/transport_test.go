package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

type rotatingTokenSource struct {
	calls atomic.Int32
}

func (s *rotatingTokenSource) Token(context.Context) (string, error) { return "old", nil }
func (s *rotatingTokenSource) Refresh(context.Context) (string, error) {
	s.calls.Add(1)
	return "new", nil
}

func TestAPITransportRefreshesTokenForGET(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "Bearer new" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"UNAUTHORIZED","message":"expired"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"ok": "yes"})
	}))
	defer server.Close()

	source := &rotatingTokenSource{}
	transport := newAPITransport(server.URL, server.Client(), source, "")
	var out map[string]string
	if err := transport.doJSON(context.Background(), http.MethodGet, "/v1/test", nil, nil, &out, true); err != nil {
		t.Fatalf("doJSON: %v", err)
	}
	if out["ok"] != "yes" {
		t.Fatalf("unexpected response: %v", out)
	}
	if source.calls.Load() != 1 {
		t.Fatalf("refresh calls = %d, want 1", source.calls.Load())
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
}
