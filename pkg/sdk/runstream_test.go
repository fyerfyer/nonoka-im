package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRunStreamRecoversTerminalStateFromRunDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/agent-runs/7/events":
			_ = json.NewEncoder(w).Encode(map[string]any{"events": []any{}, "nextSeq": 0})
		case "/v1/agent-runs/7":
			_ = json.NewEncoder(w).Encode(AgentRun{
				ID: 7, AgentID: 3, ActorUserID: 11, Topic: "grp_1", Status: "completed",
				ReplyMsgID: 99, ReplyClientMsgID: "agent-7", InputTokens: 10, OutputTokens: 20,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &Client{http: newAPITransport(server.URL, server.Client(), StaticToken("token"), "")}
	client.Runs = &RunsResource{http: client.http, client: client}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	stream := client.Runs.Stream(ctx, 7)
	defer stream.Close()

	select {
	case event, ok := <-stream.Events():
		if !ok {
			t.Fatalf("stream closed without terminal event: %v", stream.Err())
		}
		if event.Type != RunEventCompleted {
			t.Fatalf("event type = %q, want completed", event.Type)
		}
		if event.ReplyMsgID != 99 || event.ClientMessageID != "agent-7" {
			t.Fatalf("terminal event missing run fields: %+v", event)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for terminal event")
	}
}
