package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestAgentCreateIdempotencyReplays verifies the generated Agent endpoints
// preserve the previous Idempotency-Key contract: the same key and request
// execute once and the stored response is replayed.
func TestAgentCreateIdempotencyReplays(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "agent-idem-owner", "123456")
	key := "agent-create-" + time.Now().Format("150405.000000000")
	payload := map[string]any{
		"name":        "idempotent-agent",
		"instruction": "be idempotent",
		"modelId":     testModelID,
	}

	first := httpPostAuthIdempotent(t, testBaseURL+"/v1/agents", token, key, payload)
	firstBody, _ := io.ReadAll(first.Body)
	first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first create: %d: %s", first.StatusCode, firstBody)
	}

	second := httpPostAuthIdempotent(t, testBaseURL+"/v1/agents", token, key, payload)
	secondBody, _ := io.ReadAll(second.Body)
	second.Body.Close()
	if second.StatusCode != http.StatusOK {
		t.Fatalf("replay create: %d: %s", second.StatusCode, secondBody)
	}
	if second.Header.Get("Idempotent-Replayed") != "true" {
		t.Fatalf("replay header missing: %v", second.Header)
	}
	if !bytes.Equal(firstBody, secondBody) {
		t.Fatalf("replayed response differs:\nfirst=%s\nsecond=%s", firstBody, secondBody)
	}

	list := httpGet(t, testBaseURL+"/v1/agents", token)
	listBody, _ := io.ReadAll(list.Body)
	list.Body.Close()
	var page struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(listBody, &page); err != nil {
		t.Fatalf("decode agents: %v (%s)", err, listBody)
	}
	if len(page.Items) != 1 {
		t.Fatalf("idempotent create produced %d agents: %s", len(page.Items), listBody)
	}

	mismatch := httpPostAuthIdempotent(t, testBaseURL+"/v1/agents", token, key, map[string]any{
		"name":        "different-agent",
		"instruction": "different",
		"modelId":     testModelID,
	})
	mismatchBody, _ := io.ReadAll(mismatch.Body)
	mismatch.Body.Close()
	if mismatch.StatusCode != http.StatusConflict || !strings.Contains(string(mismatchBody), "IDEMPOTENCY_MISMATCH") {
		t.Fatalf("mismatched key: %d: %s", mismatch.StatusCode, mismatchBody)
	}
}

func httpPostAuthIdempotent(t *testing.T, url, token, key string, body any) *http.Response {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Idempotency-Key", key)
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("post %s: %v", url, err)
	}
	return resp
}
