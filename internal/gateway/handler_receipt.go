package gateway

import (
	"context"
	v1 "nonoka-im/api/im/v1"
	domain "nonoka-im/internal/im/domain"
	"strconv"
	"strings"
	"time"
)

// handleAck handles client ACKs for delivered messages.
// Updates the delivery status in MongoDB and pushes a DeliveryReceipt to the sender.
func (h *Handler) handleAck(c *Connection, packet *v1.Packet) {
	if c.State() != ConnStateAuthed {
		return
	}

	req := packet.GetAckReq()
	if req == nil {
		return
	}

	// ACK processing touches MongoDB. Keep it off the read loop so a slow
	// database cannot head-of-line block heartbeats and subsequent packets
	// from the same connection.
	h.runReceiptTask(func() { h.processAck(c.UserID(), req) })
}

func (h *Handler) processAck(userID int64, req *v1.AckRequest) {
	defer func() {
		if r := recover(); r != nil {
			h.log.Errorf("ack processing panic recovered: %v", r)
		}
	}()
	if !h.topicMember(context.Background(), userID, req.GetTopic()) {
		h.log.Warnf("ack for non-member topic ignored: user_id=%d topic=%s", userID, req.GetTopic())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := h.query.AckDelivery(ctx, userID, req.GetTopic(), req.GetTopicSeq()); err != nil {
		h.log.Warnf("update delivery status failed: user_id=%d, topic=%s, seq=%d, err=%v",
			userID, req.GetTopic(), req.GetTopicSeq(), err)
		return
	}
	h.log.Debugf("ack received: user_id=%d, topic=%s, msg_id=%d, seq=%d",
		userID, req.GetTopic(), req.GetMsgId(), req.GetTopicSeq())
	h.pushDeliveryReceipt(req.GetTopic(), req.GetTopicSeq(), req.GetMsgId())
}

// pushDeliveryReceipt pushes a delivery receipt to the message sender.
func (h *Handler) pushDeliveryReceipt(topic string, topicSeq uint64, msgID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Find the sender of the message.
	senderID, err := h.query.MessageSender(ctx, topic, topicSeq)
	if err != nil {
		h.log.Debugf("get message sender for delivery receipt failed: topic=%s, seq=%d, err=%v",
			topic, topicSeq, err)
		return
	}

	receipt := &v1.Packet{
		Cmd: v1.Command_CMD_DELIVERY_RECEIPT,
		Payload: &v1.Packet_DeliveryReceipt{
			DeliveryReceipt: &v1.DeliveryReceipt{
				Topic:    topic,
				TopicSeq: topicSeq,
				MsgId:    msgID,
			},
		},
	}

	h.fanoutToUsers(ctx, []int64{senderID}, receipt)
	h.log.Debugf("delivery receipt pushed: sender_id=%d, topic=%s, seq=%d",
		senderID, topic, topicSeq)
}

// handleReadReceipt processes client read receipts.
// Updates the read status in MongoDB and pushes a ReadReceipt to the other party.
func (h *Handler) handleReadReceipt(c *Connection, packet *v1.Packet) {
	if c.State() != ConnStateAuthed {
		return
	}

	req := packet.GetReadReceipt()
	if req == nil {
		return
	}

	// Read status persistence is best-effort and must not block the socket.
	h.runReceiptTask(func() { h.processReadReceipt(c.UserID(), req) })
}

func (h *Handler) processReadReceipt(readerID int64, req *v1.ReadReceipt) {
	defer func() {
		if r := recover(); r != nil {
			h.log.Errorf("read receipt panic recovered: %v", r)
		}
	}()
	topic, upToSeq := req.GetTopic(), req.GetUpToSeq()
	if !h.topicMember(context.Background(), readerID, topic) {
		h.log.Warnf("read receipt for non-member topic ignored: user_id=%d topic=%s", readerID, topic)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := h.query.MarkRead(ctx, readerID, topic, upToSeq); err != nil {
		h.log.Warnf("update read status failed: user_id=%d, topic=%s, up_to_seq=%d, err=%v",
			readerID, topic, upToSeq, err)
		return
	}
	h.log.Debugf("read receipt received: user_id=%d, topic=%s, up_to_seq=%d",
		readerID, topic, upToSeq)
	h.pushReadReceipt(readerID, topic, upToSeq)
}

// runReceiptTask bounds receipt-processing goroutines. ACKs are sent for every
// delivered message, so an unbounded goroutine per packet is a trivial DoS.
func (h *Handler) runReceiptTask(task func()) {
	select {
	case h.receiptSem <- struct{}{}:
		go func() {
			defer func() { <-h.receiptSem }()
			task()
		}()
	default:
		h.log.Warn("receipt processing queue is full; dropping receipt")
	}
}

// topicMember authorizes the receipt paths that fan a packet out to another
// user. P2P/system topics encode their members, so the check is free. Group
// read/delivery receipts are not fanned out to other members today; checking
// membership here would add one PostgreSQL query per ACK.
func (h *Handler) topicMember(ctx context.Context, userID int64, topic string) bool {
	switch domain.ParseTopicType(topic) {
	case domain.TopicTypeP2P:
		uid1, uid2, err := domain.ExtractUserIDsFromP2PTopic(topic)
		return err == nil && (userID == uid1 || userID == uid2)
	case domain.TopicTypeSystem:
		target, err := strconv.ParseInt(strings.TrimPrefix(topic, "sys_"), 10, 64)
		return err == nil && userID == target
	case domain.TopicTypeGroup:
		return true
	default:
		return false
	}
}

// pushReadReceipt pushes a read receipt to the other party in a conversation.
func (h *Handler) pushReadReceipt(readerID int64, topic string, upToSeq uint64) {
	// Determine recipient(s) based on topic type.
	// For P2P, the other user is the recipient.
	// For Group, broadcast to all online members (simplified: skip for now).
	topicType := domain.ParseTopicType(topic)

	var recipientIDs []int64
	switch topicType {
	case domain.TopicTypeP2P:
		uid1, uid2, err := domain.ExtractUserIDsFromP2PTopic(topic)
		if err != nil {
			h.log.Debugf("parse p2p topic for read receipt failed: topic=%s, err=%v", topic, err)
			return
		}
		if readerID == uid1 {
			recipientIDs = append(recipientIDs, uid2)
		} else {
			recipientIDs = append(recipientIDs, uid1)
		}
	case domain.TopicTypeGroup:
		// For groups, we could broadcast to all online members.
		// Simplified: skip group read receipts in Phase 2.
		return
	case domain.TopicTypeSystem:
		// System messages: no read receipt needed.
		return
	default:
		return
	}

	receipt := &v1.Packet{
		Cmd: v1.Command_CMD_READ_RECEIPT,
		Payload: &v1.Packet_ReadReceipt{
			ReadReceipt: &v1.ReadReceipt{
				Topic:    topic,
				UpToSeq:  upToSeq,
				ReaderId: readerID,
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	h.fanoutToUsers(ctx, recipientIDs, receipt)
	h.log.Debugf("read receipt pushed: recipients=%v, topic=%s, up_to_seq=%d", recipientIDs, topic, upToSeq)
}
