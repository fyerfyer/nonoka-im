package gateway

import (
	"context"
	"sync"
	"time"

	v1 "nonoka-im/api/im/v1"
	imapp "nonoka-im/internal/im/app"
	"nonoka-im/internal/metrics"

	"github.com/go-kratos/kratos/v2/log"
	"golang.org/x/time/rate"
)

// Default per-connection message rate limit (token bucket) applied to
// publish packets. Overridable via gateway.msg_rate_limit in config.
const (
	defaultMsgRatePerSec = 10.0
	defaultMsgBurst      = 20
	// errCodeRateLimited is returned to the sender when a connection
	// exceeds its publish rate limit. The connection stays open.
	errCodeRateLimited = 4029
)

// Handler handles incoming WebSocket packets.
type Handler struct {
	manager        *Manager
	sessionManager *SessionManager
	messages       *imapp.MessageService
	query          *imapp.QueryService
	jwtSecret      []byte
	metrics        *metrics.Metrics
	log            *log.Helper

	// Heartbeat settings
	heartbeatInterval time.Duration
	heartbeatTimeout  time.Duration

	// Send timeout for critical messages (e.g., ACKs, auth responses).
	sendTimeout  time.Duration
	recallWindow time.Duration
	groupMembers func(context.Context, string) ([]int64, error)
	peerBus      *PeerBus

	// Per-connection publish rate limit (token bucket). limiters maps
	// connID to the connection's bucket; entries are removed on close.
	msgRate  float64
	msgBurst int
	limiters sync.Map
}

// HeartbeatConfig holds heartbeat-related configuration.
type HeartbeatConfig struct {
	Interval     time.Duration
	Timeout      time.Duration
	ReadTimeout  time.Duration // WebSocket read timeout (0 = default)
	WriteTimeout time.Duration // WebSocket write timeout (0 = default)
}

// NewHandler creates a new packet handler.
func NewHandler(manager *Manager, sessionManager *SessionManager, messages *imapp.MessageService, query *imapp.QueryService, jwtSecret []byte, hb HeartbeatConfig, logger log.Logger, m ...*metrics.Metrics) *Handler {
	h := &Handler{
		manager:           manager,
		sessionManager:    sessionManager,
		messages:          messages,
		query:             query,
		jwtSecret:         jwtSecret,
		heartbeatInterval: hb.Interval,
		heartbeatTimeout:  hb.Timeout,
		sendTimeout:       100 * time.Millisecond,
		recallWindow:      2 * time.Minute,
		msgRate:           defaultMsgRatePerSec,
		msgBurst:          defaultMsgBurst,
		log:               log.NewHelper(logger),
	}
	if len(m) > 0 {
		h.metrics = m[0]
	}
	return h
}

// SetMessageRateLimit configures the per-connection publish rate limit.
// msgPerSec <= 0 disables limiting (fail open); burst < 1 resets to 1.
// Existing per-connection buckets are reset.
func (h *Handler) SetMessageRateLimit(msgPerSec float64, burst int) {
	if burst < 1 {
		burst = 1
	}
	if msgPerSec > 0 {
		h.msgRate = msgPerSec
		h.msgBurst = burst
	} else {
		// Unlimited: a very high rate with the requested burst.
		h.msgRate = float64(rate.Inf)
		h.msgBurst = burst
	}
	h.limiters = sync.Map{}
}

// limiterFor returns (creating if needed) the token bucket of a connection.
func (h *Handler) limiterFor(connID string) *rate.Limiter {
	if v, ok := h.limiters.Load(connID); ok {
		return v.(*rate.Limiter)
	}
	limiter := rate.NewLimiter(rate.Limit(h.msgRate), h.msgBurst)
	actual, _ := h.limiters.LoadOrStore(connID, limiter)
	return actual.(*rate.Limiter)
}

// SetRecallWindow configures the server-side recall age limit.
func (h *Handler) SetRecallWindow(window time.Duration) {
	if window > 0 {
		h.recallWindow = window
	}
}

// SetGroupMemberResolver enables real-time recall fan-out for group topics.
func (h *Handler) SetGroupMemberResolver(resolve func(context.Context, string) ([]int64, error)) {
	h.groupMembers = resolve
}

// SetPeerBus enables gateway-to-gateway fan-out for receipts and recalls.
func (h *Handler) SetPeerBus(bus *PeerBus) {
	h.peerBus = bus
}

// fanoutToUsers delivers a packet to all devices of the listed users. With a
// peer bus it publishes once and lets every gateway deliver to its local
// sessions; without Redis it falls back to the local connection manager.
func (h *Handler) fanoutToUsers(ctx context.Context, userIDs []int64, packet *v1.Packet) {
	if len(userIDs) == 0 || packet == nil {
		return
	}
	unique := make([]int64, 0, len(userIDs))
	seen := make(map[int64]struct{}, len(userIDs))
	for _, id := range userIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if h.peerBus != nil {
		if err := h.peerBus.Publish(ctx, unique, packet); err == nil {
			return
		} else {
			h.log.Warnf("gateway peer fanout failed, falling back locally: %v", err)
		}
	}
	for _, id := range unique {
		h.manager.BroadcastToUser(id, packet)
	}
}

// HandlePacket processes an incoming packet from a connection.
func (h *Handler) HandlePacket(c *Connection, packet *v1.Packet) {
	switch packet.Cmd {
	case v1.Command_CMD_HEARTBEAT:
		h.handleHeartbeat(c, packet)
	case v1.Command_CMD_AUTH:
		h.handleAuth(c, packet)
	case v1.Command_CMD_PUBLISH:
		h.handlePublish(c, packet)
	case v1.Command_CMD_PULL:
		h.handlePull(c, packet)
	case v1.Command_CMD_ACK:
		h.handleAck(c, packet)
	case v1.Command_CMD_READ_RECEIPT:
		h.handleReadReceipt(c, packet)
	case v1.Command_CMD_RECALL:
		h.handleRecall(c, packet)
	default:
		h.log.Warnf("unknown command from conn %s: %d", c.ConnID(), packet.Cmd)
	}
}

// sendError sends a structured error response to the client.
func (h *Handler) sendError(c *Connection, seq uint64, cmd v1.Command, code int32, message string) {
	_ = c.SendWithTimeout(&v1.Packet{
		Cmd: cmd,
		Seq: seq,
		Payload: &v1.Packet_Error{
			Error: &v1.ErrorResponse{
				Code:      code,
				Message:   message,
				Retryable: code >= 5000, // server errors are retryable
			},
		},
	}, h.sendTimeout)
}

// OnConnectionClose handles connection cleanup when a connection closes.
func (h *Handler) OnConnectionClose(c *Connection) {
	h.limiters.Delete(c.ConnID())

	if c.UserID() != 0 {
		// Remove from local connection manager
		h.manager.Remove(c)

		// Remove from distributed session index
		if h.sessionManager != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := h.sessionManager.DelSession(ctx, c.UserID(), c.DeviceID(), c.ConnID()); err != nil {
				h.log.Warnf("del session failed: user_id=%d, err=%v", c.UserID(), err)
			}
		}
	}

	h.log.Infof("connection closed: conn_id=%s, user_id=%d", c.ConnID(), c.UserID())
}
