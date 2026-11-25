package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	agentadapters "nonoka-im/internal/agent/adapters"
	agentapp "nonoka-im/internal/agent/app"
	agenttools "nonoka-im/internal/agent/tools"
	agentdata "nonoka-im/internal/data/agent"
	imapp "nonoka-im/internal/im/app"
	"nonoka-im/internal/im/domain"
	"nonoka-im/internal/im/search"
	messagingim "nonoka-im/internal/messaging/im"
)

// TestMessageSearch_IsolationAndFilters exercises the real Meilisearch index
// plus the shared QueryService used by both the HTTP API and the agent tool.
func TestMessageSearch_IsolationAndFilters(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	aliceToken, aliceID := registerAndLogin(t, "search-alice", "123456")
	_, bobID := registerAndLogin(t, "search-bob", "123456")
	eveToken, eveID := registerAndLogin(t, "search-eve", "123456")
	topic := fmt.Sprintf("p2p_%d_%d", aliceID, bobID)
	otherTopic := fmt.Sprintf("p2p_%d_%d", aliceID, eveID)

	ctx := context.Background()
	for _, pair := range []struct {
		userID int64
		topic  string
	}{
		{aliceID, topic}, {bobID, topic}, {aliceID, otherTopic}, {eveID, otherTopic},
	} {
		if err := ts.openFGA.EnsureTopicForSend(ctx, pair.userID, pair.topic); err != nil {
			t.Fatalf("grant topic %s to %d: %v", pair.topic, pair.userID, err)
		}
	}
	now := time.Now().Unix()
	persist := func(topicName, clientID, content string, msgType domain.MessageType, sentAt int64) int64 {
		res, err := ts.messageApp.HandleIncoming(ctx, domain.Message{
			Topic: topicName, SenderID: aliceID, Type: msgType,
			Content: []byte(content), ClientMsgID: clientID, Timestamp: sentAt,
		})
		if err != nil {
			t.Fatalf("persist %s: %v", clientID, err)
		}
		return res.Message.ID
	}

	incidentID := persist(topic, "search-1", "incident alpha deployment failed", domain.MessageTypeText, now-120)
	persist(topic, "search-2", "unrelated lunch plans", domain.MessageTypeText, now-60)
	persist(otherTopic, "search-3", "incident alpha belongs to the other conversation", domain.MessageTypeText, now-30)
	imageID := persist(topic, "search-4", `{"file_id":"file-search-1","name":"incident-design.png","mime":"image/png","size":12}`, domain.MessageTypeImage, now-10)

	relay := &search.Relay{Repo: ts.storage, Index: ts.searchIndex}
	if _, err := relay.Relay(ctx, 100); err != nil {
		t.Fatalf("index relay: %v", err)
	}
	waitForSearch(t, aliceToken, topic, "incident", 2)

	// Sender and time filters.
	resp := httpGet(t, fmt.Sprintf("%s/v1/message/search?topic=%s&query=incident&sender_id=%d&until=%d", testBaseURL, topic, aliceID, now-60), aliceToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("filtered search: %d", resp.StatusCode)
	}
	var reply struct {
		Messages []struct {
			MsgID    string `json:"msgId"`
			Topic    string `json:"topic"`
			TopicSeq string `json:"topicSeq"`
			SenderID string `json:"senderId"`
			HasFile  bool   `json:"hasFile"`
			Snippet  string `json:"snippet"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		t.Fatalf("decode filtered search: %v", err)
	}
	resp.Body.Close()
	if len(reply.Messages) != 1 || reply.Messages[0].MsgID != fmt.Sprint(incidentID) || reply.Messages[0].Topic != topic {
		t.Fatalf("unexpected filtered hits: %+v", reply.Messages)
	}

	// Attachment-only browse.
	fileResp := httpGet(t, fmt.Sprintf("%s/v1/message/search?topic=%s&has_file=true", testBaseURL, topic), aliceToken)
	if fileResp.StatusCode != http.StatusOK {
		t.Fatalf("file search: %d", fileResp.StatusCode)
	}
	var files struct {
		Messages []struct {
			MsgID   string `json:"msgId"`
			HasFile bool   `json:"hasFile"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(fileResp.Body).Decode(&files); err != nil {
		t.Fatalf("decode file search: %v", err)
	}
	fileResp.Body.Close()
	if len(files.Messages) != 1 || files.Messages[0].MsgID != fmt.Sprint(imageID) || !files.Messages[0].HasFile {
		t.Fatalf("unexpected file hits: %+v", files.Messages)
	}

	// A user outside the P2P topic must not search it.
	denied := httpGet(t, fmt.Sprintf("%s/v1/message/search?topic=%s&query=incident", testBaseURL, topic), eveToken)
	denied.Body.Close()
	if denied.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-topic search status: %d (eve=%d)", denied.StatusCode, eveID)
	}
}

// TestAgentSearchTool_RecordsCitations verifies that the built-in tool uses
// the same search service and records the consulted message refs on the run.
func TestAgentSearchTool_RecordsCitations(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	aliceToken, aliceID := registerAndLogin(t, "search-tool-alice", "123456")
	_, bobID := registerAndLogin(t, "search-tool-bob", "123456")
	topic := fmt.Sprintf("p2p_%d_%d", aliceID, bobID)
	ctx := context.Background()
	for _, userID := range []int64{aliceID, bobID} {
		if err := ts.openFGA.EnsureTopicForSend(ctx, userID, topic); err != nil {
			t.Fatalf("grant topic: %v", err)
		}
	}
	if _, err := ts.messageApp.HandleIncoming(ctx, domain.Message{
		Topic: topic, SenderID: bobID, Type: domain.MessageTypeText,
		Content: []byte("design review found a race condition"), ClientMsgID: "search-tool-1",
		Timestamp: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("persist message: %v", err)
	}
	relay := &search.Relay{Repo: ts.storage, Index: ts.searchIndex}
	if _, err := relay.Relay(ctx, 100); err != nil {
		t.Fatalf("index relay: %v", err)
	}
	waitForSearch(t, aliceToken, topic, "race", 1)

	store := agentdata.NewStore(ts.data.DB())
	toolsBackend := &agentapp.MessageTools{
		Reader:     agentadapters.Messages{Reader: messagingim.NewReader(ts.query)},
		Authorizer: ts.openFGA,
		Citations:  agentadapters.Citations{Store: store},
	}
	runID := uint64(time.Now().UnixNano())
	results, err := toolsBackend.SearchMessages(ctx, agenttools.RunContext{
		ActorUserID: aliceID, BotID: bobID, RunID: runID, Topic: topic,
	}, "race", 5, agenttools.SearchFilter{})
	if err != nil || len(results) != 1 {
		t.Fatalf("tool search: results=%v err=%v", results, err)
	}
	citations, err := store.ListRunCitations(ctx, runID)
	if err != nil || len(citations) != 1 {
		t.Fatalf("citations: %+v err=%v", citations, err)
	}
	if citations[0].Topic != topic || citations[0].Snippet == "" {
		t.Fatalf("unexpected citation: %+v", citations[0])
	}
}

// TestMessageSearch_MongoBackend verifies the zero-dependency fallback works
// against the existing content_text projection without Meilisearch.
func TestMessageSearch_MongoBackend(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	_, aliceID := registerAndLogin(t, "mongo-search-alice", "123456")
	_, bobID := registerAndLogin(t, "mongo-search-bob", "123456")
	_, eveID := registerAndLogin(t, "mongo-search-eve", "123456")
	topic := fmt.Sprintf("p2p_%d_%d", aliceID, bobID)
	ctx := context.Background()
	for _, pair := range []struct {
		userID int64
		topic  string
	}{{aliceID, topic}, {bobID, topic}} {
		if err := ts.openFGA.EnsureTopicForSend(ctx, pair.userID, pair.topic); err != nil {
			t.Fatalf("grant topic: %v", err)
		}
	}
	if _, err := ts.messageApp.HandleIncoming(ctx, domain.Message{
		Topic: topic, SenderID: bobID, Type: domain.MessageTypeText,
		Content: []byte("mongo fallback keyword"), ClientMsgID: "mongo-search-1", Timestamp: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("persist text: %v", err)
	}
	if _, err := ts.messageApp.HandleIncoming(ctx, domain.Message{
		Topic: topic, SenderID: bobID, Type: domain.MessageTypeImage,
		Content:     []byte(`{"file_id":"mongo-file","name":"diagram-mongo.png","mime":"image/png","size":1}`),
		ClientMsgID: "mongo-search-2", Timestamp: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("persist image: %v", err)
	}

	query := &imapp.QueryService{Reader: ts.storage, Search: search.NewMongoIndex(ts.mongoDB), Authorizer: ts.openFGA}
	hits, _, err := query.SearchMessages(ctx, aliceID, topic, "fallback", domain.SearchFilter{}, 20, 0)
	if err != nil || len(hits) != 1 || hits[0].Snippet == "" {
		t.Fatalf("mongo search: hits=%+v err=%v", hits, err)
	}
	fileHits, _, err := query.SearchMessages(ctx, aliceID, topic, "", domain.SearchFilter{HasFile: true}, 20, 0)
	if err != nil || len(fileHits) != 1 || !fileHits[0].HasFile {
		t.Fatalf("mongo has_file: hits=%+v err=%v", fileHits, err)
	}
	if _, _, err := query.SearchMessages(ctx, eveID, topic, "fallback", domain.SearchFilter{}, 20, 0); err == nil {
		t.Fatal("expected cross-topic search denial")
	}
	// The HTTP layer still uses the Meilisearch backend in this test server;
	// the Mongo adapter is exercised directly above.
}

func waitForSearch(t *testing.T, token, topic, query string, want int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var last int
	for time.Now().Before(deadline) {
		resp := httpGet(t, fmt.Sprintf("%s/v1/message/search?topic=%s&query=%s", testBaseURL, topic, query), token)
		if resp.StatusCode == http.StatusOK {
			var reply struct {
				Messages []json.RawMessage `json:"messages"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&reply)
			resp.Body.Close()
			last = len(reply.Messages)
			if last >= want {
				return
			}
		} else {
			resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("search %q returned %d hits, want at least %d", query, last, want)
}
