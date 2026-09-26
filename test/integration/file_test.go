package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/pkg/sdk"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/encoding/protojson"
)

// uploadFileMultipart POSTs content as multipart form field "file" with the
// given token (empty token omits the Authorization header).
func uploadFileMultipart(t *testing.T, token, filename, mime string, content []byte) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreatePart(map[string][]string{
		"Content-Disposition": {fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename)},
		"Content-Type":        {mime},
	})
	if err != nil {
		t.Fatalf("create multipart part failed: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write multipart content failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer failed: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, testBaseURL+"/v1/files", &buf)
	if err != nil {
		t.Fatalf("create upload request failed: %v", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upload request failed: %v", err)
	}
	return resp
}

// TestFile_UploadDownloadRoundTrip verifies multipart upload with JWT and
// public download return identical bytes with correct metadata.
func TestFile_UploadDownloadRoundTrip(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	_, userID := registerAndLogin(t, "file-roundtrip", "123456")
	token := generateJWTToken(userID, ts.authConf.JwtSecret)

	content := []byte("hello attachment 你好")
	resp := uploadFileMultipart(t, token, "greeting.txt", "text/plain", content)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected upload 200, got %d: %s", resp.StatusCode, body)
	}

	var reply v1.UploadFileReply
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		t.Fatalf("decode upload reply failed: %v", err)
	}
	if reply.FileId == "" || reply.Url == "" {
		t.Fatalf("expected file_id and url in reply, got %+v", &reply)
	}
	if reply.Name != "greeting.txt" || reply.Mime != "text/plain" || reply.Size != int64(len(content)) {
		t.Fatalf("reply metadata mismatch: %+v", &reply)
	}

	// Download: no Authorization header on purpose (public endpoint).
	dl, err := http.Get(testBaseURL + reply.Url)
	if err != nil {
		t.Fatalf("download request failed: %v", err)
	}
	defer dl.Body.Close()
	if dl.StatusCode != http.StatusOK {
		t.Fatalf("expected download 200, got %d", dl.StatusCode)
	}
	got, err := io.ReadAll(dl.Body)
	if err != nil {
		t.Fatalf("read download body failed: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("downloaded content mismatch: got %q, want %q", got, content)
	}
	if ct := dl.Header.Get("Content-Type"); ct != "text/plain" {
		t.Fatalf("expected Content-Type text/plain, got %q", ct)
	}
	if cd := dl.Header.Get("Content-Disposition"); cd == "" {
		t.Fatal("expected Content-Disposition header on download")
	}

	// Missing file -> 404.
	miss, err := http.Get(testBaseURL + "/v1/files/does-not-exist")
	if err != nil {
		t.Fatalf("missing-file request failed: %v", err)
	}
	defer miss.Body.Close()
	if miss.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing file, got %d", miss.StatusCode)
	}

	t.Logf("file round-trip verified: file_id=%s, size=%d", reply.FileId, reply.Size)
}

// TestFile_UploadRequiresAuth verifies uploads without a JWT are rejected.
func TestFile_UploadRequiresAuth(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	resp := uploadFileMultipart(t, "", "x.txt", "text/plain", []byte("data"))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated upload, got %d", resp.StatusCode)
	}

	t.Log("unauthenticated upload rejected")
}

// TestFile_UploadSizeLimit verifies the per-file size cap is enforced.
func TestFile_UploadSizeLimit(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	// Shrink the cap for the test via the exported field.
	const small = 64
	ts.fileSvc.MaxUploadBytes = small

	_, userID := registerAndLogin(t, "file-limit", "123456")
	token := generateJWTToken(userID, ts.authConf.JwtSecret)

	big := make([]byte, small+1)
	resp := uploadFileMultipart(t, token, "big.bin", "application/octet-stream", big)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for oversized upload, got %d", resp.StatusCode)
	}

	// Exactly at the limit still works.
	ok := uploadFileMultipart(t, token, "ok.bin", "application/octet-stream", make([]byte, small))
	defer ok.Body.Close()
	if ok.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(ok.Body)
		t.Fatalf("expected 200 for upload at the limit, got %d: %s", ok.StatusCode, body)
	}

	t.Log("upload size limit verified")
}

// TestFile_MediaMessageEndToEnd verifies an IMAGE message can be sent with
// JSON metadata content, pulled back with msg_type intact, and that the
// conversation preview shows the media placeholder instead of raw JSON.
func TestFile_MediaMessageEndToEnd(t *testing.T) {
	ts := setupTestServer(t, true)
	defer ts.stop()

	ctx := t.Context()
	token1, user1 := registerAndLogin(t, "media-a", "123456")
	token2, user2 := registerAndLogin(t, "media-b", "123456")

	if user1 > user2 {
		user1, user2 = user2, user1
		token1, token2 = token2, token1
	}
	topic := fmt.Sprintf("p2p_%d_%d", user1, user2)

	sender := sdk.NewClient(sdk.Options{
		GatewayURL:        testWSURL,
		Token:             token1,
		DeviceID:          "media-sender",
		HeartbeatInterval: 5 * time.Second,
		RequestTimeout:    15 * time.Second,
	})
	defer sender.Close()
	if err := sender.Connect(ctx); err != nil {
		t.Fatalf("sender connect failed: %v", err)
	}

	meta := fmt.Sprintf(`{"url":"/v1/files/abc123","name":"pic.png","size":123,"mime":"image/png"}`)
	if _, err := sender.SendMessage(ctx, topic, v1.MsgType_MSG_TYPE_IMAGE, []byte(meta)); err != nil {
		t.Fatalf("send image message failed: %v", err)
	}

	// Wait for persistence.
	inboxColl := ts.mongoDB.Collection("inboxes")
	deadline := time.Now().Add(10 * time.Second)
	for {
		count, err := inboxColl.CountDocuments(ctx, bson.M{"user_id": user2, "topic": topic})
		if err != nil {
			t.Fatalf("count inbox failed: %v", err)
		}
		if count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for media message persistence")
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Pull over HTTP and verify msg_type + JSON content survive the pipeline.
	pullURL := fmt.Sprintf("%s/v1/message/pull?topic=%s&lastSeq=0&limit=10&endSeq=0", testBaseURL, topic)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pullURL, nil)
	if err != nil {
		t.Fatalf("create pull request failed: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token2)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("pull request failed: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read pull body failed: %v", err)
	}
	var reply v1.PullReply
	if err := protojson.Unmarshal(body, &reply); err != nil {
		t.Fatalf("decode pull reply failed: %v (%s)", err, body)
	}
	if len(reply.Messages) != 1 {
		t.Fatalf("expected 1 pulled message, got %d", len(reply.Messages))
	}
	msg := reply.Messages[0]
	if msg.MsgType != int32(v1.MsgType_MSG_TYPE_IMAGE) {
		t.Fatalf("expected msg_type=IMAGE(2), got %d", msg.MsgType)
	}
	if string(msg.Content) != meta {
		t.Fatalf("content mismatch: got %q, want %q", string(msg.Content), meta)
	}

	// Conversation preview must show the media placeholder, not raw JSON.
	convReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, testBaseURL+"/v1/conversations", nil)
	convReq.Header.Set("Authorization", "Bearer "+token2)
	convResp, err := http.DefaultClient.Do(convReq)
	if err != nil {
		t.Fatalf("conversations request failed: %v", err)
	}
	defer convResp.Body.Close()
	convBody, _ := io.ReadAll(convResp.Body)
	if !bytes.Contains(convBody, []byte("[图片]")) {
		t.Fatalf("expected [图片] preview in conversations, got %s", convBody)
	}

	t.Log("media message end-to-end verified: msg_type preserved, preview placeholder")
}
