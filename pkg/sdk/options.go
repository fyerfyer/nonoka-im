package sdk

import "time"

// Options configures the IM SDK client.
type Options struct {
	// BaseURL is the HTTP base URL of the API server (e.g., "http://localhost:8000").
	BaseURL string

	// GatewayURL is the WebSocket URL of the gateway server (e.g., "ws://localhost:8000/ws").
	// If empty, the SDK will try to auto-resolve via DispatchService.
	GatewayURL string

	// Token is the JWT token for authentication.
	Token string

	// DeviceID identifies the client device. Defaults to "sdk-default".
	DeviceID string

	// HeartbeatInterval is the interval between heartbeats. Defaults to 30s.
	HeartbeatInterval time.Duration

	// RequestTimeout is the timeout for request-response operations. Defaults to 10s.
	RequestTimeout time.Duration

	// ReconnectInterval is the interval between reconnection attempts. Defaults to 5s.
	ReconnectInterval time.Duration

	// AutoReconnect enables automatic reconnection when the connection drops. Defaults to true.
	AutoReconnect bool

	// MaxReconnectAttempts is the maximum number of reconnection attempts.
	// 0 means unlimited. Defaults to 0.
	MaxReconnectAttempts int

	// AutoAck enables automatic ACK for received push messages. Defaults to true.
	AutoAck bool

	// OnMessage is called when a new message is pushed from the server.
	OnMessage MessageHandler

	// OnDisconnect is called when the client disconnects.
	OnDisconnect DisconnectHandler

	// OnConnect is called when the client successfully connects and authenticates.
	OnConnect ConnectHandler

	// OnReadReceipt is called when a read receipt is received.
	OnReadReceipt ReadReceiptHandler

	// OnDeliveryReceipt is called when a delivery receipt is received.
	OnDeliveryReceipt DeliveryReceiptHandler
}

// withDefaults returns a copy of Options with default values filled in.
func (o Options) withDefaults() Options {
	if o.DeviceID == "" {
		o.DeviceID = "sdk-default"
	}
	if o.HeartbeatInterval <= 0 {
		o.HeartbeatInterval = 30 * time.Second
	}
	if o.RequestTimeout <= 0 {
		o.RequestTimeout = 10 * time.Second
	}
	if o.ReconnectInterval <= 0 {
		o.ReconnectInterval = 5 * time.Second
	}
	if !o.AutoReconnect {
		// Default to true. Note: Go bool zero value cannot be distinguished
		// from explicit false. Use DefaultOptions() as base to explicitly disable.
		o.AutoReconnect = true
	}
	// MaxReconnectAttempts: 0 means unlimited, zero value is already correct.
	if !o.AutoAck {
		// Default to true. Same note as AutoReconnect applies.
		o.AutoAck = true
	}
	return o
}

// DefaultOptions returns the default configuration.
func DefaultOptions() Options {
	return Options{
		GatewayURL:           "ws://localhost:8000/ws",
		DeviceID:             "sdk-default",
		HeartbeatInterval:    30 * time.Second,
		RequestTimeout:       10 * time.Second,
		ReconnectInterval:    5 * time.Second,
		AutoReconnect:        true,
		MaxReconnectAttempts: 0,
		AutoAck:              true,
	}
}
