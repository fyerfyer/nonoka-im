package integration

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	botdomain "nonoka-im/internal/bot/domain"
	imdomain "nonoka-im/internal/im/domain"
	"nonoka-im/internal/messaging"
)

type botTestBot struct {
	ID        string `json:"id"`
	BotUserID string `json:"botUserId"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	Status    string `json:"status"`
}

type botTestToken struct {
	Token struct {
		ID     string `json:"id"`
		Prefix string `json:"prefix"`
	} `json:"token"`
	Secret string `json:"secret"`
}

func TestBotAPI_CreateSendAndStream(t *testing.T) {
	ts := setupTestServer(t, true)
	defer ts.stop()
	ctx := t.Context()

	userToken, userID := registerAndLogin(t, "bot-owner", "123456")
	bot := createTestBot(t, userToken, "Support Bot")
	botUserID := mustInt64(t, bot.BotUserID)
	topicName := p2pTopic(userID, botUserID)
	if err := ts.openFGA.EnsureTopicForSend(ctx, userID, topicName); err != nil {
		t.Fatalf("ensure topic: %v", err)
	}
	token := createTestBotToken(t, userToken, bot.ID)

	// Send a reply through the bot runtime API.
	resp := httpPostAuth(t, testBaseURL+"/v1/bot/messages", token, map[string]any{
		"topic": topicName, "client_msg_id": "bot-send-1", "content": "hello from the bot",
	})
	assertStatusCode(t, resp, http.StatusAccepted)
	resp.Body.Close()

	var receipt *http.Response
	for attempt := 0; attempt < 100; attempt++ {
		receipt = httpGet(t, testBaseURL+"/v1/bot/messages/by-client-id/bot-send-1?topic="+topicName, token)
		if receipt.StatusCode == http.StatusOK {
			break
		}
		receipt.Body.Close()
		time.Sleep(200 * time.Millisecond)
	}
	assertStatusCode(t, receipt, http.StatusOK)
	var receiptBody struct {
		MessageID string `json:"messageId"`
		TopicSeq  string `json:"topicSeq"`
		Content   string `json:"content"`
	}
	if err := json.NewDecoder(receipt.Body).Decode(&receiptBody); err != nil {
		t.Fatalf("decode receipt: %v", err)
	}
	receipt.Body.Close()
	if receiptBody.MessageID == "" || receiptBody.Content != "hello from the bot" {
		t.Fatalf("unexpected receipt: %+v", receiptBody)
	}

	// Streaming reply persists only the final message.
	streamResp := httpPostAuth(t, testBaseURL+"/v1/bot/streams", token, map[string]any{
		"topic": topicName, "client_msg_id": "bot-stream-1",
	})
	assertStatusCode(t, streamResp, http.StatusCreated)
	var streamBody struct {
		StreamID string `json:"stream_id"`
	}
	if err := json.NewDecoder(streamResp.Body).Decode(&streamBody); err != nil {
		t.Fatalf("decode stream: %v", err)
	}
	streamResp.Body.Close()
	for seq, delta := range []string{"Here ", "is the answer."} {
		chunk := httpPostAuth(t, testBaseURL+"/v1/bot/streams/"+streamBody.StreamID+"/chunks", token, map[string]any{
			"seq": seq + 1, "delta": delta,
		})
		assertStatusCode(t, chunk, http.StatusAccepted)
		chunk.Body.Close()
	}
	final := httpPostAuth(t, testBaseURL+"/v1/bot/streams/"+streamBody.StreamID+"/finalize", token, map[string]any{
		"content": "Here is the answer.",
	})
	assertStatusCode(t, final, http.StatusAccepted)
	final.Body.Close()

	var streamReceipt *http.Response
	for attempt := 0; attempt < 100; attempt++ {
		streamReceipt = httpGet(t, testBaseURL+"/v1/bot/messages/by-client-id/bot-stream-1?topic="+topicName, token)
		if streamReceipt.StatusCode == http.StatusOK {
			break
		}
		streamReceipt.Body.Close()
		time.Sleep(200 * time.Millisecond)
	}
	assertStatusCode(t, streamReceipt, http.StatusOK)
	streamReceipt.Body.Close()

	// Revoking the token must fail the next request immediately.
	revoke := httpDeleteAuth(t, testBaseURL+"/v1/bots/"+bot.ID+"/tokens/"+tokenID(t, userToken, bot.ID), userToken)
	assertStatusCode(t, revoke, http.StatusOK)
	revoke.Body.Close()
	unauthorized := httpGet(t, testBaseURL+"/v1/bot/me", token)
	assertStatusCode(t, unauthorized, http.StatusUnauthorized)
	unauthorized.Body.Close()
}

func TestBotDelivery_WebhookSignatureAndIdempotency(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	ctx := t.Context()

	userToken, userID := registerAndLogin(t, "bot-owner-delivery", "123456")
	bot := createTestBot(t, userToken, "Delivery Bot")
	botUserID := mustInt64(t, bot.BotUserID)
	topicName := p2pTopic(userID, botUserID)
	if err := ts.openFGA.EnsureTopicForSend(ctx, userID, topicName); err != nil {
		t.Fatalf("ensure topic: %v", err)
	}

	type captured struct {
		headers http.Header
		body    []byte
	}
	received := make(chan captured, 4)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- captured{headers: r.Header.Clone(), body: body}
		w.WriteHeader(http.StatusOK)
	}))
	defer hook.Close()

	secret := setTestWebhook(t, userToken, bot.ID, hook.URL, []string{botdomain.EventMessageDirect})

	// Persist a human DM and run the dispatcher exactly as bot-worker does.
	result, err := ts.messageApp.HandleIncoming(ctx, imdomain.Message{
		Topic: topicName, SenderID: userID, Type: imdomain.MessageTypeText,
		Content: []byte("hello delivery"), Timestamp: time.Now().Unix(), ClientMsgID: "human-dm-1",
	})
	if err != nil {
		t.Fatalf("persist dm: %v", err)
	}
	event := messaging.NewMessageEvent(result.Message.ID, topicName, userID, 1, result.Message.Timestamp, nil, "human-dm-1")
	if err := ts.botDelivery.Dispatch(ctx, event); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if err := ts.botDelivery.Dispatch(ctx, event); err != nil {
		t.Fatalf("dispatch duplicate: %v", err)
	}
	var deliveries int64
	if err := ts.data.DB().Raw("SELECT count(*) FROM bot_deliveries WHERE bot_id = ?", bot.ID).Scan(&deliveries).Error; err != nil {
		t.Fatalf("count deliveries: %v", err)
	}
	if deliveries != 1 {
		t.Fatalf("expected one idempotent delivery row, got %d", deliveries)
	}
	worked, err := ts.botDelivery.DeliverNext(ctx)
	if err != nil || !worked {
		t.Fatalf("deliver next: worked=%v err=%v", worked, err)
	}

	select {
	case got := <-received:
		if got.headers.Get("webhook-id") == "" || got.headers.Get("webhook-signature") == "" {
			t.Fatalf("missing webhook headers: %v", got.headers)
		}
		if !verifyTestSignature(secret, got.headers, got.body) {
			t.Fatalf("invalid signature")
		}
		var eventBody struct {
			ID   string `json:"id"`
			Type string `json:"type"`
			Data struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"data"`
		}
		if err := json.Unmarshal(got.body, &eventBody); err != nil {
			t.Fatalf("decode event: %v", err)
		}
		if eventBody.Type != botdomain.EventMessageDirect || eventBody.Data.Message.Content != "hello delivery" {
			t.Fatalf("unexpected event: %s", got.body)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("webhook was not delivered")
	}
}

func createTestBot(t *testing.T, userToken, name string) botTestBot {
	t.Helper()
	resp := httpPostAuth(t, testBaseURL+"/v1/bots", userToken, map[string]any{"name": name})
	assertStatusCode(t, resp, http.StatusCreated)
	defer resp.Body.Close()
	var bot botTestBot
	if err := json.NewDecoder(resp.Body).Decode(&bot); err != nil {
		t.Fatalf("decode bot: %v", err)
	}
	if bot.ID == "" || bot.BotUserID == "" {
		t.Fatalf("invalid bot response: %+v", bot)
	}
	return bot
}

func createTestBotToken(t *testing.T, userToken, botID string) string {
	t.Helper()
	resp := httpPostAuth(t, testBaseURL+"/v1/bots/"+botID+"/tokens", userToken, map[string]any{"name": "test"})
	assertStatusCode(t, resp, http.StatusCreated)
	defer resp.Body.Close()
	var token botTestToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		t.Fatalf("decode token: %v", err)
	}
	if token.Secret == "" {
		t.Fatal("token secret was not returned once")
	}
	return token.Secret
}

func tokenID(t *testing.T, userToken, botID string) string {
	t.Helper()
	resp := httpGet(t, testBaseURL+"/v1/bots/"+botID+"/tokens", userToken)
	assertStatusCode(t, resp, http.StatusOK)
	defer resp.Body.Close()
	var body struct {
		Tokens []struct {
			ID string `json:"id"`
		} `json:"tokens"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode tokens: %v", err)
	}
	if len(body.Tokens) == 0 {
		t.Fatal("no tokens")
	}
	return body.Tokens[0].ID
}

func setTestWebhook(t *testing.T, userToken, botID, url string, events []string) string {
	t.Helper()
	resp := httpPutAuth(t, testBaseURL+"/v1/bots/"+botID+"/webhook", userToken, map[string]any{
		"url": url, "events": events, "enabled": true,
	})
	assertStatusCode(t, resp, http.StatusOK)
	defer resp.Body.Close()
	var body struct {
		Secret string `json:"secret"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode webhook: %v", err)
	}
	if body.Secret == "" {
		t.Fatal("webhook secret was not returned once")
	}
	return body.Secret
}

func verifyTestSignature(secret string, headers http.Header, body []byte) bool {
	raw := secret
	if len(raw) > len("whsec_") && raw[:len("whsec_")] == "whsec_" {
		raw = raw[len("whsec_"):]
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(headers.Get("webhook-id") + "." + headers.Get("webhook-timestamp") + "."))
	mac.Write(body)
	expected := mac.Sum(nil)
	header := headers.Get("webhook-signature")
	if len(header) < 4 || header[:3] != "v1," {
		return false
	}
	got, err := base64.StdEncoding.DecodeString(header[3:])
	if err != nil {
		return false
	}
	return hmac.Equal(expected, got)
}

func p2pTopic(a, b int64) string {
	if a > b {
		a, b = b, a
	}
	return fmt.Sprintf("p2p_%d_%d", a, b)
}

func mustInt64(t *testing.T, raw string) int64 {
	t.Helper()
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		t.Fatalf("invalid int64 %q: %v", raw, err)
	}
	return value
}
