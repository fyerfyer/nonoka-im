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
	s.AddTool(mcpgo.NewTool("echo", mcpgo.WithDescription("echo"), mcpgo.WithString("text", mcpgo.Required()), mcpgo.WithReadOnlyHintAnnotation(true)), func(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		text, _ := req.RequireString("text")
		return mcpgo.NewToolResultText(text), nil
	})
	srv := httptest.NewServer(mcpserver.NewStreamableHTTPServer(s))
	t.Cleanup(srv.Close)
	return srv
}

// TestAgentMCPAndSkillSelfService covers user-owned MCP discovery, skill
// upload/validation, agent binding and the review gate for shared resources.
func TestAgentMCPAndSkillSelfService(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	tokenOwner, userOwner := registerAndLogin(t, "ext-owner", "123456")
	tokenOther, userOther := registerAndLogin(t, "ext-other", "123456")
	tokenAdmin, userAdmin := registerAndLogin(t, "ext-admin", "123456")
	if err := ts.openFGA.SetPlatformAdmin(t.Context(), userAdmin, true); err != nil {
		t.Fatalf("grant platform admin: %v", err)
	}
	fixture := startRegistryFixture(t)

	// A regular user can register a private MCP server; discovery runs
	// immediately and stores the tool schema.
	register := httpPostAuth(t, testBaseURL+"/v1/mcp/servers", tokenOwner, map[string]any{
		"name": "fixture", "displayName": "Fixture", "url": fixture.URL,
		"authType": "none", "visibility": "private", "transport": "streamable_http",
	})
	regBody, _ := io.ReadAll(register.Body)
	register.Body.Close()
	if register.StatusCode != http.StatusCreated {
		t.Fatalf("register mcp: %d: %s", register.StatusCode, regBody)
	}
	var server struct {
		ID           int64  `json:"id"`
		ReviewStatus string `json:"reviewStatus"`
		Tools        []struct {
			Name       string `json:"name"`
			Risk       string `json:"risk"`
			SchemaHash string `json:"schemaHash"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(regBody, &server); err != nil || server.ID == 0 {
		t.Fatalf("decode server: %v (%s)", err, regBody)
	}
	if server.ReviewStatus != "approved" || len(server.Tools) != 1 || server.Tools[0].Name != "echo" || server.Tools[0].Risk != "read_only" {
		t.Fatalf("unexpected discovery result: %+v", server)
	}

	// Private skills are usable immediately.
	skillResp := httpPostAuth(t, testBaseURL+"/v1/skills", tokenOwner, map[string]any{
		"slug": "summarizer", "displayName": "Summarizer", "visibility": "private",
		"content": "---\nname: summarizer\ndescription: Summarize\n---\nUse bullets.",
	})
	skillBody, _ := io.ReadAll(skillResp.Body)
	skillResp.Body.Close()
	if skillResp.StatusCode != http.StatusCreated {
		t.Fatalf("create skill: %d: %s", skillResp.StatusCode, skillBody)
	}
	var skill struct {
		ID            int64 `json:"id"`
		LatestVersion int64 `json:"latestVersion"`
	}
	if err := json.Unmarshal(skillBody, &skill); err != nil || skill.ID == 0 {
		t.Fatalf("decode skill: %v (%s)", err, skillBody)
	}

	// Invalid front matter is rejected.
	badSkill := httpPostAuth(t, testBaseURL+"/v1/skills", tokenOwner, map[string]any{
		"slug": "bad", "content": "---\nname: bad\n---\nbody",
	})
	badSkill.Body.Close()
	if badSkill.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid skill accepted: %d", badSkill.StatusCode)
	}

	create := httpPostAuth(t, testBaseURL+"/v1/agents", tokenOwner, map[string]any{
		"name": "ext-agent", "instruction": "test agent",
		"modelId":           testModelID,
		"capabilityPresets": []string{"im.read"},
		"mcpBindings": []map[string]any{
			{"serverId": server.ID, "tool": "echo"},
		},
		"skills": []map[string]any{{"skillId": skill.ID, "version": 1}},
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
	pub := httpPostAuth(t, testBaseURL+"/v1/agents/"+int64Str(created.ID)+"/publish", tokenOwner, map[string]any{"revision": created.Draft.Revision})
	pubBody, _ := io.ReadAll(pub.Body)
	pub.Body.Close()
	if pub.StatusCode != http.StatusOK {
		t.Fatalf("publish: %d: %s", pub.StatusCode, pubBody)
	}
	exposed := "mcp_" + int64Str(server.ID) + "_echo"
	if !strings.Contains(string(pubBody), exposed) || !strings.Contains(string(pubBody), "summarizer") {
		t.Fatalf("bindings not frozen into version: %s", pubBody)
	}
	if err := ts.openFGA.CanExecuteTool(context.Background(), userOwner, created.ID, exposed, "p2p_1_2", "external", ""); err != nil {
		t.Fatalf("published mcp tool not authorized: %v", err)
	}

	// A published agent with private MCP/skill bindings cannot be shared.
	share := httpPutAuth(t, testBaseURL+"/v1/agents/"+int64Str(created.ID)+"/shares", tokenOwner, map[string]any{"kind": "user", "target": int64Str(userOther), "enabled": true})
	share.Body.Close()
	if share.StatusCode != http.StatusBadRequest {
		t.Fatalf("shared agent with private resources accepted: %d", share.StatusCode)
	}

	// Removing the private resources allows sharing and republishing.
	draft := httpPutAuth(t, testBaseURL+"/v1/agents/"+int64Str(created.ID)+"/draft", tokenOwner, map[string]any{
		"name": "ext-agent", "instruction": "test agent v2",
		"modelId":           testModelID,
		"capabilityPresets": []string{"im.read"},
		"mcpBindings":       []map[string]any{},
		"skills":            []map[string]any{},
		"revision":          created.Draft.Revision,
	})
	draftBody, _ := io.ReadAll(draft.Body)
	draft.Body.Close()
	if draft.StatusCode != http.StatusOK {
		t.Fatalf("update draft: %d: %s", draft.StatusCode, draftBody)
	}
	var updated struct {
		Draft struct {
			Revision int64 `json:"revision"`
		} `json:"draft"`
	}
	if err := json.Unmarshal(draftBody, &updated); err != nil {
		t.Fatalf("decode draft: %v", err)
	}
	rePublish := httpPostAuth(t, testBaseURL+"/v1/agents/"+int64Str(created.ID)+"/publish", tokenOwner, map[string]any{"revision": updated.Draft.Revision})
	rePublish.Body.Close()
	if rePublish.StatusCode != http.StatusOK {
		t.Fatalf("republish without private resources: %d", rePublish.StatusCode)
	}
	share = httpPutAuth(t, testBaseURL+"/v1/agents/"+int64Str(created.ID)+"/shares", tokenOwner, map[string]any{"kind": "user", "target": int64Str(userOther), "enabled": true})
	share.Body.Close()
	if share.StatusCode != http.StatusOK {
		t.Fatalf("share agent after removing private resources: %d", share.StatusCode)
	}

	// Shared/public MCP servers require admin review before binding.
	sharedResp := httpPostAuth(t, testBaseURL+"/v1/mcp/servers", tokenOwner, map[string]any{
		"name": "shared-fixture", "url": fixture.URL, "authType": "none", "visibility": "shared",
	})
	sharedBody, _ := io.ReadAll(sharedResp.Body)
	sharedResp.Body.Close()
	if sharedResp.StatusCode != http.StatusCreated {
		t.Fatalf("register shared mcp: %d: %s", sharedResp.StatusCode, sharedBody)
	}
	var shared struct {
		ID           int64  `json:"id"`
		ReviewStatus string `json:"reviewStatus"`
	}
	if err := json.Unmarshal(sharedBody, &shared); err != nil || shared.ID == 0 {
		t.Fatalf("decode shared server: %v (%s)", err, sharedBody)
	}
	if shared.ReviewStatus != "pending" {
		t.Fatalf("shared server auto-approved: %+v", shared)
	}
	review := httpPostAuth(t, testBaseURL+"/v1/mcp/servers/"+int64Str(shared.ID)+"/review", tokenAdmin, map[string]any{"status": "approved"})
	review.Body.Close()
	if review.StatusCode != http.StatusOK {
		t.Fatalf("review mcp: %d", review.StatusCode)
	}
	// A different user can now see the approved shared server.
	list := httpGet(t, testBaseURL+"/v1/mcp/servers", tokenOther)
	listBody, _ := io.ReadAll(list.Body)
	list.Body.Close()
	if !strings.Contains(string(listBody), "shared-fixture") {
		t.Fatalf("approved shared server not visible: %s", listBody)
	}
}

func int64Str(v int64) string { return strconv.FormatInt(v, 10) }
