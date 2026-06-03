package gateway

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	v1 "nonoka-im/api/im/v1"

	jwt5 "github.com/golang-jwt/jwt/v5"
	"google.golang.org/protobuf/proto"
)

// Handler handles incoming WebSocket packets.
type Handler struct {
	manager        *Manager
	sessionManager *SessionManager
	producer       MessageProducer
	jwtSecret      []byte
	log            *log.Helper

	// Heartbeat settings
	heartbeatInterval time.Duration
	heartbeatTimeout  time.Duration

	// Send timeout for critical messages (e.g., ACKs, auth responses).
	// Use non-blocking Send for heartbeats and non-critical traffic.
	sendTimeout time.Duration
}

// HeartbeatConfig holds heartbeat-related configuration.
type HeartbeatConfig struct {
	Interval time.Duration
	Timeout  time.Duration
}

// NewHandler creates a new packet handler.
func NewHandler(manager *Manager, sessionManager *SessionManager, producer MessageProducer, jwtSecret []byte, hb HeartbeatConfig, logger log.Logger) *Handler {
	return &Handler{
		manager:           manager,
		sessionManager:    sessionManager,
		producer:          producer,
		jwtSecret:         jwtSecret,
		heartbeatInterval: hb.Interval,
		heartbeatTimeout:  hb.Timeout,
		sendTimeout:       100 * time.Millisecond,
		log:               log.NewHelper(logger),
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
	default:
		h.log.Warnf("unknown command from conn %s: %d", c.ConnID(), packet.Cmd)
	}
}

// handleHeartbeat processes heartbeat packets and updates session TTL.
func (h *Handler) handleHeartbeat(c *Connection, packet *v1.Packet) {
	c.RefreshActivity()

	if c.UserID() != 0 && h.sessionManager != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := h.sessionManager.ExpireSession(ctx, c.UserID(), h.heartbeatTimeout*2); err != nil {
			h.log.Warnf("expire session failed: user_id=%d, err=%v", c.UserID(), err)
		}
	}

	// Echo heartbeat back to client (non-critical, use non-blocking Send).
	_ = c.Send(&v1.Packet{
		Cmd: v1.Command_CMD_HEARTBEAT,
		Seq: packet.Seq,
	})
}

// AuthPayload represents the authentication request body.
type AuthPayload struct {
	Token    string `json:"token"`
	DeviceID string `json:"device_id"`
}

// handleAuth validates JWT token and binds the connection to a user.
func (h *Handler) handleAuth(c *Connection, packet *v1.Packet) {
	if c.State() != ConnStateConnected {
		h.log.Warnf("auth rejected: conn %s already authenticated", c.ConnID())
		return
	}

	var payload AuthPayload
	if err := json.Unmarshal(packet.Payload, &payload); err != nil {
		h.log.Warnf("auth unmarshal failed: conn %s, err=%v", c.ConnID(), err)
		h.sendError(c, packet.Seq, v1.Command_CMD_AUTH, "invalid payload")
		return
	}

	if payload.Token == "" {
		h.sendError(c, packet.Seq, v1.Command_CMD_AUTH, "token required")
		return
	}

	// Parse and validate JWT
	token, err := jwt5.Parse(payload.Token, func(token *jwt5.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt5.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return h.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		h.log.Warnf("auth invalid token: conn %s, err=%v", c.ConnID(), err)
		h.sendError(c, packet.Seq, v1.Command_CMD_AUTH, "invalid token")
		return
	}

	claims, ok := token.Claims.(jwt5.MapClaims)
	if !ok {
		h.sendError(c, packet.Seq, v1.Command_CMD_AUTH, "invalid claims")
		return
	}

	userID, ok := claims["user_id"].(float64)
	if !ok {
		h.sendError(c, packet.Seq, v1.Command_CMD_AUTH, "invalid user_id")
		return
	}

	deviceID := payload.DeviceID
	if deviceID == "" {
		deviceID = "default"
	}

	// Mark connection as authenticated
	c.SetAuthed(int64(userID), deviceID)

	// Register to connection manager
	h.manager.Add(c)

	// Register to distributed session index
	if h.sessionManager != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := h.sessionManager.SetSession(ctx, c.UserID(), deviceID, h.heartbeatTimeout*2); err != nil {
			h.log.Warnf("set session failed: user_id=%d, err=%v", c.UserID(), err)
		}
	}

	// Send auth success response (critical: client is waiting).
	authReply, _ := json.Marshal(map[string]interface{}{
		"success": true,
		"user_id": int64(userID),
	})
	_ = c.SendWithTimeout(&v1.Packet{
		Cmd:     v1.Command_CMD_AUTH,
		Seq:     packet.Seq,
		Payload: authReply,
	}, h.sendTimeout)

	h.log.Infof("auth success: user_id=%d, conn_id=%s, device_id=%s", int64(userID), c.ConnID(), deviceID)
}

// handlePublish handles client publish messages.
// It validates the message, produces it to Kafka, and ACKs the client.
func (h *Handler) handlePublish(c *Connection, packet *v1.Packet) {
	if c.State() != ConnStateAuthed {
		h.sendError(c, packet.Seq, v1.Command_CMD_PUBLISH, "authentication required")
		return
	}

	var req v1.SendMessageRequest
	if err := proto.Unmarshal(packet.Payload, &req); err != nil {
		h.log.Warnf("publish unmarshal failed: conn %s, err=%v", c.ConnID(), err)
		h.sendError(c, packet.Seq, v1.Command_CMD_PUBLISH, "invalid message format")
		return
	}

	// Basic validation
	if req.Topic == "" {
		h.sendError(c, packet.Seq, v1.Command_CMD_PUBLISH, "topic required")
		return
	}
	if req.ClientMsgId == "" {
		h.sendError(c, packet.Seq, v1.Command_CMD_PUBLISH, "client_msg_id required")
		return
	}

	// Message size limit (64KB max content to prevent abuse).
	const maxContentSize = 64 * 1024
	if len(req.Content) > maxContentSize {
		h.sendError(c, packet.Seq, v1.Command_CMD_PUBLISH, "message too large")
		return
	}

	h.log.Debugf("publish received: user_id=%d, topic=%s, client_msg_id=%s",
		c.UserID(), req.Topic, req.ClientMsgId)

	// Produce to Kafka
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	upstream := &v1.UpstreamMessage{
		SenderId:    c.UserID(),
		Topic:       req.Topic,
		MsgType:     int32(req.MsgType),
		Content:     req.Content,
		ClientMsgId: req.ClientMsgId,
		Timestamp:   time.Now().Unix(),
	}

	if err := h.producer.Produce(ctx, upstream); err != nil {
		h.log.Errorf("produce to kafka failed: user_id=%d, client_msg_id=%s, err=%v",
			c.UserID(), req.ClientMsgId, err)
		h.sendError(c, packet.Seq, v1.Command_CMD_PUBLISH, "message delivery failed")
		return
	}

	// Send ACK (critical: client is waiting for confirmation).
	reply, _ := proto.Marshal(&v1.SendMessageReply{
		ClientMsgId: req.ClientMsgId,
		Timestamp:   upstream.Timestamp,
	})

	_ = c.SendWithTimeout(&v1.Packet{
		Cmd:     v1.Command_CMD_PUBLISH,
		Seq:     packet.Seq,
		Payload: reply,
	}, h.sendTimeout)
}

// handlePull handles offline message pull requests.
// TODO: implement in Phase 5.
func (h *Handler) handlePull(c *Connection, packet *v1.Packet) {
	if c.State() != ConnStateAuthed {
		h.sendError(c, packet.Seq, v1.Command_CMD_PULL, "authentication required")
		return
	}

	h.log.Debugf("pull request: user_id=%d, conn_id=%s", c.UserID(), c.ConnID())
	// TODO: Phase 5 - query MongoDB for offline messages
}

// handleAck handles client ACKs for delivered messages.
func (h *Handler) handleAck(c *Connection, packet *v1.Packet) {
	if c.State() != ConnStateAuthed {
		return
	}
	// TODO: update message delivery status
}

// sendError sends an error response to the client.
func (h *Handler) sendError(c *Connection, seq uint64, cmd v1.Command, message string) {
	payload, _ := json.Marshal(map[string]interface{}{
		"error": message,
	})
	_ = c.SendWithTimeout(&v1.Packet{
		Cmd:     cmd,
		Seq:     seq,
		Payload: payload,
	}, h.sendTimeout)
}

// OnConnectionClose handles connection cleanup when a connection closes.
func (h *Handler) OnConnectionClose(c *Connection) {
	if c.UserID() != 0 {
		// Remove from local connection manager
		h.manager.Remove(c)

		// Remove from distributed session index
		if h.sessionManager != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := h.sessionManager.DelSession(ctx, c.UserID(), c.DeviceID()); err != nil {
				h.log.Warnf("del session failed: user_id=%d, err=%v", c.UserID(), err)
			}
		}
	}

	h.log.Infof("connection closed: conn_id=%s, user_id=%d", c.ConnID(), c.UserID())
}
