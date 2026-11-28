package transport

import (
	"bytes"
	"context"
	"errors"
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

type flakyTransport struct {
	base http.RoundTripper
	n    atomic.Int32
}

func (f *flakyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if f.n.Add(1) == 1 {
		return nil, errors.New("transient connection failure")
	}
	return f.base.RoundTrip(req)
}

func TestDoJSONRetriesNetworkFailure(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	flaky := &flakyTransport{base: server.Client().Transport}
	client := New(server.URL, &http.Client{Transport: flaky}, nil, "")
	var out struct {
		OK bool `json:"ok"`
	}
	if err := client.DoJSON(context.Background(), http.MethodGet, "/resource", nil, nil, &out, true); err != nil {
		t.Fatalf("DoJSON: %v", err)
	}
	if !out.OK || flaky.n.Load() != 2 || requests.Load() != 1 {
		t.Fatalf("ok=%v attempts=%d requests=%d, want retry after network failure", out.OK, flaky.n.Load(), requests.Load())
	}
}
