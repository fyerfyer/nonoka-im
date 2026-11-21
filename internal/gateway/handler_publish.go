package gateway

import (
	"context"
	"errors"
	v1 "nonoka-im/api/im/v1"
	imapp "nonoka-im/internal/im/app"
	domain "nonoka-im/internal/im/domain"
	"time"
)

// handlePublish validates a client publish, forwards it through the IM
// application service and ACKs the client.
func (h *Handler) handlePublish(c *Connection, packet *v1.Packet) {
	if !h.limiterFor(c.ConnID()).Allow() {
		h.log.Warnf("publish rate limited: conn %s", c.ConnID())
		h.sendError(c, packet.Seq, v1.Command_CMD_PUBLISH, errCodeRateLimited, "rate limit exceeded, slow down")
		return
	}
	if c.State() != ConnStateAuthed {
		h.sendError(c, packet.Seq, v1.Command_CMD_PUBLISH, 4001, "authentication required")
		return
	}
	req := packet.GetSendReq()
	if req == nil {
		h.sendError(c, packet.Seq, v1.Command_CMD_PUBLISH, 4001, "invalid message format")
		return
	}
	if req.Topic == "" {
		h.sendError(c, packet.Seq, v1.Command_CMD_PUBLISH, 4006, "topic required")
		return
	}
	if req.ClientMsgId == "" {
		h.sendError(c, packet.Seq, v1.Command_CMD_PUBLISH, 4007, "client_msg_id required")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	result, err := h.messages.Send(ctx, imapp.SendRequest{
		ActorID: c.UserID(), Topic: req.Topic, Type: domain.MessageType(req.MsgType),
		Content: req.Content, ClientMsgID: req.ClientMsgId, MentionedIDs: req.MentionedUserIds,
	})
	if err != nil {
		h.log.Warnf("publish failed: user_id=%d topic=%s err=%v", c.UserID(), req.Topic, err)
		h.sendError(c, packet.Seq, v1.Command_CMD_PUBLISH, publishErrorCode(err), publishErrorMessage(err))
		return
	}
	h.metrics.ObservePublishLatency(time.Since(start).Seconds())
	h.metrics.IncMessagesPublished()
	_ = c.SendWithTimeout(&v1.Packet{
		Cmd: v1.Command_CMD_PUBLISH, Seq: packet.Seq,
		Payload: &v1.Packet_SendReply{SendReply: &v1.SendMessageReply{
			ClientMsgId: result.ClientMsgID, Timestamp: result.Timestamp,
		}},
	}, h.sendTimeout)
}

func publishErrorCode(err error) int32 {
	switch {
	case errors.Is(err, imapp.ErrAuthRequired):
		return 4001
	case errors.Is(err, imapp.ErrInvalidTopic):
		return 4009
	case errors.Is(err, imapp.ErrTopicDenied):
		return 4030
	case errors.Is(err, imapp.ErrMessageTooLarge):
		return 4008
	case errors.Is(err, imapp.ErrClientMsgIDReq):
		return 4007
	default:
		return 5001
	}
}

func publishErrorMessage(err error) string {
	switch {
	case errors.Is(err, imapp.ErrAuthRequired):
		return "authentication required"
	case errors.Is(err, imapp.ErrInvalidTopic):
		return "invalid topic"
	case errors.Is(err, imapp.ErrTopicDenied):
		return "topic access denied"
	case errors.Is(err, imapp.ErrMessageTooLarge):
		return "message too large"
	case errors.Is(err, imapp.ErrClientMsgIDReq):
		return "client_msg_id required"
	default:
		return "message delivery failed"
	}
}
