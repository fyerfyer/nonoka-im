package httpclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client posts messages through the Gateway internal HTTP API on behalf of a
// bot. Credentials are supplied by a callback so messaging stays independent
// of the agent run-context helpers.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	Token   func(ctx context.Context) (string, error)
}

type sendRequest struct {
	Topic       string `json:"topic"`
	ClientMsgID string `json:"client_msg_id"`
	MsgType     int32  `json:"msg_type"`
	// Content is base64 because the proto field is bytes and the HTTP codec
	// unmarshals JSON via protojson, which expects base64 for bytes.
	Content string `json:"content"`
}

// Message types mirrored from api/im/v1 without importing the proto into the
// transport helper.
const (
	MsgTypeText      int32 = 0
	MsgTypeAgentCard int32 = 5
)

func (c *Client) Send(ctx context.Context, topic, content, clientID string) error {
	_, err := c.SendWithReceipt(ctx, topic, content, clientID)
	return err
}

func (c *Client) SendCard(ctx context.Context, topic, content, clientID string) (int64, error) {
	return c.send(ctx, topic, content, clientID, MsgTypeAgentCard)
}

func (c *Client) SendWithReceipt(ctx context.Context, topic, content, clientID string) (int64, error) {
	return c.send(ctx, topic, content, clientID, MsgTypeText)
}

func (c *Client) send(ctx context.Context, topic, content, clientID string, msgType int32) (int64, error) {
	if c == nil || c.BaseURL == "" || c.Token == nil {
		return 0, fmt.Errorf("messaging client unavailable")
	}
	body, err := json.Marshal(sendRequest{Topic: topic, ClientMsgID: clientID, MsgType: msgType, Content: base64.StdEncoding.EncodeToString([]byte(content))})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/v1/message/send", strings.NewReader(string(body)))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	token, err := c.Token(ctx)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return 0, fmt.Errorf("send reply status %s", resp.Status)
	}
	var ack struct {
		MsgID int64 `json:"msgId,string"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ack); err != nil {
		return 0, err
	}
	return ack.MsgID, nil
}
