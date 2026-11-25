package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a small Bifrost management/inference catalog client.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	Auth    Auth
}

func NewClient(baseURL string, auth Auth) *Client {
	return &Client{
		BaseURL: trimBaseURL(baseURL),
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		Auth:    auth,
	}
}

func trimBaseURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.send(req, out)
}

func (c *Client) send(req *http.Request, out any) error {
	c.applyAuth(req)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrGatewayUnavailable, err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("%w: read response: %v", ErrGatewayUnavailable, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr struct {
			Type  string `json:"type"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(payload, &apiErr)
		msg := apiErr.Error.Message
		if msg == "" {
			msg = apiErr.Message
		}
		if msg == "" {
			msg = strings.TrimSpace(string(payload))
		}
		return fmt.Errorf("%w: %w", ErrGatewayRejected, gatewayError{
			Type: apiErr.Type, Status: resp.StatusCode, Message: msg,
		})
	}
	if out == nil || len(payload) == 0 {
		return nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("%w: decode response: %v", ErrGatewayUnavailable, err)
	}
	return nil
}

func (c *Client) applyAuth(req *http.Request) {
	switch {
	case c.Auth.AdminToken != "":
		req.Header.Set("Authorization", "Bearer "+c.Auth.AdminToken)
	case c.Auth.Username != "" || c.Auth.Password != "":
		req.SetBasicAuth(c.Auth.Username, c.Auth.Password)
	}
	if c.Auth.SetupToken != "" {
		req.Header.Set("X-Bifrost-Setup-Token", c.Auth.SetupToken)
	}
}
