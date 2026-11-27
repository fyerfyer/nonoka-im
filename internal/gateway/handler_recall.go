package gateway

import (
	"context"
	"errors"
	v1 "nonoka-im/api/im/v1"
	imapp "nonoka-im/internal/im/app"
	domain "nonoka-im/internal/im/domain"
	"strings"
	"time"
)

func (h *Handler) handleRecall(c *Connection, packet *v1.Packet) {
	if c.State() != ConnStateAuthed {
		h.sendError(c, packet.Seq, v1.Command_CMD_RECALL, 4001, "authentication required")
		return
	}
	req := packet.GetRecallReq()
	if req == nil || req.Topic == "" || req.TopicSeq == 0 {
		h.sendError(c, packet.Seq, v1.Command_CMD_RECALL, 4006, "topic and topic_seq required")
		return
	}
	topic, err := domain.NormalizeTopic(req.Topic)
	if err != nil {
		h.sendError(c, packet.Seq, v1.Command_CMD_RECALL, 4009, "invalid topic")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sender, err := h.query.MessageSender(ctx, topic, req.TopicSeq)
	if err != nil || sender != c.UserID() {
		h.sendError(c, packet.Seq, v1.Command_CMD_RECALL, 4030, "only the sender can recall this message")
		return
	}
	if err := h.query.Recall(ctx, c.UserID(), topic, req.TopicSeq, req.MsgId, h.recallWindow); err != nil {
		code, message := int32(5003), "failed to recall message"
		switch {
		case errors.Is(err, domain.ErrRecallNotFound), errors.Is(err, domain.ErrRecallExpired):
			code, message = 4101, "message is outside the recall window or already recalled"
		case errors.Is(err, imapp.ErrTopicDenied), errors.Is(err, imapp.ErrInvalidTopic):
			code, message = 4030, "topic access denied"
		}
		h.sendError(c, packet.Seq, v1.Command_CMD_RECALL, code, message)
		return
	}
	notice := &v1.Packet{Cmd: v1.Command_CMD_RECALL, Payload: &v1.Packet_RecallNotice{RecallNotice: &v1.RecallNotice{Topic: topic, TopicSeq: req.TopicSeq, MsgId: req.MsgId, SenderId: c.UserID(), RecalledAt: time.Now().Unix()}}}
	recipients := []int64{c.UserID()}
	if uid1, uid2, err := domain.ExtractUserIDsFromP2PTopic(topic); err == nil {
		other := uid1
		if other == c.UserID() {
			other = uid2
		}
		recipients = append(recipients, other)
	} else if h.groupMembers != nil && strings.HasPrefix(topic, "grp_") {
		if members, err := h.groupMembers(ctx, strings.TrimPrefix(topic, "grp_")); err == nil {
			recipients = append(recipients, members...)
		}
	}
	h.fanoutToUsers(ctx, recipients, notice)
	_ = c.SendWithTimeout(&v1.Packet{Cmd: v1.Command_CMD_RECALL, Seq: packet.Seq}, h.sendTimeout)
}
