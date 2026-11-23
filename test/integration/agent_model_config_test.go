package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"testing"
)

// TestAgentModelConfigLifecycle verifies the platform ModelService boundary:
// agents only reference service/model IDs, the gateway URL/key never appear in
// agent DTOs, and the old per-agent base_url/api_key fields are rejected.
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

	// Platform admin creates a manual OpenAI-compatible service and one model.
	svcResp := httpPostAuth(t, testBaseURL+"/v1/model-services", token, map[string]any{
		"name": "fixture-manual", "displayName": "Fixture Manual", "gatewayType": "openai_compatible",
		"baseUrl": "https://fixture.invalid/v1", "apiKey": "sk-test-secret-123",
	})
	svcBody, _ := io.ReadAll(svcResp.Body)
	svcResp.Body.Close()
	if svcResp.StatusCode != http.StatusCreated {
		t.Fatalf("create model service: %d: %s", svcResp.StatusCode, svcBody)
	}
	if string(svcBody) == "" || containsSecret(svcBody) {
		t.Fatalf("model service response leaked the key: %s", svcBody)
	}
	var svc struct {
		ID        int64  `json:"id"`
		BaseURL   string `json:"baseUrl"`
		HasAPIKey bool   `json:"hasApiKey"`
	}
	if err := json.Unmarshal(svcBody, &svc); err != nil || svc.ID == 0 {
		t.Fatalf("decode service: %v (%s)", err, svcBody)
	}
	if svc.BaseURL == "" || !svc.HasAPIKey {
		t.Fatalf("admin service view missing metadata: %+v", svc)
	}
	modelResp := httpPostAuth(t, testBaseURL+"/v1/model-services/"+strconv.FormatInt(svc.ID, 10)+"/models", token, map[string]any{
		"modelKey": "fixture-model", "displayName": "Fixture", "maxOutputTokens": 2048, "supportsTools": true,
	})
	modelBody, _ := io.ReadAll(modelResp.Body)
	modelResp.Body.Close()
	if modelResp.StatusCode != http.StatusCreated {
		t.Fatalf("create model: %d: %s", modelResp.StatusCode, modelBody)
	}
	var model struct {
		ID uint64 `json:"id"`
	}
	if err := json.Unmarshal(modelBody, &model); err != nil || model.ID == 0 {
		t.Fatalf("decode model: %v (%s)", err, modelBody)
	}

	// Non-admin catalog hides the gateway URL and credentials.
	tokenUser, _ := registerAndLogin(t, "agent-model-user", "123456")
	listResp := httpGet(t, testBaseURL+"/v1/model-services", tokenUser)
	listBody, _ := io.ReadAll(listResp.Body)
	listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list services: %d: %s", listResp.StatusCode, listBody)
	}
	if containsSecret(listBody) || containsString(listBody, "fixture.invalid") {
		t.Fatalf("non-admin service list leaked gateway details: %s", listBody)
	}

	// Create and publish an agent that references the platform model.
	createResp := httpPostAuth(t, testBaseURL+"/v1/agents", token, map[string]any{
		"name":           "model-agent",
		"instruction":    "you are a fixture-backed agent",
		"modelServiceId": svc.ID,
		"modelId":        model.ID,
		"maxTokens":      1024,
	})
	body, _ := io.ReadAll(createResp.Body)
	createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create agent failed: status %d: %s", createResp.StatusCode, body)
	}
	if containsSecret(body) || containsString(body, "fixture.invalid") {
		t.Fatalf("agent response leaked model service details: %s", body)
	}
	var created struct {
		ID    int64 `json:"id"`
		Draft struct {
			ModelServiceID int64 `json:"modelServiceId"`
			ModelID        int64 `json:"modelId"`
			MaxTokens      int   `json:"maxTokens"`
			Revision       int64 `json:"revision"`
		} `json:"draft"`
	}
	if err := json.Unmarshal(body, &created); err != nil || created.ID == 0 {
		t.Fatalf("decode created agent failed: %v", err)
	}
	if created.Draft.ModelServiceID != svc.ID || created.Draft.ModelID != int64(model.ID) || created.Draft.MaxTokens != 1024 {
		t.Fatalf("draft model reference mismatch: %+v", created.Draft)
	}

	pubResp := httpPostAuth(t, testBaseURL+"/v1/agents/"+strconv.FormatInt(created.ID, 10)+"/publish", token, map[string]any{"revision": created.Draft.Revision})
	pubBody, _ := io.ReadAll(pubResp.Body)
	pubResp.Body.Close()
	if pubResp.StatusCode != http.StatusOK {
		t.Fatalf("publish failed: status %d: %s", pubResp.StatusCode, pubBody)
	}
	if containsSecret(pubBody) || containsString(pubBody, "baseUrl") {
		t.Fatalf("publish response leaked model service details: %s", pubBody)
	}
}

func containsSecret(body []byte) bool {
	return containsString(body, "sk-test-secret-123") || containsString(body, "apiKeyEnc")
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
