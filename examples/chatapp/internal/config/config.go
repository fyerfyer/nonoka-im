package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all configuration for the chat application.
type Config struct {
	// Server addresses
	HTTPServerAddr string
	GatewayWSURL   string
	BaseURL        string

	// User credentials (for demo purposes)
	Username string
	Password string
	DeviceID string

	// PeerUserID is the ID of the other party in P2P chat.
	// If 0, defaults to userID+1.
	PeerUserID int64
}

// Load loads configuration from environment variables with defaults.
func Load() *Config {
	return &Config{
		HTTPServerAddr: getEnv("CHAT_HTTP_ADDR", "http://127.0.0.1:18000"),
		GatewayWSURL:   getEnv("CHAT_GATEWAY_WS", "ws://127.0.0.1:18000/ws"),
		BaseURL:        getEnv("CHAT_BASE_URL", "http://127.0.0.1:18000"),
		Username:       getEnv("CHAT_USERNAME", "demo-user"),
		Password:       getEnv("CHAT_PASSWORD", "123456"),
		DeviceID:       getEnv("CHAT_DEVICE_ID", "demo-device"),
		PeerUserID:     getEnvInt64("CHAT_PEER_ID", 0),
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getEnvInt64(key string, defaultVal int64) int64 {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i
		}
	}
	return defaultVal
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.GatewayWSURL == "" {
		return fmt.Errorf("gateway WS URL is required")
	}
	if c.BaseURL == "" {
		return fmt.Errorf("base URL is required")
	}
	return nil
}
