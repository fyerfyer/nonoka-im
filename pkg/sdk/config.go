package sdk

import (
	"net/http"
	"time"
)

// RealtimeConfig controls the WebSocket client. AutoReconnect/AutoAck are
// explicit booleans; use the constructor defaults and override only when
// needed.
type RealtimeConfig struct {
	GatewayURLs          []string
	DeviceID             string
	HeartbeatInterval    time.Duration
	HeartbeatTimeout     time.Duration
	ReconnectInterval    time.Duration
	MaxReconnectDelay    time.Duration
	MaxReconnectAttempts int
	DisableAutoReconnect bool
	DisableAutoAck       bool
}

func DefaultRealtimeConfig() RealtimeConfig {
	return RealtimeConfig{
		DeviceID:          "sdk-default",
		HeartbeatInterval: 30 * time.Second,
		HeartbeatTimeout:  60 * time.Second,
		ReconnectInterval: 1 * time.Second,
		MaxReconnectDelay: 30 * time.Second,
	}
}

// Config configures a Client.
type Config struct {
	BaseURL     string
	TokenSource TokenSource
	HTTPClient  *http.Client
	UserAgent   string
	Timeout     time.Duration
	Realtime    RealtimeConfig
}

// Option mutates a Config.
type Option func(*Config)

func WithBaseURL(baseURL string) Option         { return func(c *Config) { c.BaseURL = baseURL } }
func WithToken(token string) Option             { return func(c *Config) { c.TokenSource = StaticToken(token) } }
func WithTokenSource(source TokenSource) Option { return func(c *Config) { c.TokenSource = source } }
func WithHTTPClient(client *http.Client) Option { return func(c *Config) { c.HTTPClient = client } }
func WithUserAgent(agent string) Option         { return func(c *Config) { c.UserAgent = agent } }
func WithTimeout(timeout time.Duration) Option  { return func(c *Config) { c.Timeout = timeout } }
func WithRealtime(cfg RealtimeConfig) Option    { return func(c *Config) { c.Realtime = cfg } }

func (c Config) withDefaults() Config {
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: c.Timeout}
	}
	if c.UserAgent == "" {
		c.UserAgent = "nonoka-sdk-go/1"
	}
	if c.Realtime.DeviceID == "" {
		c.Realtime = DefaultRealtimeConfig()
	}
	if c.Realtime.HeartbeatInterval <= 0 {
		c.Realtime.HeartbeatInterval = 30 * time.Second
	}
	if c.Realtime.HeartbeatTimeout <= 0 {
		c.Realtime.HeartbeatTimeout = 60 * time.Second
	}
	if c.Realtime.ReconnectInterval <= 0 {
		c.Realtime.ReconnectInterval = time.Second
	}
	if c.Realtime.MaxReconnectDelay <= 0 {
		c.Realtime.MaxReconnectDelay = 30 * time.Second
	}
	return c
}
