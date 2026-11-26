package delivery

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nonoka-im/internal/bot/app"

	"github.com/go-kratos/kratos/v2/log"
)

const (
	defaultTimeout = 5 * time.Second
	maxSnippet     = 512
)

// URLGuard validates and dials webhook targets. It is implemented by the
// shared SSRF guard used for MCP connectors.
type URLGuard interface {
	ValidateURL(raw string) error
	HTTPClient(timeout time.Duration) *http.Client
}

// Sender signs and posts Standard Webhooks requests.
type Sender struct {
	Guard  URLGuard
	Client *http.Client
	Now    func() time.Time
	Log    *log.Helper
}

func NewSender(guard URLGuard, logger log.Logger) *Sender {
	return &Sender{Guard: guard, Log: log.NewHelper(logger)}
}

func (s *Sender) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Sender) client() (*http.Client, error) {
	if s.Client != nil {
		return s.Client, nil
	}
	if s.Guard == nil {
		return nil, fmt.Errorf("webhook guard unavailable")
	}
	client := s.Guard.HTTPClient(defaultTimeout)
	if client == nil {
		return nil, fmt.Errorf("webhook guard unavailable")
	}
	return client, nil
}

func (s *Sender) Send(ctx context.Context, target app.WebhookTarget, eventID string, body []byte) (app.WebhookResponse, error) {
	if target.URL == "" || eventID == "" {
		return app.WebhookResponse{}, fmt.Errorf("webhook url and event id are required")
	}
	if s.Guard != nil {
		if err := s.Guard.ValidateURL(target.URL); err != nil {
			return app.WebhookResponse{}, err
		}
	}
	client, err := s.client()
	if err != nil {
		return app.WebhookResponse{}, err
	}
	// Never follow redirects: a redirected webhook can bypass URL validation.
	requestClient := *client
	requestClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	timestamp := s.now().Unix()
	signature, err := sign(target.Secret, eventID, timestamp, body)
	if err != nil {
		return app.WebhookResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.URL, bytes.NewReader(body))
	if err != nil {
		return app.WebhookResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Nonoka-Webhook/1")
	req.Header.Set("webhook-id", eventID)
	req.Header.Set("webhook-timestamp", strconv.FormatInt(timestamp, 10))
	req.Header.Set("webhook-signature", "v1,"+signature)
	if target.Attempt > 0 {
		req.Header.Set("webhook-attempt", strconv.Itoa(target.Attempt))
	}
	resp, err := requestClient.Do(req)
	if err != nil {
		return app.WebhookResponse{StatusCode: 0}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	snippet := strings.TrimSpace(string(raw))
	if len(snippet) > maxSnippet {
		snippet = snippet[:maxSnippet]
	}
	return app.WebhookResponse{
		StatusCode: resp.StatusCode,
		Snippet:    snippet,
		RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
	}, nil
}

func sign(secret []byte, eventID string, timestamp int64, body []byte) (string, error) {
	raw := strings.TrimSpace(string(secret))
	raw = strings.TrimPrefix(raw, "whsec_")
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", fmt.Errorf("invalid webhook secret: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	_, _ = fmt.Fprintf(mac, "%s.%d.", eventID, timestamp)
	_, _ = mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil)), nil
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

var _ app.WebhookSender = (*Sender)(nil)
