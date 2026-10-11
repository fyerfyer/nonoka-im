package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// startRegistryFixture starts a minimal MCP server for registry tests.
func startRegistryFixture(t *testing.T) *httptest.Server {
	t.Helper()
	s := mcpserver.NewMCPServer("registry-fixture", "1.0.0")
	s.AddTool(mcpgo.NewTool("echo", mcpgo.WithDescription("echo"), mcpgo.WithString("text", mcpgo.Required())), func(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		text, _ := req.RequireString("text")
		return mcpgo.NewToolResultText(text), nil
	})
	srv := httptest.NewServer(mcpserver.NewStreamableHTTPServer(s))
	t.Cleanup(srv.Close)
	return srv
}

// TestAgentExtAdminAndBindings covers the MCP registry admin APIs, the skill
// publishing flow, and agent draft binding validation.
func TestAgentExtAdminAndBindings(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	tokenAdmin, userAdmin := registerAndLogin(t, "ext-admin", "123456")
	tokenUser, _ := registerAndLogin(t, "ext-user", "123456")
	t.Setenv("AGENT_ADMIN_USER_IDS", int64Str(userAdmin))

	fixture := startRegistryFixture(t)

	// Non-admin registration is refused.
	forbidden := httpPostAuth(t, testBaseURL+"/v1/mcp/servers", tokenUser, map[string]any{"name": "x", "url": fixture.URL, "tools": []string{"echo"}})
	forbidden.Body.Close()
	if forbidden.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin register: %d", forbidden.StatusCode)
	}

	// Admin registers the fixture server.
	register := httpPostAuth(t, testBaseURL+"/v1/mcp/servers", tokenAdmin, map[string]any{"name": "fixture", "url": fixture.URL, "tools": []string{"echo"}, "risk": "read_only"})
	regBody, _ := io.ReadAll(register.Body)
	register.Body.Close()
	if register.StatusCode != http.StatusCreated {
		t.Fatalf("admin register: %d: %s", register.StatusCode, regBody)
	}

	// Non-admin skill publishing is refused.
	badSkill := httpPostAuth(t, testBaseURL+"/v1/skills", tokenUser, map[string]any{"name": "n", "version": 1, "content": "---\nname: n\n---\nbody"})
	badSkill.Body.Close()
	if badSkill.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin publish skill: %d", badSkill.StatusCode)
	}

	// Admin publishes a skill; everyone can list published skills.
	publish := httpPostAuth(t, testBaseURL+"/v1/skills", tokenAdmin, map[string]any{"name": "summarizer", "version": 1, "content": "---\nname: summarizer\ndescription: Summarize\n---\nUse bullets."})
	publish.Body.Close()
	if publish.StatusCode != http.StatusCreated {
		t.Fatalf("admin publish skill: %d", publish.StatusCode)
	}
	skillsResp := httpGet(t, testBaseURL+"/v1/skills", tokenUser)
	skillsBody, _ := io.ReadAll(skillsResp.Body)
	skillsResp.Body.Close()
	if !strings.Contains(string(skillsBody), "summarizer") {
		t.Fatalf("published skill not listed: %s", skillsBody)
	}

	// Agent draft with valid MCP + skill bindings publishes and freezes them.
	create := httpPostAuth(t, testBaseURL+"/v1/agents", tokenAdmin, map[string]any{
		"name":        "ext-agent",
		"instruction": "test agent",
		"mcp_bindings": []map[string]any{
			{"server": "fixture", "tool": "echo"},
		},
		"skills": []map[string]any{
			{"name": "summarizer", "version": 1},
		},
	})
	createBody, _ := io.ReadAll(create.Body)
	create.Body.Close()
	if create.StatusCode != http.StatusCreated {
		t.Fatalf("create agent with bindings: %d: %s", create.StatusCode, createBody)
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

	// Binding a tool the registry does not allow is rejected.
	badBind := httpPutAuth(t, testBaseURL+"/v1/agents/"+int64Str(created.ID)+"/draft", tokenAdmin, map[string]any{
		"name":         "ext-agent",
		"instruction":  "test agent",
		"revision":     created.Draft.Revision,
		"mcp_bindings": []map[string]any{{"server": "fixture", "tool": "steal"}},
	})
	badBind.Body.Close()
	if badBind.StatusCode != http.StatusBadRequest {
		t.Fatalf("unlisted tool binding: %d", badBind.StatusCode)
	}

	pub := httpPostAuth(t, testBaseURL+"/v1/agents/"+int64Str(created.ID)+"/publish", tokenAdmin, map[string]any{"revision": created.Draft.Revision})
	pubBody, _ := io.ReadAll(pub.Body)
	pub.Body.Close()
	if pub.StatusCode != http.StatusOK {
		t.Fatalf("publish: %d: %s", pub.StatusCode, pubBody)
	}
	if !strings.Contains(string(pubBody), "mcp_fixture_echo") || !strings.Contains(string(pubBody), "summarizer") {
		t.Fatalf("bindings not frozen into version: %s", pubBody)
	}

	// The published MCP tool is authorized for the owner via OpenFGA.
	if err := ts.openFGA.CanExecuteTool(context.Background(), userAdmin, created.ID, "mcp_fixture_echo", "p2p_1_2", "external", ""); err != nil {
		t.Fatalf("published mcp tool not authorized: %v", err)
	}
	if err := ts.openFGA.CanExecuteTool(context.Background(), userAdmin, created.ID, "mcp_fixture_steal", "p2p_1_2", "external", ""); err == nil {
		t.Fatalf("unbound mcp tool authorized")
	}
}

func int64Str(v int64) string { return strconv.FormatInt(v, 10) }
