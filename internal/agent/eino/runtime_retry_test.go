package eino

import (
	"context"
	"errors"
	"net"
	"testing"

	openai "github.com/cloudwego/eino-ext/components/model/openai"
)

func TestRetryableModelError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "rate limit", err: &openai.APIError{HTTPStatusCode: 429}, want: true},
		{name: "server error", err: &openai.APIError{HTTPStatusCode: 503}, want: true},
		{name: "request timeout", err: &openai.APIError{HTTPStatusCode: 408}, want: true},
		{name: "bad request", err: &openai.APIError{HTTPStatusCode: 400}, want: false},
		{name: "unauthorized", err: &openai.APIError{HTTPStatusCode: 401}, want: false},
		{name: "not found", err: &openai.APIError{HTTPStatusCode: 404}, want: false},
		{name: "deadline", err: context.DeadlineExceeded, want: true},
		{name: "canceled", err: context.Canceled, want: false},
		{name: "network", err: &net.DNSError{Err: "timeout", IsTimeout: true}, want: true},
		{name: "unknown", err: errors.New("provider hiccup"), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := retryableModelError(context.Background(), tt.err); got != tt.want {
				t.Fatalf("retryableModelError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
