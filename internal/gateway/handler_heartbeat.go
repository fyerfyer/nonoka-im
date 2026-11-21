package gateway

import (
	"context"
	v1 "nonoka-im/api/im/v1"
	"time"
)

// handleHeartbeat processes heartbeat packets and updates session TTL.
func (h *Handler) handleHeartbeat(c *Connection, packet *v1.Packet) {
	c.RefreshActivity()

	if c.UserID() != 0 && h.sessionManager != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := h.sessionManager.ExpireSession(ctx, c.UserID(), c.DeviceID(), c.ConnID(), h.heartbeatTimeout*2); err != nil {
			h.log.Warnf("expire session failed: user_id=%d, err=%v", c.UserID(), err)
		}
	}

	// Echo heartbeat back to client (non-critical, use non-blocking Send).
	_ = c.Send(&v1.Packet{
		Cmd: v1.Command_CMD_HEARTBEAT,
		Seq: packet.Seq,
	})
}
