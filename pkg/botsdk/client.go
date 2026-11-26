// Package botsdk is the reference Go SDK for Nonoka external bots.
//
// It provides a typed HTTP client, a Standard Webhooks-compatible handler and
// a streaming reply helper. It intentionally depends only on the public HTTP
// contract in docs/bot-protocol.md.
package botsdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultUserAgent = "nonoka-botsdk-go/1"

type Config struct {
	BaseURL string
	Token   string
	// HTTPClient is optional. The SDK uses a 30s client by default.
	HTTPClient *http.Client
	// ReceiptTimeout waits for the durable message id after Send. Zero
	// disables the wait and returns the accepted client message id immediately.
	ReceiptTimeout time.Duration
	UserAgent      string
}

type Client struct {
	baseURL        string
	token          string
	http           *http.Client
	userAgent      string
	receiptTimeout time.Duration
}

func New(cfg Config) (*Client, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("botsdk: BaseURL is required")
	}
	if _, err := url.Parse(base); err != nil {
		return nil, fmt.Errorf("botsdk: invalid BaseURL: %w", err)
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, fmt.Errorf("botsdk: Token is required")
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	agent := cfg.UserAgent
	if agent == "" {
		agent = defaultUserAgent
	}
	return &Client{
		baseURL: base, token: strings.TrimSpace(cfg.Token), http: httpClient,
		userAgent: agent, receiptTimeout: cfg.ReceiptTimeout,
	}, nil
}

type Bot struct {
	ID       string `json:"id"`
	UserID   string `json:"botUserId"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

type SendRequest struct {
	Topic            string   `json:"topic"`
	ClientMsgID      string   `json:"client_msg_id,omitempty"`
	Content          string   `json:"content"`
	MentionedUserIDs []string `json:"mentioned_user_ids,omitempty"`
}

type SendResult struct {
	ClientMsgID string    `json:"client_msg_id"`
	MessageID   string    `json:"message_id,omitempty"`
	TopicSeq    string    `json:"topic_seq,omitempty"`
	AcceptedAt  time.Time `json:"-"`
}

type Message struct {
	MessageID string `json:"messageId"`
	Topic     string `json:"topic"`
	TopicSeq  string `json:"topicSeq"`
	SenderID  string `json:"senderId"`
	Type      string `json:"type"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`
}

type SendOption func(*sendOptions)

type sendOptions struct {
	clientMsgID string
	mentions    []string
	wait        *time.Duration
}

func WithClientMsgID(id string) SendOption {
	return func(o *sendOptions) { o.clientMsgID = id }
}

func WithMentions(ids ...string) SendOption {
	return func(o *sendOptions) { o.mentions = append(o.mentions, ids...) }
}

func WithWait(timeout time.Duration) SendOption {
	return func(o *sendOptions) { o.wait = &timeout }
}

// Me returns the bot identity for the configured token.
func (c *Client) Me(ctx context.Context) (*Bot, error) {
	var response struct {
		Bot Bot `json:"bot"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/v1/bot/me", nil, &response); err != nil {
		return nil, err
	}
	return &response.Bot, nil
}

// Send posts one text message. It retries retryable failures with the same
// client message id, then optionally waits for the durable receipt.
func (c *Client) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	if strings.TrimSpace(req.Topic) == "" || strings.TrimSpace(req.Content) == "" {
		return SendResult{}, fmt.Errorf("botsdk: topic and content are required")
	}
	if strings.TrimSpace(req.ClientMsgID) == "" {
		req.ClientMsgID = newClientMsgID()
	}
	var accepted struct {
		ClientMsgID string `json:"client_msg_id"`
		AcceptedAt  string `json:"accepted_at"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/v1/bot/messages", req, &accepted); err != nil {
		return SendResult{}, err
	}
	result := SendResult{ClientMsgID: accepted.ClientMsgID}
	if result.ClientMsgID == "" {
		result.ClientMsgID = req.ClientMsgID
	}
	if parsed, err := time.Parse(time.RFC3339Nano, accepted.AcceptedAt); err == nil {
		result.AcceptedAt = parsed
	}
	if c.receiptTimeout <= 0 {
		return result, nil
	}
	receipt, err := c.waitReceipt(ctx, req.Topic, result.ClientMsgID, c.receiptTimeout)
	if err != nil {
		// The message was accepted; a missing receipt is not a send failure.
		return result, nil
	}
	if receipt != nil {
		result.MessageID = receipt.MessageID
		result.TopicSeq = receipt.TopicSeq
	}
	return result, nil
}

func (c *Client) SendText(ctx context.Context, topic, text string, opts ...SendOption) (SendResult, error) {
	options := sendOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	req := SendRequest{Topic: topic, Content: text, ClientMsgID: options.clientMsgID, MentionedUserIDs: options.mentions}
	if options.wait != nil {
		receipt, err := c.sendAndWait(ctx, req, *options.wait)
		return receipt, err
	}
	return c.Send(ctx, req)
}

func (c *Client) sendAndWait(ctx context.Context, req SendRequest, wait time.Duration) (SendResult, error) {
	if req.ClientMsgID == "" {
		req.ClientMsgID = newClientMsgID()
	}
	var accepted struct {
		ClientMsgID string `json:"client_msg_id"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/v1/bot/messages", req, &accepted); err != nil {
		return SendResult{}, err
	}
	result := SendResult{ClientMsgID: req.ClientMsgID}
	receipt, err := c.waitReceipt(ctx, req.Topic, req.ClientMsgID, wait)
	if err != nil || receipt == nil {
		return result, err
	}
	result.MessageID = receipt.MessageID
	result.TopicSeq = receipt.TopicSeq
	return result, nil
}

// MessageByClientID resolves a previously accepted send. A nil message with a
// nil error means the message is still pending.
func (c *Client) MessageByClientID(ctx context.Context, topic, clientMsgID string, wait time.Duration) (*Message, error) {
	path := "/v1/bot/messages/by-client-id/" + url.PathEscape(clientMsgID)
	query := url.Values{}
	query.Set("topic", topic)
	if wait > 0 {
		query.Set("wait", wait.String())
	}
	path += "?" + query.Encode()
	var response struct {
		MessageID   string `json:"messageId"`
		Topic       string `json:"topic"`
		TopicSeq    string `json:"topicSeq"`
		SenderID    string `json:"senderId"`
		Type        string `json:"type"`
		Content     string `json:"content"`
		CreatedAt   string `json:"createdAt"`
		ClientMsgID string `json:"client_msg_id"`
		Status      string `json:"status"`
	}
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusAccepted {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, decodeAPIError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	return &Message{
		MessageID: response.MessageID, Topic: response.Topic, TopicSeq: response.TopicSeq,
		SenderID: response.SenderID, Type: response.Type, Content: response.Content,
		CreatedAt: response.CreatedAt,
	}, nil
}

func (c *Client) waitReceipt(ctx context.Context, topic, clientMsgID string, timeout time.Duration) (*Message, error) {
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, nil
		}
		wait := remaining
		if wait > 3*time.Second {
			wait = 3 * time.Second
		}
		message, err := c.MessageByClientID(ctx, topic, clientMsgID, wait)
		if err != nil {
			return nil, err
		}
		if message != nil {
			return message, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// History returns a backward page of messages visible to the bot.
func (c *Client) History(ctx context.Context, topic string, limit int, beforeSeq uint64) ([]Message, error) {
	query := url.Values{}
	query.Set("topic", topic)
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	if beforeSeq > 0 {
		query.Set("before_seq", strconv.FormatUint(beforeSeq, 10))
	}
	var response struct {
		Messages []Message `json:"messages"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/v1/bot/messages?"+query.Encode(), nil, &response); err != nil {
		return nil, err
	}
	return response.Messages, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, in, out any) error {
	var rawBody []byte
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		rawBody = raw
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		var body io.Reader
		if rawBody != nil {
			body = bytes.NewReader(rawBody)
		}
		resp, err := c.do(ctx, method, path, body)
		if err != nil {
			lastErr = err
			if !sleepBackoff(ctx, attempt) {
				return err
			}
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = decodeAPIError(resp)
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
			resp.Body.Close()
			if !sleepRetry(ctx, attempt, retryAfter) {
				return lastErr
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			err := decodeAPIError(resp)
			resp.Body.Close()
			return err
		}
		defer resp.Body.Close()
		if out == nil {
			return nil
		}
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return lastErr
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.http.Do(req)
}

type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Retryable  bool
	RetryAfter time.Duration
	RequestID  string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("botsdk: %s (%d): %s", e.Code, e.StatusCode, e.Message)
}

func decodeAPIError(resp *http.Response) error {
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var body struct {
		Code       string `json:"code"`
		Message    string `json:"message"`
		Retryable  bool   `json:"retryable"`
		RequestID  string `json:"requestId"`
		RetryAfter int64  `json:"retryAfterMs"`
	}
	_ = json.Unmarshal(raw, &body)
	if body.Message == "" {
		body.Message = strings.TrimSpace(string(raw))
	}
	return &APIError{
		StatusCode: resp.StatusCode, Code: body.Code, Message: body.Message,
		Retryable: body.Retryable, RequestID: body.RequestID,
		RetryAfter: time.Duration(body.RetryAfter) * time.Millisecond,
	}
}

func parseRetryAfter(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(raw); err == nil {
		if d := time.Until(when); d > 0 {
			return d
		}
	}
	return 0
}

func sleepBackoff(ctx context.Context, attempt int) bool {
	return sleep(ctx, time.Duration(200*(1<<attempt))*time.Millisecond)
}

func sleepRetry(ctx context.Context, attempt int, retryAfter time.Duration) bool {
	delay := time.Duration(500*(1<<attempt)) * time.Millisecond
	if retryAfter > delay {
		delay = retryAfter
	}
	if delay > 10*time.Second {
		delay = 10 * time.Second
	}
	return sleep(ctx, delay)
}

func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func newClientMsgID() string {
	return "bot-" + strconv.FormatInt(time.Now().UnixNano(), 36) + "-" + randHex(6)
}
