package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
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
// authenticated download returns identical bytes with correct metadata.
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

	unauthenticated, err := http.Get(testBaseURL + reply.Url)
	if err != nil {
		t.Fatalf("unauthenticated download request failed: %v", err)
	}
	unauthenticated.Body.Close()
	if unauthenticated.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated download 401, got %d", unauthenticated.StatusCode)
	}
	req, err := http.NewRequest(http.MethodGet, testBaseURL+reply.Url, nil)
	if err != nil {
		t.Fatalf("create download request failed: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	dl, err := http.DefaultClient.Do(req)
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
	missingReq, err := http.NewRequest(http.MethodGet, testBaseURL+"/v1/files/does-not-exist", nil)
	if err != nil {
		t.Fatalf("create missing-file request failed: %v", err)
	}
	missingReq.Header.Set("Authorization", "Bearer "+token)
	miss, err := http.DefaultClient.Do(missingReq)
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

// TestFile_SignedMediaURL verifies short-TTL media URLs work for <img> without
// an Authorization header, while still re-checking file authorization.
func TestFile_SignedMediaURL(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	token1, user1 := registerAndLogin(t, "signed-a", "123456")
	token2, user2 := registerAndLogin(t, "signed-b", "123456")
	_ = user1
	content := []byte("signed media bytes")
	up := uploadFileMultipart(t, token1, "pic.png", "image/png", content)
	defer up.Body.Close()
	if up.StatusCode != http.StatusOK {
		t.Fatalf("upload: %d", up.StatusCode)
	}
	var uploaded v1.UploadFileReply
	if err := json.NewDecoder(up.Body).Decode(&uploaded); err != nil {
		t.Fatalf("decode upload: %v", err)
	}

	signResp := httpPostAuth(t, testBaseURL+"/v1/files/"+uploaded.FileId+"/sign", token1, map[string]any{})
	signBody, _ := io.ReadAll(signResp.Body)
	signResp.Body.Close()
	if signResp.StatusCode != http.StatusOK {
		t.Fatalf("sign: %d %s", signResp.StatusCode, signBody)
	}
	var signed struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(signBody, &signed); err != nil || signed.URL == "" {
		t.Fatalf("decode signed url: %v %s", err, signBody)
	}

	// No Authorization header: this is the browser <img> path.
	resp, err := http.Get(testBaseURL + signed.URL)
	if err != nil {
		t.Fatalf("signed GET: %v", err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !bytes.Equal(got, content) {
		t.Fatalf("signed GET: %d %q", resp.StatusCode, got)
	}

	// A recipient without a share cannot mint a URL.
	deny := httpPostAuth(t, testBaseURL+"/v1/files/"+uploaded.FileId+"/sign", token2, map[string]any{})
	deny.Body.Close()
	if deny.StatusCode != http.StatusForbidden {
		t.Fatalf("non-recipient sign: %d", deny.StatusCode)
	}
	share := httpPostAuth(t, testBaseURL+"/v1/files/"+uploaded.FileId+"/share", token1, map[string]any{
		"user_ids": []int64{user2}, "enabled": true,
	})
	share.Body.Close()
	if share.StatusCode != http.StatusOK {
		t.Fatalf("share: %d", share.StatusCode)
	}
	allow := httpPostAuth(t, testBaseURL+"/v1/files/"+uploaded.FileId+"/sign", token2, map[string]any{})
	allowBody, _ := io.ReadAll(allow.Body)
	allow.Body.Close()
	if allow.StatusCode != http.StatusOK {
		t.Fatalf("recipient sign after share: %d", allow.StatusCode)
	}
	var recipientSigned struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(allowBody, &recipientSigned); err != nil || recipientSigned.URL == "" {
		t.Fatalf("decode recipient signed url: %v %s", err, allowBody)
	}

	// Tampering with the signed uid must be rejected.
	parsed, err := url.Parse(testBaseURL + recipientSigned.URL)
	if err != nil {
		t.Fatalf("parse signed url: %v", err)
	}
	q := parsed.Query()
	q.Set("uid", fmt.Sprintf("%d", user1))
	parsed.RawQuery = q.Encode()
	tampered, err := http.Get(parsed.String())
	if err != nil {
		t.Fatalf("tampered GET: %v", err)
	}
	tampered.Body.Close()
	if tampered.StatusCode != http.StatusUnauthorized {
		t.Fatalf("tampered signature: %d", tampered.StatusCode)
	}

	// Revoking the share invalidates already-minted URLs on download.
	revoke := httpPostAuth(t, testBaseURL+"/v1/files/"+uploaded.FileId+"/share", token1, map[string]any{
		"user_ids": []int64{user2}, "enabled": false,
	})
	revoke.Body.Close()
	if revoke.StatusCode != http.StatusOK {
		t.Fatalf("revoke share: %d", revoke.StatusCode)
	}
	afterRevoke, err := http.Get(testBaseURL + recipientSigned.URL)
	if err != nil {
		t.Fatalf("revoked share GET: %v", err)
	}
	afterRevoke.Body.Close()
	if afterRevoke.StatusCode != http.StatusForbidden {
		t.Fatalf("revoked share signed url: %d", afterRevoke.StatusCode)
	}

	// Expired signatures are rejected.
	ts.fileSvc.SignedTTL = -time.Second
	expired := httpPostAuth(t, testBaseURL+"/v1/files/"+uploaded.FileId+"/sign", token1, map[string]any{})
	var expiredSigned struct {
		URL string `json:"url"`
	}
	_ = json.NewDecoder(expired.Body).Decode(&expiredSigned)
	expired.Body.Close()
	exp, err := http.Get(testBaseURL + expiredSigned.URL)
	if err != nil {
		t.Fatalf("expired GET: %v", err)
	}
	exp.Body.Close()
	if exp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired signed url: %d", exp.StatusCode)
	}
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

	sender := newTestSDKClient(t, token1, "media-sender")
	defer sender.Close()
	if err := sender.Connect(ctx); err != nil {
		t.Fatalf("sender connect failed: %v", err)
	}

	meta := `{"url":"/v1/files/abc123","name":"pic.png","size":123,"mime":"image/png"}`
	if _, err := sender.Messages.Send(ctx, sdk.SendRequest{Topic: topic, Type: sdk.MessageTypeImage, Content: []byte(meta)}); err != nil {
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
