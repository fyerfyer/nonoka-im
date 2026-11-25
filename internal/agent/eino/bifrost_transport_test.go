package eino

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"nonoka-im/internal/agent/domain"
)

func TestBifrostTransportMapsBudgetAndRateLimit(t *testing.T) {
	status := http.StatusPaymentRequired
	body := `{"type":"budget_exceeded","error":{"message":"budget exceeded"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-bf-vk") != "sk-bf-test" {
			t.Errorf("missing virtual key header")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	tr := &bifrostTransport{base: http.DefaultTransport, vk: "sk-bf-test"}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL, nil)
	_, err := tr.RoundTrip(req)
	var budgetErr *domain.BudgetError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected budget error, got %T %v", err, err)
	}

	status = http.StatusTooManyRequests
	body = `{"type":"rate_limited","error":{"message":"Rate limit exceeded: resets every 30s"}}`
	_, err = tr.RoundTrip(req)
	var rateErr *domain.RateLimitError
	if !errors.As(err, &rateErr) || rateErr.RetryAfter != 30*time.Second {
		t.Fatalf("expected rate limit error, got %T %v", err, err)
	}
}
