package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"testing"

	"nonoka-im/internal/agent/app"
	"nonoka-im/internal/agent/tools"
)

// TestAgentToolReadFileAuthorization verifies that the agent read_file tool
// backend enforces file resource authorization: the owner can read the
// attachment, another user cannot.
func TestAgentToolReadFileAuthorization(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	tokenOwner, userOwner := registerAndLogin(t, "tool-file-owner", "123456")
	_, userOther := registerAndLogin(t, "tool-file-other", "123456")

	// Owner uploads a text attachment.
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="notes.txt"`)
	header.Set("Content-Type", "text/plain")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("quarterly report content")); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, testBaseURL+"/v1/files", &buf)
	if err != nil {
		t.Fatalf("create upload request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tokenOwner)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upload file: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload failed: status %d: %s", resp.StatusCode, body)
	}
	var uploaded struct {
		FileID string `json:"file_id"`
	}
	if err := json.Unmarshal(body, &uploaded); err != nil || uploaded.FileID == "" {
		t.Fatalf("decode upload reply failed: %v (%s)", err, body)
	}

	mt := &app.MessageTools{Files: ts.mongoDB, Authorizer: ts.openFGA}
	rc := tools.RunContext{ActorUserID: userOwner, AgentID: 1, BotID: 2, Topic: "p2p_1_2", EventID: "evt-1"}

	name, mime, content, truncated, err := mt.ReadFile(context.Background(), rc, uploaded.FileID, 1<<20)
	if err != nil {
		t.Fatalf("owner read_file denied: %v", err)
	}
	if name != "notes.txt" || mime != "text/plain" || !strings.Contains(string(content), "quarterly report") || truncated {
		t.Fatalf("unexpected read: name=%q mime=%q content=%q truncated=%v", name, mime, content, truncated)
	}

	rc.ActorUserID = userOther
	if _, _, _, _, err := mt.ReadFile(context.Background(), rc, uploaded.FileID, 1<<20); err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("expected file access denied for other user, got %v", err)
	}

	if _, _, _, _, err := mt.ReadFile(context.Background(), rc, "../etc/passwd", 1<<20); err == nil {
		t.Fatalf("path-like file id accepted")
	}
}
