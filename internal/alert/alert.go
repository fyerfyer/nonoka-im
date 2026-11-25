// Package alert defines the small notification seam used by operational
// watchdogs. The built-in webhook is intentionally generic JSON so deployments
// can adapt it to Slack, Feishu, PagerDuty or an internal alert service.
package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Event is the generic payload delivered to a Notifier.
type Event struct {
	Type             string    `json:"type"`
	Pending          int64     `json:"pending"`
	Failed           int64     `json:"failed"`
	OldestAgeSeconds float64   `json:"oldest_age_seconds"`
	At               time.Time `json:"at"`
}

// Notifier receives one operational alert. Implementations should be
// best-effort and respect their own timeout.
type Notifier interface {
	Notify(ctx context.Context, event Event) error
}

// Webhook posts Event as JSON to a user-configured URL.
type Webhook struct {
	URL    string
	HTTP   *http.Client
	Header map[string]string
}

func NewWebhook(url string) *Webhook {
	if url == "" {
		return nil
	}
	return &Webhook{URL: url, HTTP: &http.Client{Timeout: 5 * time.Second}}
}

func (w *Webhook) Notify(ctx context.Context, event Event) error {
	if w == nil || w.URL == "" {
		return nil
	}
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range w.Header {
		req.Header.Set(key, value)
	}
	client := w.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("alert webhook returned %s", resp.Status)
	}
	return nil
}
