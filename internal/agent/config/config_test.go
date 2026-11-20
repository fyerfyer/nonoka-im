package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadRejectsMissingExplicitPath(t *testing.T) {
	if _, err := Load("/tmp/nonoka-agent-config-does-not-exist.yaml"); err == nil {
		t.Fatal("explicit missing config path must fail")
	}
}

func TestValidateRejectsHeartbeatNotShorterThanLease(t *testing.T) {
	cfg := Default()
	cfg.JWTSecret = []byte(strings.Repeat("x", 32))
	cfg.Worker.Lease = Duration(10 * time.Minute)
	cfg.Worker.Heartbeat = Duration(10 * time.Minute)
	if err := cfg.Validate(); err == nil {
		t.Fatal("heartbeat >= lease must fail validation")
	}
}

func TestDefaultConfigIsValid(t *testing.T) {
	cfg := Default()
	cfg.JWTSecret = []byte(strings.Repeat("x", 32))
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
}
