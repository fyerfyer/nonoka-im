package gateway

import (
	"testing"

	"github.com/segmentio/kafka-go"
)

func TestParseCompression(t *testing.T) {
	tests := []struct {
		input    string
		expected kafka.Compression
	}{
		{"gzip", kafka.Gzip},
		{"snappy", kafka.Snappy},
		{"lz4", kafka.Lz4},
		{"zstd", kafka.Zstd},
		{"none", kafka.Lz4},
		{"", kafka.Lz4},
		{"UNKNOWN", kafka.Lz4},
	}
	for _, tt := range tests {
		got := ParseCompression(tt.input)
		if got != tt.expected {
			t.Fatalf("ParseCompression(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestParseRequiredAcks(t *testing.T) {
	tests := []struct {
		input    string
		expected kafka.RequiredAcks
	}{
		{"none", kafka.RequireNone},
		{"one", kafka.RequireOne},
		{"all", kafka.RequireAll},
		{"", kafka.RequireAll},
		{"UNKNOWN", kafka.RequireAll},
	}
	for _, tt := range tests {
		got := ParseRequiredAcks(tt.input)
		if got != tt.expected {
			t.Fatalf("ParseRequiredAcks(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestNoopProducerHealthy(t *testing.T) {
	p := NewNoopProducer()
	if !p.Healthy() {
		t.Fatal("NoopProducer should always be healthy")
	}
}
