package gateway

import (
	"context"
	"errors"
	v1 "nonoka-im/api/im/v1"
	imapp "nonoka-im/internal/im/app"
	"time"
)

// handlePull handles offline message pull requests through the IM query
// service, so WebSocket and HTTP pulls share one implementation.
func (h *Handler) handlePull(c *Connection, packet *v1.Packet) {
	if c.State() != ConnStateAuthed {
		h.sendError(c, packet.Seq, v1.Command_CMD_PULL, 4001, "authentication required")
		return
	}
	req := packet.GetPullReq()
	if req == nil || req.Topic == "" {
		h.sendError(c, packet.Seq, v1.Command_CMD_PULL, 4006, "topic required")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := h.query.Pull(ctx, imapp.PullRequest{
		UserID: c.UserID(), Topic: req.Topic, LastSeq: req.LastSeq, EndSeq: req.EndSeq, Limit: int(req.Limit),
	})
	if err != nil {
		switch {
		case errors.Is(err, imapp.ErrAuthRequired):
			h.sendError(c, packet.Seq, v1.Command_CMD_PULL, 4001, "authentication required")
		case errors.Is(err, imapp.ErrInvalidTopic):
			h.sendError(c, packet.Seq, v1.Command_CMD_PULL, 4009, "invalid topic")
		case errors.Is(err, imapp.ErrTopicDenied):
			h.sendError(c, packet.Seq, v1.Command_CMD_PULL, 4030, "topic access denied")
		default:
			h.sendError(c, packet.Seq, v1.Command_CMD_PULL, 5002, "failed to fetch messages")
		}
		return
	}
	messages := make([]*v1.PullMessage, len(result.Messages))
	for i, msg := range result.Messages {
		messages[i] = &v1.PullMessage{
			MsgId: msg.ID, Topic: msg.Topic, SenderId: msg.SenderID, MsgType: int32(msg.Type),
			Content: msg.Content, Timestamp: msg.Timestamp, TopicSeq: msg.TopicSeq,
			ClientMsgId: msg.ClientMsgID, Recalled: msg.Recalled,
		}
	}
	_ = c.SendWithTimeout(&v1.Packet{
		Cmd: v1.Command_CMD_PULL, Seq: packet.Seq,
		Payload: &v1.Packet_PullReply{PullReply: &v1.PullReply{
			Messages: messages, HasMore: result.HasMore, NextSeq: result.NextSeq,
		}},
	}, h.sendTimeout)
}
