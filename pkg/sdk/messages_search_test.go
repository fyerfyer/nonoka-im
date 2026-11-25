package sdk

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMessagesSearchTypedResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/message/search" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("topic"); got != "p2p_1_2" {
			t.Fatalf("topic: %q", got)
		}
		if got := r.URL.Query().Get("query"); got != "incident" {
			t.Fatalf("query: %q", got)
		}
		if got := r.URL.Query().Get("hasFile"); got != "true" {
			t.Fatalf("hasFile: %q", got)
		}
		_, _ = w.Write([]byte(`{"messages":[{"msgId":"42","topic":"p2p_1_2","topicSeq":"7","senderId":"3","timestamp":"1700000000","msgType":1,"hasFile":true,"snippet":"incident notes"}],"hasMore":false}`))
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL, TokenSource: StaticToken("token")})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	result, err := client.Messages.Search(context.Background(), SearchRequest{
		Topic: "p2p_1_2", Query: "incident", HasFile: true,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Hits) != 1 || result.Hits[0].MessageID != 42 || result.Hits[0].TopicSeq != 7 ||
		!result.Hits[0].HasFile || result.Hits[0].Snippet != "incident notes" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestMessagesSearchUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"code":503,"reason":"SEARCH_UNAVAILABLE","message":"search is not configured"}`))
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL, TokenSource: StaticToken("token")})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_, err = client.Messages.Search(context.Background(), SearchRequest{Topic: "p2p_1_2", Query: "x"})
	if !errors.Is(err, ErrSearchUnavailable) {
		t.Fatalf("expected ErrSearchUnavailable, got %v", err)
	}
}
