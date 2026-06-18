package sdk

import (
	"hash/fnv"
	"time"
)

// GatewaySelector determines how the SDK picks a gateway URL from a list.
type GatewaySelector string

const (
	// GatewaySelectorRoundRobin cycles through gateway URLs in order.
	GatewaySelectorRoundRobin GatewaySelector = "round_robin"
	// GatewaySelectorRandom picks a random gateway URL.
	GatewaySelectorRandom GatewaySelector = "random"
	// GatewaySelectorHashUserID deterministically selects a gateway URL based
	// on the user ID. This keeps a given user on the same gateway across
	// reconnections, which improves routing cache hit rates on the server.
	GatewaySelectorHashUserID GatewaySelector = "hash_user_id"
)

// Options configures the IM SDK client.
type Options struct {
	// BaseURL is the HTTP base URL of the API server (e.g., "http://localhost:8000").
	BaseURL string

	// GatewayURL is the WebSocket URL of the gateway server (e.g., "ws://localhost:8000/ws").
	// If empty, the SDK will try to auto-resolve via DispatchService.
	// If GatewayURLs is also provided, GatewayURL takes precedence for backward
	// compatibility.
	GatewayURL string

	// GatewayURLs is a list of WebSocket URLs for multi-gateway deployments.
	// The SDK selects one entry according to GatewaySelector. If the selected
	// gateway fails, it falls back to the next entries.
	GatewayURLs []string

	// GatewaySelector decides which URL from GatewayURLs (or from the server
	// discovered list) to use. Defaults to GatewaySelectorRoundRobin.
	GatewaySelector GatewaySelector

	// Token is the JWT token for authentication.
	Token string

	// DeviceID identifies the client device. Defaults to "sdk-default".
	DeviceID string

	// HeartbeatInterval is the interval between heartbeats. Defaults to 30s.
	// Should be aligned with the server's gateway.heartbeat_interval (#32).
	HeartbeatInterval time.Duration

	// HeartbeatTimeout is the maximum time to wait for a heartbeat echo
	// from the server before considering the connection dead. Defaults to 60s.
	// Should be aligned with the server's gateway.heartbeat_timeout (#32).
	HeartbeatTimeout time.Duration

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

	// OnSendReceipt is called when a send receipt (msg_id/topic_seq confirmation) is received.
	OnSendReceipt SendReceiptHandler
}

// withDefaults returns a copy of Options with default values filled in.
func (o Options) withDefaults() Options {
	if o.DeviceID == "" {
		o.DeviceID = "sdk-default"
	}
	if o.HeartbeatInterval <= 0 {
		o.HeartbeatInterval = 30 * time.Second
	}
	if o.HeartbeatTimeout <= 0 {
		o.HeartbeatTimeout = 60 * time.Second
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
		HeartbeatTimeout:     60 * time.Second,
		RequestTimeout:       10 * time.Second,
		ReconnectInterval:    5 * time.Second,
		AutoReconnect:        true,
		MaxReconnectAttempts: 0,
		AutoAck:              true,
		GatewaySelector:      GatewaySelectorRoundRobin,
	}
}

// selectGatewayURL picks a gateway URL from the provided list using the
// configured selector and user ID. It returns the selected index.
func selectGatewayURL(urls []string, selector GatewaySelector, userID int64, attempt int) (string, int) {
	if len(urls) == 0 {
		return "", 0
	}

	if selector == "" {
		selector = GatewaySelectorRoundRobin
	}

	var idx int
	switch selector {
	case GatewaySelectorHashUserID:
		h := fnv.New32a()
		_, _ = h.Write([]byte("im-sdk"))
		idx = int(h.Sum32()+uint32(userID)) % len(urls)
	case GatewaySelectorRandom:
		// For deterministic tests, fall back to round-robin when userID is 0.
		if userID == 0 {
			idx = attempt % len(urls)
		} else {
			h := fnv.New32a()
			_, _ = h.Write([]byte("im-sdk-random"))
			idx = int(h.Sum32()+uint32(userID)+uint32(attempt)) % len(urls)
		}
	case GatewaySelectorRoundRobin:
		fallthrough
	default:
		idx = attempt % len(urls)
	}

	return urls[idx], idx
}

// gatewayList returns the effective list of gateway URLs. The explicit
// GatewayURL takes precedence over GatewayURLs for backward compatibility.
func gatewayList(gatewayURL string, gatewayURLs []string, serverDiscovered []string) []string {
	if gatewayURL != "" {
		return []string{gatewayURL}
	}
	if len(gatewayURLs) > 0 {
		return gatewayURLs
	}
	if len(serverDiscovered) > 0 {
		return serverDiscovered
	}
	return nil
}
