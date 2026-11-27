package botsdk

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWebhookVerifiesAndDeduplicates(t *testing.T) {
	secretBytes := []byte(strings.Repeat("s", 32))
	secret := "whsec_" + base64.StdEncoding.EncodeToString(secretBytes)
	webhook, err := NewWebhook(WebhookConfig{
		Secret: secret, Deduper: NewMemoryDeduper(time.Hour, 100),
	})
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	webhook.OnMention(func(_ context.Context, event *Event) error {
		calls.Add(1)
		if event.Data.Message == nil || event.Data.Message.Content != "hello" {
			return fmt.Errorf("unexpected event")
		}
		return nil
	})
	body, _ := json.Marshal(Event{
		SpecVersion: "1", ID: "evt_1", Type: EventMessageMention,
		Data: EventData{Message: &EventMessage{ID: "9", Topic: "grp_1", Content: "hello"}},
	})
	handler := httptest.NewServer(webhook)
	defer handler.Close()

	send := func(body []byte, eventID string, timestamp int64, key []byte) *http.Response {
		req, _ := http.NewRequest(http.MethodPost, handler.URL, strings.NewReader(string(body)))
		req.Header.Set("webhook-id", eventID)
		req.Header.Set("webhook-timestamp", strconv.FormatInt(timestamp, 10))
		req.Header.Set("webhook-signature", "v1,"+signTest(key, eventID, timestamp, body))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	resp := send(body, "evt_1", time.Now().Unix(), secretBytes)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first delivery status %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = send(body, "evt_1", time.Now().Unix(), secretBytes)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("duplicate delivery status %d", resp.StatusCode)
	}
	resp.Body.Close()
	if calls.Load() != 1 {
		t.Fatalf("expected one handler call, got %d", calls.Load())
	}

	resp = send(body, "evt_2", time.Now().Add(-10*time.Minute).Unix(), secretBytes)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("stale delivery status %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = send(body, "evt_3", time.Now().Unix(), []byte(strings.Repeat("x", 32)))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad signature status %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestWebhookRejectsInFlightDuplicateWithoutAck(t *testing.T) {
	secretBytes := []byte(strings.Repeat("s", 32))
	secret := "whsec_" + base64.StdEncoding.EncodeToString(secretBytes)
	webhook, err := NewWebhook(WebhookConfig{Secret: secret, Deduper: NewMemoryDeduper(time.Hour, 100)})
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	webhook.OnMention(func(context.Context, *Event) error {
		close(started)
		<-release
		return nil
	})
	body, _ := json.Marshal(Event{
		SpecVersion: "1", ID: "evt_inflight", Type: EventMessageMention,
		Data: EventData{Message: &EventMessage{ID: "10", Topic: "grp_1", Content: "hello"}},
	})
	handler := httptest.NewServer(webhook)
	defer handler.Close()

	send := func() *http.Response {
		timestamp := time.Now().Unix()
		req, _ := http.NewRequest(http.MethodPost, handler.URL, strings.NewReader(string(body)))
		req.Header.Set("webhook-id", "evt_inflight")
		req.Header.Set("webhook-timestamp", strconv.FormatInt(timestamp, 10))
		req.Header.Set("webhook-signature", "v1,"+signTest(secretBytes, "evt_inflight", timestamp, body))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Error(err)
			return nil
		}
		return resp
	}

	firstDone := make(chan *http.Response, 1)
	go func() { firstDone <- send() }()
	<-started
	second := send()
	if second == nil {
		t.Fatal("duplicate request failed")
	}
	if second.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("in-flight duplicate status = %d, want 503", second.StatusCode)
	}
	second.Body.Close()
	close(release)
	first := <-firstDone
	if first == nil {
		t.Fatal("first request failed")
	}
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first delivery status = %d, want 200", first.StatusCode)
	}
	first.Body.Close()
}

func TestClientSendRetriesWithStableIDAndReceipt(t *testing.T) {
	var calls atomic.Int32
	var firstID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/bot/messages":
			var body SendRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			if firstID == "" {
				firstID = body.ClientMsgID
			} else if firstID != body.ClientMsgID {
				t.Errorf("client message id changed: %s != %s", firstID, body.ClientMsgID)
			}
			if calls.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"code":"DELIVERY_UNAVAILABLE"}`))
				return
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"client_msg_id":"` + body.ClientMsgID + `","accepted_at":"2026-11-26T00:00:00Z"}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/bot/messages/by-client-id/"):
			_, _ = w.Write([]byte(`{"messageId":"42","topic":"p2p_1_2","topicSeq":"7","senderId":"3","type":"text","content":"hi"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := New(Config{
		BaseURL: server.URL, Token: "nb_test", ReceiptTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.SendText(context.Background(), "p2p_1_2", "hi", WithClientMsgID("stable-1"))
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected two send attempts, got %d", calls.Load())
	}
	if result.MessageID != "42" || result.TopicSeq != "7" || result.ClientMsgID != "stable-1" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestStreamFlushAndFinalize(t *testing.T) {
	var chunks int
	var finalized bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/bot/streams":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"stream_id":"bst_1","client_msg_id":"stream-1"}`))
		case strings.HasSuffix(r.URL.Path, "/chunks"):
			chunks++
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"stream_id":"bst_1","seq":1}`))
		case strings.HasSuffix(r.URL.Path, "/finalize"):
			finalized = true
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"client_msg_id":"stream-1","accepted_at":"2026-11-26T00:00:00Z"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Token: "nb_test"})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.NewStream(context.Background(), "p2p_1_2", WithClientMsgID("stream-1"))
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Write(context.Background(), strings.Repeat("a", 300)); err != nil {
		t.Fatal(err)
	}
	if chunks != 1 {
		t.Fatalf("expected one auto-flushed chunk, got %d", chunks)
	}
	if _, err := stream.Finalize(context.Background(), "final"); err != nil {
		t.Fatal(err)
	}
	if !finalized {
		t.Fatal("finalize was not called")
	}
}

func signTest(secret []byte, eventID string, timestamp int64, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(eventID + "." + strconv.FormatInt(timestamp, 10) + "."))
	_, _ = mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
