// Package transport is the SDK's single HTTP transport. It owns token
// injection, 401 refresh, retry policy, typed error decoding and proto JSON
// bodies for both generated and hand-written API surfaces.
package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// TokenSource supplies access tokens. Implementations should be safe for
// concurrent use and refresh proactively.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// RefreshableTokenSource can rotate an expired access token without user
// interaction. HTTP transports retry once after a successful refresh.
type RefreshableTokenSource interface {
	TokenSource
	Refresh(ctx context.Context) (string, error)
}

// APIError is the typed error returned for HTTP API failures and WebSocket
// error packets. It never wraps internal protobuf or Kratos types.
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
		return "<nil>"
	}
	if e.Code != "" {
		return fmt.Sprintf("sdk: %s (%d): %s", e.Code, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("sdk: http %d: %s", e.StatusCode, e.Message)
}

// JSONInt64 accepts both protojson strings ("123") and plain JSON numbers.
type JSONInt64 int64

func (n *JSONInt64) UnmarshalJSON(data []byte) error {
	raw := strings.Trim(string(data), `"`)
	if raw == "" || raw == "null" {
		*n = 0
		return nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return err
	}
	*n = JSONInt64(value)
	return nil
}

// Client sends SDK HTTP requests through one retry/refresh/error pipeline.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	Tokens     TokenSource
	UserAgent  string

	tokenMu sync.RWMutex
}

// SetTokenSource atomically swaps the source used by in-flight requests.
func (t *Client) SetTokenSource(source TokenSource) {
	t.tokenMu.Lock()
	t.Tokens = source
	t.tokenMu.Unlock()
}

func (t *Client) tokenSource() TokenSource {
	t.tokenMu.RLock()
	source := t.Tokens
	t.tokenMu.RUnlock()
	return source
}

func New(baseURL string, client *http.Client, tokens TokenSource, userAgent string) *Client {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"), HTTPClient: client,
		Tokens: tokens, UserAgent: userAgent,
	}
}

// Do sends an arbitrary HTTP request (multipart uploads, downloads, signed
// URLs) through the same token injection and 401-refresh pipeline as JSON and
// proto calls. The caller owns the response body.
func (t *Client) Do(req *http.Request) (*http.Response, error) {
	do := func(token string) (*http.Response, error) {
		clone := req.Clone(req.Context())
		if clone.Header == nil {
			clone.Header = http.Header{}
		}
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			clone.Body = body
		}
		if token != "" {
			clone.Header.Set("Authorization", "Bearer "+token)
		}
		if t.UserAgent != "" {
			clone.Header.Set("User-Agent", t.UserAgent)
		}
		return t.HTTPClient.Do(clone)
	}
	token, err := t.token(req.Context())
	if err != nil {
		return nil, err
	}
	resp, err := do(token)
	if err != nil || resp.StatusCode != http.StatusUnauthorized || t.tokenSource() == nil {
		return resp, err
	}
	if req.Body != nil && req.GetBody == nil {
		return resp, nil
	}
	refreshed, refreshErr := t.refresh(req.Context())
	if refreshErr != nil {
		return resp, nil
	}
	resp.Body.Close()
	return do(refreshed)
}

func (t *Client) DoJSON(ctx context.Context, method, path string, query url.Values, body any, out any, retry bool) error {
	if t == nil || t.BaseURL == "" {
		return fmt.Errorf("sdk: invalid configuration: base URL is required")
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	endpoint := t.BaseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	attempts := 1
	if retry && (method == http.MethodGet || method == http.MethodHead || method == http.MethodPut || method == http.MethodDelete) {
		attempts = 3
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<attempt) * 250 * time.Millisecond
			var apiErr *APIError
			if errors.As(lastErr, &apiErr) && apiErr.RetryAfter > 0 {
				delay = apiErr.RetryAfter
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
		err = t.once(ctx, method, endpoint, payload, out)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retryableError(err) {
			return err
		}
	}
	return lastErr
}

func (t *Client) once(ctx context.Context, method, endpoint string, payload []byte, out any) error {
	var reader io.Reader
	if len(payload) > 0 && string(payload) != "null" {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if t.UserAgent != "" {
		req.Header.Set("User-Agent", t.UserAgent)
	}
	if token, tokenErr := t.token(ctx); tokenErr != nil {
		return tokenErr
	} else if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := t.HTTPClient.Do(req)
	if err != nil {
		return &APIError{Code: "API_UNREACHABLE", Message: err.Error(), Retryable: true}
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		if refreshed, refreshErr := t.refresh(ctx); refreshErr == nil && refreshed != "" {
			req2 := req.Clone(ctx)
			req2.Header.Set("Authorization", "Bearer "+refreshed)
			if req.Body != nil {
				if req.GetBody == nil {
					return DecodeAPIError(resp)
				}
				body, bodyErr := req.GetBody()
				if bodyErr != nil {
					return DecodeAPIError(resp)
				}
				req2.Body = body
			}
			resp.Body.Close()
			resp, err = t.HTTPClient.Do(req2)
			if err != nil {
				return &APIError{Code: "API_UNREACHABLE", Message: err.Error(), Retryable: true}
			}
			defer resp.Body.Close()
		}
	}
	if resp.StatusCode/100 != 2 {
		return DecodeAPIError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (t *Client) token(ctx context.Context) (string, error) {
	source := t.tokenSource()
	if source == nil {
		return "", nil
	}
	return source.Token(ctx)
}

func (t *Client) refresh(ctx context.Context) (string, error) {
	refreshable, ok := t.tokenSource().(RefreshableTokenSource)
	if !ok {
		return "", fmt.Errorf("token source is not refreshable")
	}
	return refreshable.Refresh(ctx)
}

// DoProto is the unified path for generated proto endpoints. It sends proto
// JSON and preserves int64-as-string decoding.
func (t *Client) DoProto(ctx context.Context, method, path string, query url.Values, body, out proto.Message) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = protojson.Marshal(body)
		if err != nil {
			return err
		}
	}
	endpoint := t.BaseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	attempts := 1
	if method == http.MethodGet || method == http.MethodDelete || method == http.MethodPut {
		attempts = 3
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<attempt) * 250 * time.Millisecond
			var apiErr *APIError
			if errors.As(lastErr, &apiErr) && apiErr.RetryAfter > 0 {
				delay = apiErr.RetryAfter
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
		err = t.onceProto(ctx, method, endpoint, payload, out)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retryableError(err) {
			return err
		}
	}
	return lastErr
}

func (t *Client) onceProto(ctx context.Context, method, endpoint string, payload []byte, out proto.Message) error {
	var reader io.Reader
	if len(payload) > 0 {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if t.UserAgent != "" {
		req.Header.Set("User-Agent", t.UserAgent)
	}
	if token, tokenErr := t.token(ctx); tokenErr != nil {
		return tokenErr
	} else if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := t.HTTPClient.Do(req)
	if err != nil {
		return &APIError{Code: "API_UNREACHABLE", Message: err.Error(), Retryable: true}
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		if refreshed, refreshErr := t.refresh(ctx); refreshErr == nil && refreshed != "" {
			req2 := req.Clone(ctx)
			req2.Header.Set("Authorization", "Bearer "+refreshed)
			if req.Body != nil {
				if req.GetBody == nil {
					return DecodeAPIError(resp)
				}
				body, bodyErr := req.GetBody()
				if bodyErr != nil {
					return DecodeAPIError(resp)
				}
				req2.Body = body
			}
			resp.Body.Close()
			resp, err = t.HTTPClient.Do(req2)
			if err != nil {
				return &APIError{Code: "API_UNREACHABLE", Message: err.Error(), Retryable: true}
			}
			defer resp.Body.Close()
		}
	}
	if resp.StatusCode/100 != 2 {
		return DecodeAPIError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	return protojson.Unmarshal(data, out)
}

func DecodeAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var payload struct {
		Code         string    `json:"code"`
		Reason       string    `json:"reason"`
		Message      string    `json:"message"`
		Retryable    bool      `json:"retryable"`
		RetryAfterMs JSONInt64 `json:"retryAfterMs"`
		RequestID    string    `json:"requestId"`
	}
	_ = json.Unmarshal(body, &payload)
	code := payload.Code
	if code == "" {
		code = payload.Reason
	}
	if code == "" {
		code = fmt.Sprintf("HTTP_%d", resp.StatusCode)
	}
	message := payload.Message
	if message == "" {
		message = strings.TrimSpace(string(body))
	}
	if message == "" {
		message = resp.Status
	}
	requestID := payload.RequestID
	if requestID == "" {
		requestID = resp.Header.Get("X-Request-Id")
	}
	retryable := payload.Retryable || resp.StatusCode == 408 || resp.StatusCode == 429 || resp.StatusCode >= 500
	retryAfter := time.Duration(payload.RetryAfterMs) * time.Millisecond
	if retryAfter == 0 {
		if raw := resp.Header.Get("Retry-After"); raw != "" {
			if seconds, err := strconv.Atoi(raw); err == nil {
				retryAfter = time.Duration(seconds) * time.Second
			}
		}
	}
	return &APIError{
		StatusCode: resp.StatusCode, Code: code, Message: message, Retryable: retryable,
		RetryAfter: retryAfter, RequestID: requestID,
	}
}

func retryableError(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Retryable && apiErr.StatusCode != 0
}
