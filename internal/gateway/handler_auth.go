package gateway

import (
	"context"
	jwt5 "github.com/golang-jwt/jwt/v5"
	v1 "nonoka-im/api/im/v1"
	"time"
)

// handleAuth validates JWT token and binds the connection to a user.
func (h *Handler) handleAuth(c *Connection, packet *v1.Packet) {
	if c.State() != ConnStateConnected {
		h.log.Warnf("auth rejected: conn %s already authenticated", c.ConnID())
		return
	}

	req := packet.GetAuthReq()
	if req == nil {
		h.sendError(c, packet.Seq, v1.Command_CMD_AUTH, 4001, "invalid payload")
		return
	}

	if req.Token == "" {
		h.sendError(c, packet.Seq, v1.Command_CMD_AUTH, 4002, "token required")
		return
	}

	// Parse and validate JWT
	token, err := jwt5.Parse(req.Token, func(token *jwt5.Token) (any, error) {
		if _, ok := token.Method.(*jwt5.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return h.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		h.log.Warnf("auth invalid token: conn %s, err=%v", c.ConnID(), err)
		h.sendError(c, packet.Seq, v1.Command_CMD_AUTH, 4003, "invalid token")
		return
	}

	claims, ok := token.Claims.(jwt5.MapClaims)
	if !ok {
		h.sendError(c, packet.Seq, v1.Command_CMD_AUTH, 4004, "invalid claims")
		return
	}

	userID, ok := claims["user_id"].(float64)
	if !ok {
		h.sendError(c, packet.Seq, v1.Command_CMD_AUTH, 4005, "invalid user_id")
		return
	}

	deviceID := req.DeviceId
	if deviceID == "" {
		deviceID = "default"
	}

	// Mark connection as authenticated
	c.SetAuthed(int64(userID), deviceID)

	// Register to connection manager
	replaced := h.manager.Add(c)

	// Register to distributed session index
	if h.sessionManager != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := h.sessionManager.SetSession(ctx, c.UserID(), deviceID, c.ConnID(), h.heartbeatTimeout*2); err != nil {
			h.log.Warnf("set session failed: user_id=%d, err=%v", c.UserID(), err)
		}
	}

	// A device has one authoritative socket. Close older local sockets only
	// after Redis points at this connection; their guarded cleanup then becomes
	// a no-op instead of deleting the replacement route.
	for _, old := range replaced {
		h.log.Infof("replacing connection: user_id=%d device_id=%s old_conn_id=%s new_conn_id=%s",
			c.UserID(), deviceID, old.ConnID(), c.ConnID())
		old.Close()
	}

	// Send auth success response (critical: client is waiting).
	_ = c.SendWithTimeout(&v1.Packet{
		Cmd: v1.Command_CMD_AUTH,
		Seq: packet.Seq,
		Payload: &v1.Packet_AuthResp{
			AuthResp: &v1.AuthResponse{
				Success:  true,
				UserId:   int64(userID),
				DeviceId: deviceID,
			},
		},
	}, h.sendTimeout)

	h.log.Infof("auth success: user_id=%d, conn_id=%s, device_id=%s", int64(userID), c.ConnID(), deviceID)
}
