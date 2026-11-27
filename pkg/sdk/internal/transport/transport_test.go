package transport

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

type refreshSource struct{ calls atomic.Int32 }

func (*refreshSource) Token(context.Context) (string, error) { return "old", nil }
func (s *refreshSource) Refresh(context.Context) (string, error) {
	s.calls.Add(1)
	return "new", nil
}

func TestClientDoRefreshesAndReplaysBody(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "Bearer new" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(r.Body)
		if buf.String() != "payload" {
			t.Errorf("body = %q", buf.String())
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	source := &refreshSource{}
	client := New(server.URL, server.Client(), source, "")
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+"/upload", bytes.NewBufferString("payload"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if source.calls.Load() != 1 || requests.Load() != 2 {
		t.Fatalf("refresh=%d requests=%d", source.calls.Load(), requests.Load())
	}
}
