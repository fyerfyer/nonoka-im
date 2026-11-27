package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestAgentToolsRegistryAndPolicies verifies the shared Tool Registry, hidden
// system tools, capability presets and auto/ask/deny resolution.
func TestAgentToolsRegistryAndPolicies(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "tools-registry-owner", "123456")

	registry := httpGet(t, testBaseURL+"/v1/agent-tools", token)
	regBody, _ := io.ReadAll(registry.Body)
	registry.Body.Close()
	if registry.StatusCode != http.StatusOK {
		t.Fatalf("agent-tools: %d: %s", registry.StatusCode, regBody)
	}
	for _, needle := range []string{`"presets"`, `"im.read"`, `"get_current_time"`, `"system":true`, `"hidden":true`} {
		if !strings.Contains(string(regBody), needle) {
			t.Fatalf("tool registry missing %s: %s", needle, regBody)
		}
	}

	create := httpPostAuth(t, testBaseURL+"/v1/agents", token, map[string]any{
		"name": "tools-agent", "instruction": "test tools",
		"modelId":           testModelID,
		"capabilityPresets": []string{"im.read", "im.write"},
		"toolPolicies":      map[string]string{"send_message": "ask", "read_file": "deny"},
	})
	createBody, _ := io.ReadAll(create.Body)
	create.Body.Close()
	if create.StatusCode != http.StatusOK {
		t.Fatalf("create agent: %d: %s", create.StatusCode, createBody)
	}
	var created struct {
		ID    int64 `json:"id"`
		Draft struct {
			Revision int64 `json:"revision"`
		} `json:"draft"`
	}
	if err := json.Unmarshal(createBody, &created); err != nil || created.ID == 0 {
		t.Fatalf("decode agent: %v (%s)", err, createBody)
	}
	pub := httpPostAuth(t, testBaseURL+"/v1/agents/"+int64Str(created.ID)+"/publish", token, map[string]any{"revision": created.Draft.Revision})
	pubBody, _ := io.ReadAll(pub.Body)
	pub.Body.Close()
	if pub.StatusCode != http.StatusOK {
		t.Fatalf("publish: %d: %s", pub.StatusCode, pubBody)
	}
	var version struct {
		Tools        []string          `json:"tools"`
		ToolPolicies map[string]string `json:"toolPolicies"`
	}
	if err := json.Unmarshal(pubBody, &version); err != nil {
		t.Fatalf("decode version: %v", err)
	}
	tools := version.Tools
	has := func(name string) bool {
		for _, tool := range tools {
			if tool == name {
				return true
			}
		}
		return false
	}
	if !has("send_message") || !has("search_messages") || !has("get_current_time") {
		t.Fatalf("resolved tools missing expected entries: %v", tools)
	}
	if has("read_file") {
		t.Fatalf("deny policy did not remove read_file: %v", tools)
	}
	if version.ToolPolicies["send_message"] != "ask" {
		t.Fatalf("policy not frozen: %+v", version.ToolPolicies)
	}

	// Denying a hidden system tool is rejected.
	bad := httpPutAuth(t, testBaseURL+"/v1/agents/"+int64Str(created.ID)+"/draft", token, map[string]any{
		"name": "tools-agent", "instruction": "test tools", "revision": created.Draft.Revision,
		"modelId":           testModelID,
		"capabilityPresets": []string{"im.read"},
		"toolPolicies":      map[string]string{"get_current_time": "deny"},
	})
	bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("system tool deny accepted: %d", bad.StatusCode)
	}
}
