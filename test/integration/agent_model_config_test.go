package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// TestAgentModelConfigLifecycle verifies that an agent owner can configure a
// custom OpenAI-compatible endpoint (base URL + API key + max tokens), that
// the key is sealed and never echoed back, and that publishing freezes the
// configuration into the version snapshot.
func TestAgentModelConfigLifecycle(t *testing.T) {
	t.Setenv("AGENT_SECRET_KEY", strings.Repeat("s", 32))
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "agent-model-owner", "123456")

	// Invalid base URLs are rejected.
	badResp := httpPostAuth(t, testBaseURL+"/v1/agents", token, map[string]any{
		"name":        "bad-url-agent",
		"instruction": "test",
		"base_url":    "ftp://example.com",
	})
	badResp.Body.Close()
	if badResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for ftp base_url, got %d", badResp.StatusCode)
	}

	// Create with a custom endpoint and key.
	createResp := httpPostAuth(t, testBaseURL+"/v1/agents", token, map[string]any{
		"name":        "model-agent",
		"instruction": "you are a fixture-backed agent",
		"model":       "fixture-model",
		"base_url":    "https://api.example.com/v1",
		"api_key":     "sk-test-secret-123",
		"max_tokens":  1024,
	})
	body, _ := io.ReadAll(createResp.Body)
	createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create agent failed: status %d: %s", createResp.StatusCode, body)
	}
	if strings.Contains(string(body), "sk-test-secret-123") {
		t.Fatalf("create response leaks the API key")
	}
	var created struct {
		ID    int64 `json:"id"`
		Draft struct {
			BaseURL   string `json:"baseUrl"`
			HasAPIKey bool   `json:"hasApiKey"`
			MaxTokens int    `json:"maxTokens"`
			Model     string `json:"model"`
			Revision  int64  `json:"revision"`
		} `json:"draft"`
	}
	if err := json.Unmarshal(body, &created); err != nil || created.ID == 0 {
		t.Fatalf("decode created agent failed: %v", err)
	}
	if !created.Draft.HasAPIKey || created.Draft.BaseURL != "https://api.example.com/v1" || created.Draft.MaxTokens != 1024 || created.Draft.Model != "fixture-model" {
		t.Fatalf("draft config mismatch: %+v", created.Draft)
	}

	// Publish freezes the sealed key into the version; the response masks it.
	pubResp := httpPostAuth(t, testBaseURL+"/v1/agents/"+strconv.FormatInt(created.ID, 10)+"/publish", token, map[string]any{"revision": created.Draft.Revision})
	pubBody, _ := io.ReadAll(pubResp.Body)
	pubResp.Body.Close()
	if pubResp.StatusCode != http.StatusOK {
		t.Fatalf("publish failed: status %d: %s", pubResp.StatusCode, pubBody)
	}
	if strings.Contains(string(pubBody), "sk-test-secret-123") || strings.Contains(string(pubBody), "APIKeyEnc") {
		t.Fatalf("publish response leaks the sealed key: %s", pubBody)
	}
	var version struct {
		Version   int64  `json:"version"`
		BaseURL   string `json:"baseUrl"`
		Model     string `json:"model"`
		HasAPIKey bool   `json:"hasApiKey"`
		MaxTokens int    `json:"maxTokens"`
	}
	if err := json.Unmarshal(pubBody, &version); err != nil {
		t.Fatalf("decode version failed: %v", err)
	}
	if version.Version != 1 || !version.HasAPIKey || version.BaseURL != "https://api.example.com/v1" || version.MaxTokens != 1024 {
		t.Fatalf("version config mismatch: %+v", version)
	}

	// Detail keeps masking the key for the manager.
	detailResp := httpGet(t, testBaseURL+"/v1/agents/"+strconv.FormatInt(created.ID, 10), token)
	detailBody, _ := io.ReadAll(detailResp.Body)
	detailResp.Body.Close()
	if strings.Contains(string(detailBody), "sk-test-secret-123") {
		t.Fatalf("detail leaks the API key")
	}

	// A draft update without api_key keeps the existing sealed key.
	draftResp := httpPutAuth(t, testBaseURL+"/v1/agents/"+strconv.FormatInt(created.ID, 10)+"/draft", token, map[string]any{
		"name":        "model-agent",
		"instruction": "you are a fixture-backed agent v2",
		"model":       "fixture-model",
		"base_url":    "https://api.example.com/v1",
		"max_tokens":  2048,
		"revision":    created.Draft.Revision,
	})
	draftBody, _ := io.ReadAll(draftResp.Body)
	draftResp.Body.Close()
	if draftResp.StatusCode != http.StatusOK {
		t.Fatalf("draft update failed: status %d: %s", draftResp.StatusCode, draftBody)
	}
	if !strings.Contains(string(draftBody), `"hasApiKey":true`) {
		t.Fatalf("draft update dropped the sealed key: %s", draftBody)
	}
}
