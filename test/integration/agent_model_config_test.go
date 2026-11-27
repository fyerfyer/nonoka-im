package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"testing"
)

// TestAgentModelConfigLifecycle verifies the flat Bifrost model catalog
// boundary: agents reference only model_id, provider details never appear in
// DTOs, and admins curate the read-only catalog.
func TestAgentModelConfigLifecycle(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, userID := registerAndLogin(t, "agent-model-owner", "123456")
	if err := ts.openFGA.SetPlatformAdmin(t.Context(), userID, true); err != nil {
		t.Fatalf("grant platform admin: %v", err)
	}

	// Old per-agent model credentials are no longer accepted.
	badResp := httpPostAuth(t, testBaseURL+"/v1/agents", token, map[string]any{
		"name":        "bad-url-agent",
		"instruction": "test",
		"base_url":    "https://example.com/v1",
		"api_key":     "sk-test-secret-123",
	})
	badResp.Body.Close()
	if badResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for per-agent base_url/api_key, got %d", badResp.StatusCode)
	}

	// The admin catalog is flat and does not expose provider credentials.
	listResp := httpGet(t, testBaseURL+"/v1/models", token)
	listBody, _ := io.ReadAll(listResp.Body)
	listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list models: %d: %s", listResp.StatusCode, listBody)
	}
	if containsSecret(listBody) || containsString(listBody, "fixture.invalid") {
		t.Fatalf("model catalog leaked gateway details: %s", listBody)
	}
	var catalog struct {
		Models []struct {
			ID       int64  `json:"id"`
			ModelKey string `json:"modelKey"`
			Provider string `json:"provider"`
			Enabled  bool   `json:"enabled"`
			Curated  bool   `json:"curated"`
		} `json:"models"`
		CanManage bool `json:"canManage"`
	}
	if err := json.Unmarshal(listBody, &catalog); err != nil || len(catalog.Models) == 0 {
		t.Fatalf("decode model catalog: %v (%s)", err, listBody)
	}
	if !catalog.CanManage {
		t.Fatal("platform admin should be able to manage the catalog")
	}
	model := catalog.Models[0]

	// Non-admin catalog hides disabled/uncurated models but still shows no
	// provider URL or credentials.
	tokenUser, _ := registerAndLogin(t, "agent-model-user", "123456")
	userResp := httpGet(t, testBaseURL+"/v1/models", tokenUser)
	userBody, _ := io.ReadAll(userResp.Body)
	userResp.Body.Close()
	if userResp.StatusCode != http.StatusOK || containsSecret(userBody) || containsString(userBody, "fixture.invalid") {
		t.Fatalf("non-admin catalog leaked details: %d %s", userResp.StatusCode, userBody)
	}

	// Curation toggles a model without touching provider configuration.
	curateResp := httpPatchAuth(t, testBaseURL+"/v1/models/"+strconv.FormatInt(model.ID, 10), token, map[string]any{
		"displayName": "Fixture Curated", "enabled": true, "recommended": true,
	})
	curateBody, _ := io.ReadAll(curateResp.Body)
	curateResp.Body.Close()
	if curateResp.StatusCode != http.StatusOK {
		t.Fatalf("curate model: %d: %s", curateResp.StatusCode, curateBody)
	}

	createResp := httpPostAuth(t, testBaseURL+"/v1/agents", token, map[string]any{
		"name":        "model-agent",
		"instruction": "you are a fixture-backed agent",
		"modelId":     model.ID,
		"maxTokens":   1024,
	})
	body, _ := io.ReadAll(createResp.Body)
	createResp.Body.Close()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("create agent failed: status %d: %s", createResp.StatusCode, body)
	}
	if containsSecret(body) || containsString(body, "fixture.invalid") {
		t.Fatalf("agent response leaked model gateway details: %s", body)
	}
	var created struct {
		ID    int64 `json:"id"`
		Draft struct {
			ModelID   int64 `json:"modelId"`
			MaxTokens int   `json:"maxTokens"`
			Revision  int64 `json:"revision"`
		} `json:"draft"`
	}
	if err := json.Unmarshal(body, &created); err != nil || created.ID == 0 {
		t.Fatalf("decode created agent failed: %v", err)
	}
	if created.Draft.ModelID != model.ID || created.Draft.MaxTokens != 1024 {
		t.Fatalf("draft model reference mismatch: %+v", created.Draft)
	}

	pubResp := httpPostAuth(t, testBaseURL+"/v1/agents/"+strconv.FormatInt(created.ID, 10)+"/publish", token, map[string]any{"revision": created.Draft.Revision})
	pubBody, _ := io.ReadAll(pubResp.Body)
	pubResp.Body.Close()
	if pubResp.StatusCode != http.StatusOK {
		t.Fatalf("publish failed: status %d: %s", pubResp.StatusCode, pubBody)
	}
	if containsSecret(pubBody) || containsString(pubBody, "baseUrl") || containsString(pubBody, "sk-bf-test") {
		t.Fatalf("publish response leaked gateway details: %s", pubBody)
	}
}

func containsSecret(body []byte) bool {
	return containsString(body, "sk-test-secret-123") || containsString(body, "apiKeyEnc") || containsString(body, "sk-bf-test")
}

func containsString(body []byte, needle string) bool {
	return len(needle) > 0 && len(body) >= len(needle) && (string(body) == needle || indexOf(body, needle) >= 0)
}

func indexOf(body []byte, needle string) int {
	for i := 0; i+len(needle) <= len(body); i++ {
		if string(body[i:i+len(needle)]) == needle {
			return i
		}
	}
	return -1
}
