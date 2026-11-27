package kafka

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestIsTransientHandlerError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "deadline", err: context.DeadlineExceeded, want: true},
		{name: "canceled", err: context.Canceled, want: true},
		{name: "net timeout", err: &net.DNSError{IsTimeout: true}, want: true},
		{name: "poison", err: errors.New("invalid message payload"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTransientHandlerError(tt.err); got != tt.want {
				t.Fatalf("isTransientHandlerError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
