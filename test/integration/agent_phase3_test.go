package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"nonoka-im/internal/agent/domain"
	agentdata "nonoka-im/internal/data/agent"
)

func TestAgentVersionsRestoreAndPublishNote(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, _ := registerAndLogin(t, "phase3-version-owner", "123456")
	created := createPhase3Agent(t, token, "version-agent", "first instructions")
	publishPhase3Agent(t, token, created.ID, created.Revision, "initial release")

	versions := httpGet(t, testBaseURL+"/v1/agents/"+int64Str(created.ID)+"/versions", token)
	versionsBody, _ := io.ReadAll(versions.Body)
	versions.Body.Close()
	if versions.StatusCode != http.StatusOK {
		t.Fatalf("versions: %d: %s", versions.StatusCode, versionsBody)
	}
	if !containsAll(string(versionsBody), `"version":1`, "initial release", `"sourceRevision":1`) {
		t.Fatalf("unexpected versions response: %s", versionsBody)
	}

	detail := httpGet(t, testBaseURL+"/v1/agents/"+int64Str(created.ID)+"/versions/1", token)
	detailBody, _ := io.ReadAll(detail.Body)
	detail.Body.Close()
	if detail.StatusCode != http.StatusOK {
		t.Fatalf("version detail: %d: %s", detail.StatusCode, detailBody)
	}
	if !containsAll(string(detailBody), "first instructions", "initial release") {
		t.Fatalf("version detail missing content: %s", detailBody)
	}

	// Edit the draft, then restore v1 and verify the draft snapshot comes back.
	update := httpPutAuth(t, testBaseURL+"/v1/agents/"+int64Str(created.ID)+"/draft", token, map[string]any{
		"name": "version-agent", "instruction": "second instructions",
		"modelId":           testModelID,
		"capabilityPresets": []string{"im.read"}, "toolPolicies": map[string]string{},
		"revision": created.Revision,
	})
	update.Body.Close()
	if update.StatusCode != http.StatusOK {
		t.Fatalf("update draft: %d", update.StatusCode)
	}

	restore := httpPostAuth(t, testBaseURL+"/v1/agents/"+int64Str(created.ID)+"/restore", token, map[string]any{
		"version": 1, "revision": created.Revision + 1,
	})
	restoreBody, _ := io.ReadAll(restore.Body)
	restore.Body.Close()
	if restore.StatusCode != http.StatusOK {
		t.Fatalf("restore: %d: %s", restore.StatusCode, restoreBody)
	}
	var restored struct {
		Draft struct {
			Instruction string `json:"instruction"`
			Revision    int64  `json:"revision"`
		} `json:"draft"`
	}
	if err := json.Unmarshal(restoreBody, &restored); err != nil {
		t.Fatalf("decode restore: %v (%s)", err, restoreBody)
	}
	if restored.Draft.Instruction != "first instructions" || restored.Draft.Revision <= created.Revision+1 {
		t.Fatalf("restore did not copy the version into the draft: %+v", restored.Draft)
	}
}

func TestAgentShareRolesAndOwnerOnlyDestructiveActions(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	ownerToken, _ := registerAndLogin(t, "phase3-role-owner", "123456")
	editorToken, editorID := registerAndLogin(t, "phase3-role-editor", "123456")
	created := createPhase3Agent(t, ownerToken, "role-agent", "instructions")

	share := httpPutAuth(t, testBaseURL+"/v1/agents/"+int64Str(created.ID)+"/shares", ownerToken, map[string]any{
		"kind": "user", "target": int64Str(editorID), "role": "editor", "enabled": true,
	})
	share.Body.Close()
	if share.StatusCode != http.StatusOK {
		t.Fatalf("share editor: %d", share.StatusCode)
	}

	detail := httpGet(t, testBaseURL+"/v1/agents/"+int64Str(created.ID), editorToken)
	detailBody, _ := io.ReadAll(detail.Body)
	detail.Body.Close()
	if detail.StatusCode != http.StatusOK {
		t.Fatalf("editor detail: %d: %s", detail.StatusCode, detailBody)
	}
	var view struct {
		CanManage bool `json:"canManage"`
		CanOwn    bool `json:"canOwn"`
	}
	_ = json.Unmarshal(detailBody, &view)
	if !view.CanManage || view.CanOwn {
		t.Fatalf("editor permissions wrong: %+v (%s)", view, detailBody)
	}

	// Editors can edit the draft.
	update := httpPutAuth(t, testBaseURL+"/v1/agents/"+int64Str(created.ID)+"/draft", editorToken, map[string]any{
		"name": "role-agent", "instruction": "editor changed this",
		"modelId":           testModelID,
		"capabilityPresets": []string{"im.read"}, "toolPolicies": map[string]string{},
		"revision": created.Revision,
	})
	update.Body.Close()
	if update.StatusCode != http.StatusOK {
		t.Fatalf("editor update: %d", update.StatusCode)
	}

	// But not share/delete.
	deniedShare := httpPutAuth(t, testBaseURL+"/v1/agents/"+int64Str(created.ID)+"/shares", editorToken, map[string]any{
		"kind": "user", "target": int64Str(editorID), "role": "viewer", "enabled": true,
	})
	deniedShare.Body.Close()
	if deniedShare.StatusCode != http.StatusForbidden {
		t.Fatalf("editor share should be forbidden, got %d", deniedShare.StatusCode)
	}
	deniedDelete := httpDoAuth(t, http.MethodDelete, testBaseURL+"/v1/agents/"+int64Str(created.ID), editorToken, nil)
	deniedDelete.Body.Close()
	if deniedDelete.StatusCode != http.StatusForbidden {
		t.Fatalf("editor delete should be forbidden, got %d", deniedDelete.StatusCode)
	}
}

func TestAgentDraftTestRunAndCancel(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token, ownerID := registerAndLogin(t, "phase3-test-owner", "123456")
	created := createPhase3Agent(t, token, "test-agent", "draft instructions")

	testRun := httpPostAuth(t, testBaseURL+"/v1/agents/"+int64Str(created.ID)+"/test", token, map[string]any{
		"message": "hello draft",
	})
	testBody, _ := io.ReadAll(testRun.Body)
	testRun.Body.Close()
	if testRun.StatusCode != http.StatusAccepted {
		t.Fatalf("test run: %d: %s", testRun.StatusCode, testBody)
	}
	var run struct {
		ID        uint64 `json:"id"`
		Kind      string `json:"kind"`
		Status    string `json:"status"`
		InputText string `json:"inputText"`
	}
	if err := json.Unmarshal(testBody, &run); err != nil {
		t.Fatalf("decode test run: %v (%s)", err, testBody)
	}
	if run.ID == 0 || run.Kind != domain.RunKindTest || run.Status != domain.RunQueued || run.InputText != "hello draft" {
		t.Fatalf("unexpected test run: %+v", run)
	}

	// The draft test must not create a published version.
	detail := httpGet(t, testBaseURL+"/v1/agents/"+int64Str(created.ID), token)
	detailBody, _ := io.ReadAll(detail.Body)
	detail.Body.Close()
	if !containsAll(string(detailBody), `"currentVersion":0`) {
		t.Fatalf("test run changed published version: %s", detailBody)
	}

	cancel := httpPostAuth(t, testBaseURL+"/v1/agent-runs/"+uint64Str(run.ID)+"/cancel", token, map[string]any{})
	cancel.Body.Close()
	if cancel.StatusCode != http.StatusOK {
		t.Fatalf("cancel: %d", cancel.StatusCode)
	}
	stored, err := agentdata.NewStore(ts.data.DB()).GetRun(t.Context(), run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if stored.Status != domain.RunCancelled || stored.ActorUserID != ownerID {
		t.Fatalf("run not cancelled: %+v", stored)
	}
}

type phase3Agent struct {
	ID       int64
	Revision int64
}

func createPhase3Agent(t *testing.T, token, name, instruction string) phase3Agent {
	t.Helper()
	resp := httpPostAuth(t, testBaseURL+"/v1/agents", token, map[string]any{
		"name": name, "instruction": instruction,
		"modelId":           testModelID,
		"capabilityPresets": []string{"im.read"}, "toolPolicies": map[string]string{},
	})
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create agent: %d: %s", resp.StatusCode, body)
	}
	var created struct {
		ID    int64 `json:"id"`
		Draft struct {
			Revision int64 `json:"revision"`
		} `json:"draft"`
	}
	if err := json.Unmarshal(body, &created); err != nil || created.ID == 0 {
		t.Fatalf("decode agent: %v (%s)", err, body)
	}
	return phase3Agent{ID: created.ID, Revision: created.Draft.Revision}
}

func publishPhase3Agent(t *testing.T, token string, agentID, revision int64, note string) int64 {
	t.Helper()
	resp := httpPostAuth(t, testBaseURL+"/v1/agents/"+int64Str(agentID)+"/publish", token, map[string]any{
		"revision": revision, "note": note,
	})
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("publish agent: %d: %s", resp.StatusCode, body)
	}
	var version struct {
		Version int64 `json:"version"`
	}
	_ = json.Unmarshal(body, &version)
	return version.Version
}

func containsAll(body string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(body, needle) {
			return false
		}
	}
	return true
}

func uint64Str(value uint64) string {
	if value == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = byte('0' + value%10)
		value /= 10
	}
	return string(buf[i:])
}
