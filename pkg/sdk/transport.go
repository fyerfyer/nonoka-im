package sdk

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
	"time"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// jsonInt64 accepts both protojson strings ("123") and plain JSON numbers.
type jsonInt64 int64

func (n *jsonInt64) UnmarshalJSON(data []byte) error {
	raw := strings.Trim(string(data), `"`)
	if raw == "" || raw == "null" {
		*n = 0
		return nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return err
	}
	*n = jsonInt64(value)
	return nil
}

type apiTransport struct {
	baseURL   string
	client    *http.Client
	tokens    TokenSource
	userAgent string
}

func newAPITransport(baseURL string, client *http.Client, tokens TokenSource, userAgent string) *apiTransport {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &apiTransport{baseURL: strings.TrimRight(baseURL, "/"), client: client, tokens: tokens, userAgent: userAgent}
}

func (t *apiTransport) doJSON(ctx context.Context, method, path string, query url.Values, body any, out any, retry bool) error {
	if t == nil || t.baseURL == "" {
		return fmt.Errorf("%w: base URL is required", ErrInvalidConfiguration)
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	endpoint := t.baseURL + path
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

func (t *apiTransport) once(ctx context.Context, method, endpoint string, payload []byte, out any) error {
	var reader io.Reader
	if len(payload) > 0 && string(payload) != "null" {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if t.userAgent != "" {
		req.Header.Set("User-Agent", t.userAgent)
	}
	if token, tokenErr := t.token(ctx); tokenErr != nil {
		return tokenErr
	} else if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := t.client.Do(req)
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
					return decodeAPIError(resp)
				}
				body, bodyErr := req.GetBody()
				if bodyErr != nil {
					return decodeAPIError(resp)
				}
				req2.Body = body
			}
			resp.Body.Close()
			resp, err = t.client.Do(req2)
			if err != nil {
				return &APIError{Code: "API_UNREACHABLE", Message: err.Error(), Retryable: true}
			}
			defer resp.Body.Close()
		}
	}
	if resp.StatusCode/100 != 2 {
		return decodeAPIError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (t *apiTransport) token(ctx context.Context) (string, error) {
	if t.tokens == nil {
		return "", nil
	}
	return t.tokens.Token(ctx)
}

func (t *apiTransport) refresh(ctx context.Context) (string, error) {
	refreshable, ok := t.tokens.(RefreshableTokenSource)
	if !ok {
		return "", fmt.Errorf("token source is not refreshable")
	}
	return refreshable.Refresh(ctx)
}

func decodeAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var payload struct {
		Code         string    `json:"code"`
		Reason       string    `json:"reason"`
		Message      string    `json:"message"`
		Retryable    bool      `json:"retryable"`
		RetryAfterMs jsonInt64 `json:"retryAfterMs"`
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

// authRoundTripper injects a dynamic token and retries once after a refresh.
type authRoundTripper struct {
	base      http.RoundTripper
	tokens    TokenSource
	userAgent string
}

func (t *authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
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
		if t.userAgent != "" {
			clone.Header.Set("User-Agent", t.userAgent)
		}
		return t.base.RoundTrip(clone)
	}
	token := ""
	if t.tokens != nil {
		value, err := t.tokens.Token(req.Context())
		if err != nil {
			return nil, err
		}
		token = value
	}
	resp, err := do(token)
	if err != nil || resp.StatusCode != http.StatusUnauthorized || t.tokens == nil {
		return resp, err
	}
	// A request with a body can only be replayed when GetBody is available.
	// GET/HEAD requests have no body and can always be retried after refresh.
	if req.Body != nil && req.GetBody == nil {
		return resp, nil
	}
	refreshable, ok := t.tokens.(RefreshableTokenSource)
	if !ok {
		return resp, nil
	}
	refreshed, refreshErr := refreshable.Refresh(req.Context())
	if refreshErr != nil {
		return resp, nil
	}
	resp.Body.Close()
	return do(refreshed)
}

func newProtoHTTPClient(baseURL string, timeout time.Duration, transport http.RoundTripper) (*khttp.Client, error) {
	options := []khttp.ClientOption{khttp.WithEndpoint(strings.TrimRight(baseURL, "/")), khttp.WithTimeout(timeout)}
	options = append(options, khttp.WithTransport(transport))
	return khttp.NewClient(context.Background(), options...)
}

// doProto is used for generated proto endpoints whose path parameters Kratos
// cannot bind from camelCase form keys. It keeps protojson int64-as-string
// decoding while allowing the SDK to build the path itself.
func (t *apiTransport) doProto(ctx context.Context, method, path string, query url.Values, body, out proto.Message) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = protojson.Marshal(body)
		if err != nil {
			return err
		}
	}
	endpoint := t.baseURL + path
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

func (t *apiTransport) onceProto(ctx context.Context, method, endpoint string, payload []byte, out proto.Message) error {
	var reader io.Reader
	if len(payload) > 0 {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if t.userAgent != "" {
		req.Header.Set("User-Agent", t.userAgent)
	}
	if token, tokenErr := t.token(ctx); tokenErr != nil {
		return tokenErr
	} else if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := t.client.Do(req)
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
					return decodeAPIError(resp)
				}
				body, bodyErr := req.GetBody()
				if bodyErr != nil {
					return decodeAPIError(resp)
				}
				req2.Body = body
			}
			resp.Body.Close()
			resp, err = t.client.Do(req2)
			if err != nil {
				return &APIError{Code: "API_UNREACHABLE", Message: err.Error(), Retryable: true}
			}
			defer resp.Body.Close()
		}
	}
	if resp.StatusCode/100 != 2 {
		return decodeAPIError(resp)
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
