package app

import (
	"context"
	"time"
)

// WebhookSender posts a signed webhook request.
type WebhookSender interface {
	Send(ctx context.Context, target WebhookTarget, eventID string, body []byte) (WebhookResponse, error)
}

type WebhookTarget struct {
	URL     string
	Secret  []byte
	Attempt int
}

type WebhookResponse struct {
	StatusCode int
	Snippet    string
	RetryAfter time.Duration
}

// URLValidator rejects unsafe webhook URLs at registration time.
type URLValidator interface {
	ValidateURL(raw string) error
}
