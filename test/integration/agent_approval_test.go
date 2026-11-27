package integration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nonoka-im/internal/agent/credential"
	"nonoka-im/internal/agent/domain"
	agentdata "nonoka-im/internal/data/agent"
)

// newWaitingApprovalRun seeds a run stuck at waiting_approval plus its pending
// approval directly through the data layer: reaching this state through HTTP
// requires a real agent execution interrupted by a tool-approval request,
// which only the worker with a live LLM can produce.
func newWaitingApprovalRun(t *testing.T, store *agentdata.Store, agentID, actorID int64, topic, eventID string) (*domain.AgentRun, *domain.AgentApproval) {
	t.Helper()
	ctx := context.Background()
	run := &domain.AgentRun{
		EventID:      eventID,
		AgentID:      agentID,
		AgentVersion: 1,
		ActorUserID:  actorID,
		Topic:        topic,
		MsgID:        1,
		Status:       "waiting_approval",
	}
	created, err := store.EnqueueRun(ctx, run)
	if err != nil || !created {
		t.Fatalf("seed waiting_approval run: created=%v err=%v", created, err)
	}
	approval := &domain.AgentApproval{
		RunID:       run.ID,
		ToolCallID:  "agent:A;node:n;tool:1",
		Tool:        "send_message",
		ArgsSummary: `send "hello" to the topic`,
		ArgsHash:    "deadbeef",
		ApproverID:  actorID,
		Status:      "pending",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	if err := store.CreateApproval(ctx, approval); err != nil {
		t.Fatalf("seed pending approval: %v", err)
	}
	return run, approval
}

// approvalAct POSTs an approve/reject decision and returns the status code and
// decoded body.
func approvalAct(t *testing.T, approvalID uint64, action, token string) (int, map[string]any) {
	t.Helper()
	resp := httpPostAuth(t, fmt.Sprintf("%s/v1/agent-approvals/%d/%s", testBaseURL, approvalID, action), token, map[string]any{})
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var decoded map[string]any
	_ = json.Unmarshal(body, &decoded)
	return resp.StatusCode, decoded
}

// runStatus reads a run through the public HTTP API.
func runStatus(t *testing.T, token string, runID uint64) string {
	t.Helper()
	resp := httpGet(t, fmt.Sprintf("%s/v1/agent-runs/%d", testBaseURL, runID), token)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get run %d: status %d", runID, resp.StatusCode)
	}
	var run struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	return run.Status
}

// TestAgentApprovalLifecycle exercises the approval decision API end to end:
// the initiating actor approves and the run returns to queued, repeat actions
// and cross-user actions are rejected, and unknown or malformed approval IDs
// map to 404/400.
func TestAgentApprovalLifecycle(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	ctx := context.Background()

	tokenOwner, userOwner := registerAndLogin(t, "approval-owner", "123456")
	tokenOther, _ := registerAndLogin(t, "approval-other", "123456")

	agentID := createPublishedAgent(t, tokenOwner, "approval-agent", "test agent", nil)
	store := agentdata.NewStore(ts.data.DB())

	// Approve: pending approval flips to approved and the run is requeued.
	run, approval := newWaitingApprovalRun(t, store, agentID, userOwner, "p2p_1_2", "evt-approval-approve")
	code, body := approvalAct(t, approval.ID, "approve", tokenOwner)
	if code != http.StatusOK || body["status"] != "approved" {
		t.Fatalf("approve: code=%d body=%v", code, body)
	}
	if status := runStatus(t, tokenOwner, run.ID); status != "queued" {
		t.Fatalf("run after approve: status=%q, want queued", status)
	}
	row, err := store.GetApproval(ctx, approval.ID)
	if err != nil || row.Status != "approved" {
		t.Fatalf("approval row after approve: status=%q err=%v", row.Status, err)
	}
	// A second approve on the same approval conflicts (one-shot transition).
	code, body = approvalAct(t, approval.ID, "approve", tokenOwner)
	if code != http.StatusConflict || body["code"] != "APPROVAL_NOT_PENDING" {
		t.Fatalf("double approve: code=%d body=%v", code, body)
	}

	// Reject: pending approval flips to rejected and the run is requeued too.
	run, approval = newWaitingApprovalRun(t, store, agentID, userOwner, "p2p_1_2", "evt-approval-reject")
	code, body = approvalAct(t, approval.ID, "reject", tokenOwner)
	if code != http.StatusOK || body["status"] != "rejected" {
		t.Fatalf("reject: code=%d body=%v", code, body)
	}
	if status := runStatus(t, tokenOwner, run.ID); status != "queued" {
		t.Fatalf("run after reject: status=%q, want queued", status)
	}
	code, body = approvalAct(t, approval.ID, "reject", tokenOwner)
	if code != http.StatusConflict || body["code"] != "APPROVAL_NOT_PENDING" {
		t.Fatalf("double reject: code=%d body=%v", code, body)
	}

	// A user that did not initiate the run cannot decide, even if the agent
	// is shared with them.
	run, approval = newWaitingApprovalRun(t, store, agentID, userOwner, "p2p_1_2", "evt-approval-forbid")
	code, body = approvalAct(t, approval.ID, "approve", tokenOther)
	if code != http.StatusForbidden {
		t.Fatalf("stranger approve: code=%d body=%v", code, body)
	}
	code, body = approvalAct(t, approval.ID, "reject", tokenOther)
	if code != http.StatusForbidden {
		t.Fatalf("stranger reject: code=%d body=%v", code, body)
	}
	// The decision must still be pending and the run still waiting.
	if row, err := store.GetApproval(ctx, approval.ID); err != nil || row.Status != "pending" {
		t.Fatalf("approval after stranger action: status=%q err=%v", row.Status, err)
	}
	if status := runStatus(t, tokenOwner, run.ID); status != "waiting_approval" {
		t.Fatalf("run after stranger action: status=%q", status)
	}

	// Unknown approval IDs are 404; malformed IDs are 400.
	resp := httpPostAuth(t, fmt.Sprintf("%s/v1/agent-approvals/999999/approve", testBaseURL), tokenOwner, map[string]any{})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("approve missing approval: %d", resp.StatusCode)
	}
	resp = httpPostAuth(t, testBaseURL+"/v1/agent-approvals/abc/approve", tokenOwner, map[string]any{})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("approve malformed approval id: %d", resp.StatusCode)
	}
}

// TestAgentApprovalConcurrentOneShot fires concurrent approve requests at one
// pending approval: exactly one wins, the rest conflict, and the run is
// requeued exactly once.
func TestAgentApprovalConcurrentOneShot(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	ctx := context.Background()

	tokenOwner, userOwner := registerAndLogin(t, "approval-conc-owner", "123456")
	agentID := createPublishedAgent(t, tokenOwner, "approval-conc-agent", "test agent", nil)
	store := agentdata.NewStore(ts.data.DB())

	run, approval := newWaitingApprovalRun(t, store, agentID, userOwner, "p2p_1_2", "evt-approval-concurrent")

	const voters = 8
	var okCount, conflictCount int32
	var wg sync.WaitGroup
	for i := 0; i < voters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, _ := approvalAct(t, approval.ID, "approve", tokenOwner)
			switch code {
			case http.StatusOK:
				atomic.AddInt32(&okCount, 1)
			case http.StatusConflict:
				atomic.AddInt32(&conflictCount, 1)
			default:
				t.Errorf("concurrent approve: unexpected code %d", code)
			}
		}()
	}
	wg.Wait()

	if okCount != 1 || conflictCount != voters-1 {
		t.Fatalf("concurrent approve: ok=%d conflict=%d, want 1/%d", okCount, conflictCount, voters-1)
	}
	row, err := store.GetApproval(ctx, approval.ID)
	if err != nil || row.Status != "approved" {
		t.Fatalf("approval after concurrent approve: status=%q err=%v", row.Status, err)
	}
	if status := runStatus(t, tokenOwner, run.ID); status != "queued" {
		t.Fatalf("run after concurrent approve: status=%q, want queued", status)
	}
	// The waiting-run claim is one-shot as well: re-claiming the requeued run
	// conflicts instead of corrupting its state.
	if err := store.ClaimWaitingRun(ctx, run.ID); !errors.Is(err, domain.ErrRunState) {
		t.Fatalf("second ClaimWaitingRun: err=%v, want ErrRunState", err)
	}
	if got, err := store.GetRun(ctx, run.ID); err != nil || got.Status != "queued" {
		t.Fatalf("run after double claim: status=%q err=%v", got.Status, err)
	}
}

// TestAgentDelete verifies owner-only agent deletion, that detail turns 404,
// and that run/session-scoped data is removed.
func TestAgentDelete(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	ctx := context.Background()

	tokenOwner, userOwner := registerAndLogin(t, "agent-del-owner", "123456")
	tokenOther, _ := registerAndLogin(t, "agent-del-other", "123456")

	agentID := createPublishedAgent(t, tokenOwner, "agent-del", "test agent", nil)
	store := agentdata.NewStore(ts.data.DB())
	run, _ := newWaitingApprovalRun(t, store, agentID, userOwner, "p2p_1_2", "evt-agent-del")

	// Non-owner deletion is refused.
	resp := httpDoAuth(t, http.MethodDelete, fmt.Sprintf("%s/v1/agents/%d", testBaseURL, agentID), tokenOther, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-owner delete: %d", resp.StatusCode)
	}

	// Owner deletes; detail disappears afterwards.
	resp = httpDoAuth(t, http.MethodDelete, fmt.Sprintf("%s/v1/agents/%d", testBaseURL, agentID), tokenOwner, nil)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("owner delete: %d: %s", resp.StatusCode, body)
	}
	resp = httpGet(t, fmt.Sprintf("%s/v1/agents/%d", testBaseURL, agentID), tokenOwner)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("detail after delete: %d, want 404", resp.StatusCode)
	}

	// Hard delete removes the run, its approval and the session-scoped state;
	// IM messages and the bot account are intentionally preserved.
	if _, err := store.GetRun(ctx, run.ID); err == nil {
		t.Fatalf("run %d survived agent delete", run.ID)
	}
	if _, err := store.ApprovalForRun(ctx, run.ID); err != nil {
		t.Fatalf("approval lookup after delete: %v", err)
	}
	var sessions int64
	if err := ts.data.DB().Model(&domain.AgentSession{}).Where("agent_id = ?", agentID).Count(&sessions).Error; err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessions != 0 {
		t.Fatalf("agent sessions survived delete: %d", sessions)
	}
}

// TestBotSendDisabledAgent signs a real agent bot credential and drives
// /v1/message/send with it: an active published agent may send, a disabled
// agent may not, re-enabling restores the ability, and the bot scope rejects
// client message IDs outside the agent prefixes.
func TestBotSendDisabledAgent(t *testing.T) {
	const botSecret = "test-agent-worker-jwt-secret-do-not-use!!"
	t.Setenv("AGENT_WORKER_JWT_SECRET", botSecret)
	ts := setupTestServer(t, false)
	defer ts.stop()

	tokenOwner, _ := registerAndLogin(t, "bot-send-owner", "123456")

	create := httpPostAuth(t, testBaseURL+"/v1/agents", tokenOwner, map[string]any{
		"name":        "bot-send-agent",
		"instruction": "test agent",
		"tools":       []string{"send_message"},
		"modelId":     testModelID,
	})
	createBody, _ := io.ReadAll(create.Body)
	create.Body.Close()
	if create.StatusCode != http.StatusOK {
		t.Fatalf("create agent: %d: %s", create.StatusCode, createBody)
	}
	var created struct {
		ID        int64 `json:"id"`
		BotUserID int64 `json:"botUserId"`
		Draft     struct {
			Revision int64 `json:"revision"`
		} `json:"draft"`
	}
	if err := json.Unmarshal(createBody, &created); err != nil || created.ID == 0 || created.BotUserID == 0 {
		t.Fatalf("decode agent: %v (%s)", err, createBody)
	}
	pub := httpPostAuth(t, fmt.Sprintf("%s/v1/agents/%d/publish", testBaseURL, created.ID), tokenOwner, map[string]any{"revision": created.Draft.Revision})
	pub.Body.Close()
	if pub.StatusCode != http.StatusOK {
		t.Fatalf("publish agent: %d", pub.StatusCode)
	}

	// Share the agent into a group so the bot is a member of the group topic.
	groupID := createGroupWithMembers(t, tokenOwner, "bot-send-group", nil)
	share := httpPutAuth(t, fmt.Sprintf("%s/v1/agents/%d/shares", testBaseURL, created.ID), tokenOwner, map[string]any{"kind": "group", "target": groupID, "enabled": true})
	share.Body.Close()
	if share.StatusCode != http.StatusOK {
		t.Fatalf("group share: %d", share.StatusCode)
	}

	grpTopic := "grp_" + groupID
	botSend := func(clientMsgID string) int {
		t.Helper()
		token, err := credential.Sign([]byte(botSecret), created.ID, created.BotUserID, grpTopic)
		if err != nil {
			t.Fatalf("sign bot token: %v", err)
		}
		resp := httpPostAuth(t, testBaseURL+"/v1/message/send", token, map[string]any{
			"topic":         grpTopic,
			"client_msg_id": clientMsgID,
			"msg_type":      1,
			"content":       base64.StdEncoding.EncodeToString([]byte("hello from bot")),
		})
		resp.Body.Close()
		return resp.StatusCode
	}

	if code := botSend("agent-smoke-1"); code != http.StatusOK {
		t.Fatalf("active agent bot send: %d, want 200", code)
	}

	// Disabling the agent revokes the bot credential even though the agent
	// stays published and the bot remains a group member.
	disable := httpPostAuth(t, fmt.Sprintf("%s/v1/agents/%d/status", testBaseURL, created.ID), tokenOwner, map[string]any{"status": "disabled"})
	disable.Body.Close()
	if disable.StatusCode != http.StatusOK {
		t.Fatalf("disable agent: %d", disable.StatusCode)
	}
	if code := botSend("agent-smoke-2"); code != http.StatusForbidden {
		t.Fatalf("disabled agent bot send: %d, want 403", code)
	}

	// Re-enabling restores the send ability.
	enable := httpPostAuth(t, fmt.Sprintf("%s/v1/agents/%d/status", testBaseURL, created.ID), tokenOwner, map[string]any{"status": "active"})
	enable.Body.Close()
	if enable.StatusCode != http.StatusOK {
		t.Fatalf("re-enable agent: %d", enable.StatusCode)
	}
	if code := botSend("agent-smoke-3"); code != http.StatusOK {
		t.Fatalf("re-enabled agent bot send: %d, want 200", code)
	}

	// The bot scope only covers agent-prefixed client message IDs.
	if code := botSend("user-style-1"); code != http.StatusForbidden {
		t.Fatalf("bot send with non-agent client_msg_id: %d, want 403", code)
	}

	// A bot token for an unknown agent is rejected as well.
	badToken, err := credential.Sign([]byte(botSecret), created.ID+9999, created.BotUserID, grpTopic)
	if err != nil {
		t.Fatalf("sign bad bot token: %v", err)
	}
	resp := httpPostAuth(t, testBaseURL+"/v1/message/send", badToken, map[string]any{
		"topic":         grpTopic,
		"client_msg_id": "agent-smoke-4",
		"msg_type":      1,
		"content":       base64.StdEncoding.EncodeToString([]byte("x")),
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("bot send for unknown agent: %d, want 403", resp.StatusCode)
	}

	// Deleting the agent also revokes the bot credential: the bot account is
	// preserved but inert.
	del := httpDoAuth(t, http.MethodDelete, fmt.Sprintf("%s/v1/agents/%d", testBaseURL, created.ID), tokenOwner, nil)
	del.Body.Close()
	if del.StatusCode != http.StatusOK {
		t.Fatalf("delete agent: %d", del.StatusCode)
	}
	if code := botSend("agent-smoke-5"); code != http.StatusForbidden {
		t.Fatalf("bot send after agent delete: %d, want 403", code)
	}
}
