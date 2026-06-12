package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
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

	// SDK / runtime tuning (all overridable via environment variables).
	HeartbeatInterval         time.Duration // WebSocket heartbeat interval
	ConnectionMonitorInterval time.Duration // How often to poll connection state
	RequestTimeout            time.Duration // Timeout for HTTP requests and synchronous SDK calls
	ConnectTimeout            time.Duration // Timeout for establishing connection
	SendTimeout               time.Duration // Timeout for sending a message
	LoadHistoryTimeout        time.Duration // Timeout for loading message history
	MarkReadTimeout           time.Duration // Timeout for marking messages as read
	AuthTimeout               time.Duration // Timeout for register/login
	PushWaitDuration          time.Duration // How long to wait for push messages before auto-exit
	AutoExitDuration          time.Duration // Auto-exit after this duration (0 = wait for signal)
}

// Load loads configuration from environment variables with defaults.
func Load() *Config {
	return &Config{
		HTTPServerAddr:            getEnv("CHAT_HTTP_ADDR", "http://127.0.0.1:18000"),
		GatewayWSURL:              getEnv("CHAT_GATEWAY_WS", "ws://127.0.0.1:18000/ws"),
		BaseURL:                   getEnv("CHAT_BASE_URL", "http://127.0.0.1:18000"),
		Username:                  getEnv("CHAT_USERNAME", "demo-user"),
		Password:                  getEnv("CHAT_PASSWORD", "123456"),
		DeviceID:                  getEnv("CHAT_DEVICE_ID", "demo-device"),
		PeerUserID:                getEnvInt64("CHAT_PEER_ID", 0),
		HeartbeatInterval:         getEnvDuration("CHAT_HEARTBEAT_INTERVAL", 30*time.Second),
		ConnectionMonitorInterval: getEnvDuration("CHAT_CONN_MONITOR_INTERVAL", 5*time.Second),
		RequestTimeout:            getEnvDuration("CHAT_REQUEST_TIMEOUT", 10*time.Second),
		ConnectTimeout:            getEnvDuration("CHAT_CONNECT_TIMEOUT", 15*time.Second),
		SendTimeout:               getEnvDuration("CHAT_SEND_TIMEOUT", 10*time.Second),
		LoadHistoryTimeout:        getEnvDuration("CHAT_LOAD_HISTORY_TIMEOUT", 10*time.Second),
		MarkReadTimeout:           getEnvDuration("CHAT_MARK_READ_TIMEOUT", 10*time.Second),
		AuthTimeout:               getEnvDuration("CHAT_AUTH_TIMEOUT", 10*time.Second),
		PushWaitDuration:          getEnvDuration("CHAT_PUSH_WAIT_DURATION", 5*time.Second),
		AutoExitDuration:          getEnvDuration("CHAT_AUTO_EXIT", 0),
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

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
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
