package integration

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
)

// createPublishedAgent creates and publishes an agent through the management
// API and returns its ID.
func createPublishedAgent(t *testing.T, token, name, instruction string, tools []string) int64 {
	t.Helper()
	resp := httpPostAuth(t, testBaseURL+"/v1/agents", token, map[string]any{
		"name":        name,
		"instruction": instruction,
		"tools":       tools,
	})
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create agent failed: status %d: %s", resp.StatusCode, body)
	}
	var created struct {
		ID    int64 `json:"id"`
		Draft struct {
			Revision int64 `json:"revision"`
		} `json:"draft"`
	}
	if err := json.Unmarshal(body, &created); err != nil || created.ID == 0 {
		t.Fatalf("decode agent failed: %v (%s)", err, body)
	}
	pubResp := httpPostAuth(t, fmt.Sprintf("%s/v1/agents/%d/publish", testBaseURL, created.ID), token, map[string]any{"revision": created.Draft.Revision})
	pubResp.Body.Close()
	if pubResp.StatusCode != http.StatusOK {
		t.Fatalf("publish agent %d failed: status %d", created.ID, pubResp.StatusCode)
	}
	return created.ID
}

// TestAuthz_AgentToolExecution verifies that tool execution is authorized in
// OpenFGA: the actor must be able to use the agent, the tool must be bound to
// the agent by a published version, and the actor must access the resource.
func TestAuthz_AgentToolExecution(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	ctx := context.Background()

	tokenOwner, userOwner := registerAndLogin(t, "tool-authz-owner", "123456")
	_, userMember := registerAndLogin(t, "tool-authz-member", "123456")

	agentID := createPublishedAgent(t, tokenOwner, "tool-authz-agent", "test agent", []string{"search_messages"})

	// Publish binds search_messages; the owner may execute it in a topic they
	// participate in.
	p2pTopic := fmt.Sprintf("p2p_%d_%d", userOwner, userMember)
	if err := ts.openFGA.SetTopicParticipant(ctx, p2pTopic, userOwner); err != nil {
		t.Fatalf("establish topic: %v", err)
	}
	if err := ts.openFGA.CanExecuteTool(ctx, userOwner, agentID, "search_messages", p2pTopic, "topic", ""); err != nil {
		t.Fatalf("owner tool execution denied: %v", err)
	}
	// An unbound tool is denied even for the owner.
	if err := ts.openFGA.CanExecuteTool(ctx, userOwner, agentID, "read_file", p2pTopic, "topic", ""); err == nil {
		t.Fatalf("unbound tool allowed")
	}
	// A user without a share cannot execute anything.
	if err := ts.openFGA.CanExecuteTool(ctx, userMember, agentID, "search_messages", p2pTopic, "topic", ""); err == nil {
		t.Fatalf("non-shared user tool execution allowed")
	}
	// Topic scope is enforced: another topic the actor cannot access is denied.
	if err := ts.openFGA.CanExecuteTool(ctx, userOwner, agentID, "search_messages", "grp_missing", "topic", ""); err == nil {
		t.Fatalf("tool execution on inaccessible topic allowed")
	}
	// Resource references must stay on the current topic.
	if err := ts.openFGA.CanExecuteTool(ctx, userOwner, agentID, "search_messages", p2pTopic, "topic", "grp_elsewhere"); err == nil {
		t.Fatalf("tool execution with foreign topic resource allowed")
	}

	// Group revocation: after sharing, a member can execute; removal revokes.
	groupID := createGroupWithMembers(t, tokenOwner, "tool-authz-group", []int64{userMember})
	grpTopic := "grp_" + groupID
	if err := ts.openFGA.SetTopicParticipant(ctx, p2pTopic, userMember); err != nil {
		t.Fatalf("establish member topic: %v", err)
	}
	shareResp := httpPutAuth(t, fmt.Sprintf("%s/v1/agents/%d/shares", testBaseURL, agentID), tokenOwner, map[string]any{"kind": "group", "target": groupID, "enabled": true})
	shareResp.Body.Close()
	if shareResp.StatusCode != http.StatusOK {
		t.Fatalf("group share failed: status %d", shareResp.StatusCode)
	}
	if err := ts.openFGA.CanExecuteTool(ctx, userMember, agentID, "search_messages", grpTopic, "topic", ""); err != nil {
		t.Fatalf("group member tool execution denied: %v", err)
	}
	removeReq, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/v1/groups/%s/members/%d", testBaseURL, groupID, userMember), nil)
	if err != nil {
		t.Fatalf("build remove member request: %v", err)
	}
	removeReq.Header.Set("Authorization", "Bearer "+tokenOwner)
	removeResp, err := http.DefaultClient.Do(removeReq)
	if err != nil {
		t.Fatalf("remove member: %v", err)
	}
	removeResp.Body.Close()
	if removeResp.StatusCode != http.StatusOK {
		t.Fatalf("remove member failed: status %d", removeResp.StatusCode)
	}
	if err := ts.openFGA.CanExecuteTool(ctx, userMember, agentID, "search_messages", grpTopic, "topic", ""); err == nil {
		t.Fatalf("removed member still executes tools")
	}
}

// TestAuthz_FileVisibility verifies the file#can_read paths: owner always,
// explicit viewer via the share endpoint, and topic participants once the
// file is shared into the conversation.
func TestAuthz_FileVisibility(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	tokenOwner, _ := registerAndLogin(t, "file-authz-owner", "123456")
	tokenMember, userMember := registerAndLogin(t, "file-authz-member", "123456")
	tokenViewer, userViewer := registerAndLogin(t, "file-authz-viewer", "123456")

	fileID := uploadTextFile(t, tokenOwner, "visible.txt", "shared content")

	download := func(token string) int {
		t.Helper()
		resp := httpGet(t, testBaseURL+"/v1/files/"+fileID, token)
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := download(tokenOwner); code != http.StatusOK {
		t.Fatalf("owner download: %d", code)
	}
	if code := download(tokenMember); code != http.StatusForbidden {
		t.Fatalf("stranger download before share: %d", code)
	}

	// Owner grants explicit viewer access; revocation takes it away again.
	shareURL := testBaseURL + "/v1/files/" + fileID + "/share"
	grantResp := httpPostAuth(t, shareURL, tokenOwner, map[string]any{"user_ids": []int64{userViewer}, "enabled": true})
	grantResp.Body.Close()
	if grantResp.StatusCode != http.StatusOK {
		t.Fatalf("grant viewer failed: %d", grantResp.StatusCode)
	}
	if code := download(tokenViewer); code != http.StatusOK {
		t.Fatalf("viewer download after grant: %d", code)
	}
	revokeResp := httpPostAuth(t, shareURL, tokenOwner, map[string]any{"user_ids": []int64{userViewer}, "enabled": false})
	revokeResp.Body.Close()
	if revokeResp.StatusCode != http.StatusOK {
		t.Fatalf("revoke viewer failed: %d", revokeResp.StatusCode)
	}
	if code := download(tokenViewer); code != http.StatusForbidden {
		t.Fatalf("viewer download after revoke: %d", code)
	}

	// A non-owner cannot use the share endpoint.
	stealResp := httpPostAuth(t, shareURL, tokenViewer, map[string]any{"user_ids": []int64{userMember}, "enabled": true})
	stealResp.Body.Close()
	if stealResp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-owner share: %d", stealResp.StatusCode)
	}

	// Sharing the file into a group conversation lets group members read it.
	groupID := createGroupWithMembers(t, tokenOwner, "file-authz-group", []int64{userMember})
	sendFileMessage(t, tokenOwner, "grp_"+groupID, fileID)
	if code := download(tokenMember); code != http.StatusOK {
		t.Fatalf("group member download after topic share: %d", code)
	}
}

func uploadTextFile(t *testing.T, token, name, content string) string {
	t.Helper()
	body, contentType := multipartBody(t, name, content)
	req, err := http.NewRequest(http.MethodPost, testBaseURL+"/v1/files", body)
	if err != nil {
		t.Fatalf("create upload request: %v", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload failed: %d: %s", resp.StatusCode, respBody)
	}
	var reply struct {
		FileID string `json:"file_id"`
	}
	if err := json.Unmarshal(respBody, &reply); err != nil || reply.FileID == "" {
		t.Fatalf("decode upload: %v (%s)", err, respBody)
	}
	return reply.FileID
}

func sendFileMessage(t *testing.T, token, topic, fileID string) {
	t.Helper()
	meta, _ := json.Marshal(map[string]any{"url": "/v1/files/" + fileID, "name": "visible.txt", "size": 15, "mime": "text/plain"})
	resp := httpPostAuth(t, testBaseURL+"/v1/message/send", token, map[string]any{
		"topic":         topic,
		"client_msg_id": "file-share-" + fileID,
		"msg_type":      3,
		"content":       base64.StdEncoding.EncodeToString(meta),
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("send file message failed: %d", resp.StatusCode)
	}
}

// createGroupWithMembers creates a group with extra members and returns its ID.
func createGroupWithMembers(t *testing.T, token, name string, memberIDs []int64) string {
	t.Helper()
	resp := httpPostAuth(t, testBaseURL+"/v1/groups", token, map[string]any{"name": name, "member_ids": memberIDs})
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create group failed: %d: %s", resp.StatusCode, body)
	}
	var reply struct {
		GroupID string `json:"groupId"`
	}
	if err := json.Unmarshal(body, &reply); err != nil || reply.GroupID == "" {
		t.Fatalf("decode group: %v (%s)", err, body)
	}
	return reply.GroupID
}

// multipartBody builds a multipart/form-data file upload body.
func multipartBody(t *testing.T, name, content string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	return &buf, writer.FormDataContentType()
}
