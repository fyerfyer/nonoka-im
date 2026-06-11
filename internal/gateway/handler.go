package gateway

import (
	"context"
	"sort"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/msgworker"

	jwt5 "github.com/golang-jwt/jwt/v5"
)

// Handler handles incoming WebSocket packets.
type Handler struct {
	manager        *Manager
	sessionManager *SessionManager
	producer       MessageProducer
	storage        *msgworker.MessageStorage
	jwtSecret      []byte
	log            *log.Helper

	// Heartbeat settings
	heartbeatInterval time.Duration
	heartbeatTimeout  time.Duration

	// Send timeout for critical messages (e.g., ACKs, auth responses).
	sendTimeout time.Duration
}

// HeartbeatConfig holds heartbeat-related configuration.
type HeartbeatConfig struct {
	Interval    time.Duration
	Timeout     time.Duration
	ReadTimeout time.Duration // WebSocket read timeout (0 = default)
}

// NewHandler creates a new packet handler.
func NewHandler(manager *Manager, sessionManager *SessionManager, producer MessageProducer, storage *msgworker.MessageStorage, jwtSecret []byte, hb HeartbeatConfig, logger log.Logger) *Handler {
	return &Handler{
		manager:           manager,
		sessionManager:    sessionManager,
		producer:          producer,
		storage:           storage,
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
	case v1.Command_CMD_READ_RECEIPT:
		h.handleReadReceipt(c, packet)
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
	token, err := jwt5.Parse(req.Token, func(token *jwt5.Token) (interface{}, error) {
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

// handlePublish handles client publish messages.
// It validates the message, produces it to Kafka, and ACKs the client.
func (h *Handler) handlePublish(c *Connection, packet *v1.Packet) {
	if c.State() != ConnStateAuthed {
		h.sendError(c, packet.Seq, v1.Command_CMD_PUBLISH, 4001, "authentication required")
		return
	}

	req := packet.GetSendReq()
	if req == nil {
		h.sendError(c, packet.Seq, v1.Command_CMD_PUBLISH, 4001, "invalid message format")
		return
	}

	// Basic validation
	if req.Topic == "" {
		h.sendError(c, packet.Seq, v1.Command_CMD_PUBLISH, 4006, "topic required")
		return
	}
	if req.ClientMsgId == "" {
		h.sendError(c, packet.Seq, v1.Command_CMD_PUBLISH, 4007, "client_msg_id required")
		return
	}

	// Message size limit (64KB max content to prevent abuse).
	const maxContentSize = 64 * 1024
	if len(req.Content) > maxContentSize {
		h.sendError(c, packet.Seq, v1.Command_CMD_PUBLISH, 4008, "message too large")
		return
	}

	h.log.Debugf("publish received: user_id=%d, topic=%s, client_msg_id=%s",
		c.UserID(), req.Topic, req.ClientMsgId)

	// Produce to Kafka
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	upstream := &v1.UpstreamMessage{
		SenderId:         c.UserID(),
		Topic:            req.Topic,
		MsgType:          int32(req.MsgType),
		Content:          req.Content,
		ClientMsgId:      req.ClientMsgId,
		Timestamp:        time.Now().Unix(),
		MentionedUserIds: req.MentionedUserIds,
	}

	if err := h.producer.Produce(ctx, upstream); err != nil {
		h.log.Errorf("produce to kafka failed: user_id=%d, client_msg_id=%s, err=%v",
			c.UserID(), req.ClientMsgId, err)
		h.sendError(c, packet.Seq, v1.Command_CMD_PUBLISH, 5001, "message delivery failed")
		return
	}

	// Send ACK to confirm the message has been accepted and queued for delivery.
	// Note: msgID and topicSeq are generated by the msgworker after Kafka
	// consumption, so they are not available at this point. The client will
	// receive the full message metadata via push notification or pull.
	_ = c.SendWithTimeout(&v1.Packet{
		Cmd: v1.Command_CMD_PUBLISH,
		Seq: packet.Seq,
		Payload: &v1.Packet_SendReply{
			SendReply: &v1.SendMessageReply{
				ClientMsgId: req.ClientMsgId,
				Timestamp:   upstream.Timestamp,
			},
		},
	}, h.sendTimeout)
}

// handlePull handles offline message pull requests.
func (h *Handler) handlePull(c *Connection, packet *v1.Packet) {
	if c.State() != ConnStateAuthed {
		h.sendError(c, packet.Seq, v1.Command_CMD_PULL, 4001, "authentication required")
		return
	}

	req := packet.GetPullReq()
	if req == nil {
		h.sendError(c, packet.Seq, v1.Command_CMD_PULL, 4001, "invalid request format")
		return
	}

	if req.Topic == "" {
		h.sendError(c, packet.Seq, v1.Command_CMD_PULL, 4006, "topic required")
		return
	}

	h.log.Debugf("pull request: user_id=%d, topic=%s, last_seq=%d", c.UserID(), req.Topic, req.LastSeq)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	topicType := msgworker.ParseTopicType(req.Topic)
	var pullMessages []*v1.PullMessage
	var hasMore bool

	switch topicType {
	case msgworker.TopicTypeP2P, msgworker.TopicTypeSystem:
		// P2P and system messages are stored in inbox (write扩散).
		msgs, err := h.storage.GetOfflineMessages(ctx, c.UserID(), req.Topic, req.LastSeq, int(req.Limit))
		if err != nil {
			h.log.Warnf("get offline messages failed: user_id=%d, err=%v", c.UserID(), err)
			h.sendError(c, packet.Seq, v1.Command_CMD_PULL, 5002, "failed to fetch messages")
			return
		}
		pullMessages = inboxToPullMessages(msgs)
		hasMore = len(msgs) >= int(req.Limit) && int(req.Limit) > 0

	case msgworker.TopicTypeGroup:
		// Group messages are stored in messages collection (read扩散).
		groupMsgs, err := h.storage.GetGroupMessages(ctx, req.Topic, req.LastSeq, int(req.Limit))
		if err != nil {
			h.log.Warnf("get group messages failed: user_id=%d, err=%v", c.UserID(), err)
			h.sendError(c, packet.Seq, v1.Command_CMD_PULL, 5002, "failed to fetch messages")
			return
		}

		// Also fetch @mention messages for the user in this group.
		mentionMsgs, err := h.storage.GetMentionMessages(ctx, c.UserID(), req.Topic, req.LastSeq, int(req.Limit))
		if err != nil {
			h.log.Warnf("get mention messages failed: user_id=%d, err=%v", c.UserID(), err)
			// Non-fatal: continue without mentions
		}

		// Merge and sort by topic_seq to ensure correct order.
		pullMessages = mergeAndSortMessages(groupMsgs, mentionMsgs)

		// Apply limit after merge.
		if len(pullMessages) > int(req.Limit) && int(req.Limit) > 0 {
			pullMessages = pullMessages[:req.Limit]
		}
		hasMore = len(pullMessages) >= int(req.Limit) && int(req.Limit) > 0

	default:
		h.sendError(c, packet.Seq, v1.Command_CMD_PULL, 4009, "invalid topic")
		return
	}

	// Compute next_seq: if we have messages, next_seq = last_message.topic_seq + 1.
	// This means "client should start pulling from this seq next time".
	// The query condition remains $gt: lastSeq, so using lastSeq = nextSeq will
	// correctly fetch messages after the last one we returned.
	var nextSeq uint64
	if len(pullMessages) > 0 {
		nextSeq = pullMessages[len(pullMessages)-1].TopicSeq + 1
	}

	_ = c.SendWithTimeout(&v1.Packet{
		Cmd: v1.Command_CMD_PULL,
		Seq: packet.Seq,
		Payload: &v1.Packet_PullReply{
			PullReply: &v1.PullReply{
				Messages: pullMessages,
				HasMore:  hasMore,
				NextSeq:  nextSeq,
			},
		},
	}, h.sendTimeout)

	h.log.Debugf("pull reply sent: user_id=%d, topic=%s, count=%d, next_seq=%d",
		c.UserID(), req.Topic, len(pullMessages), nextSeq)
}

// inboxToPullMessages converts inbox messages to PullMessage protobufs.
func inboxToPullMessages(msgs []*msgworker.InboxMessage) []*v1.PullMessage {
	result := make([]*v1.PullMessage, len(msgs))
	for i, m := range msgs {
		result[i] = &v1.PullMessage{
			MsgId:     m.MsgID,
			Topic:     m.Topic,
			SenderId:  m.SenderID,
			MsgType:   m.MsgType,
			Content:   m.Content,
			Timestamp: m.Timestamp,
			TopicSeq:  m.TopicSeq,
		}
	}
	return result
}

// mergeAndSortMessages merges group messages and mention messages,
// then sorts the result by topic_seq in ascending order.
func mergeAndSortMessages(groupMsgs []*msgworker.StoredMessage, mentionMsgs []*msgworker.MentionMessage) []*v1.PullMessage {
	// Pre-allocate with total capacity.
	total := len(groupMsgs) + len(mentionMsgs)
	result := make([]*v1.PullMessage, 0, total)

	for _, m := range groupMsgs {
		result = append(result, &v1.PullMessage{
			MsgId:     m.MsgID,
			Topic:     m.Topic,
			SenderId:  m.SenderID,
			MsgType:   m.MsgType,
			Content:   m.Content,
			Timestamp: m.Timestamp,
			TopicSeq:  m.TopicSeq,
		})
	}
	for _, m := range mentionMsgs {
		result = append(result, &v1.PullMessage{
			MsgId:     m.MsgID,
			Topic:     m.Topic,
			SenderId:  m.SenderID,
			MsgType:   m.MsgType,
			Content:   m.Content,
			Timestamp: m.Timestamp,
			TopicSeq:  m.TopicSeq,
		})
	}

	// Sort by topic_seq ascending. Stable sort preserves original order for equal seqs.
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].TopicSeq < result[j].TopicSeq
	})

	return result
}

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

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Update delivery status in storage.
	if err := h.storage.UpdateDeliveryStatus(ctx, c.UserID(), req.Topic, req.TopicSeq); err != nil {
		h.log.Warnf("update delivery status failed: user_id=%d, topic=%s, seq=%d, err=%v",
			c.UserID(), req.Topic, req.TopicSeq, err)
	} else {
		h.log.Debugf("ack received: user_id=%d, topic=%s, msg_id=%d, seq=%d",
			c.UserID(), req.Topic, req.MsgId, req.TopicSeq)
	}

	// Push DeliveryReceipt to the sender asynchronously with a fresh context.
	go h.pushDeliveryReceipt(req.Topic, req.TopicSeq, req.MsgId)
}

// pushDeliveryReceipt pushes a delivery receipt to the message sender.
func (h *Handler) pushDeliveryReceipt(topic string, topicSeq uint64, msgID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Find the sender of the message.
	senderID, err := h.storage.GetMessageSender(ctx, topic, topicSeq)
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

	sent := h.manager.BroadcastToUser(senderID, receipt)
	h.log.Debugf("delivery receipt pushed: sender_id=%d, topic=%s, seq=%d, devices=%d",
		senderID, topic, topicSeq, sent)
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

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Update read status in storage.
	if err := h.storage.UpdateReadStatus(ctx, c.UserID(), req.Topic, req.UpToSeq); err != nil {
		h.log.Warnf("update read status failed: user_id=%d, topic=%s, up_to_seq=%d, err=%v",
			c.UserID(), req.Topic, req.UpToSeq, err)
		return
	}

	h.log.Debugf("read receipt received: user_id=%d, topic=%s, up_to_seq=%d",
		c.UserID(), req.Topic, req.UpToSeq)

	// Push ReadReceipt to the other party in the conversation asynchronously with a fresh context.
	go h.pushReadReceipt(c.UserID(), req.Topic, req.UpToSeq)
}

// pushReadReceipt pushes a read receipt to the other party in a conversation.
func (h *Handler) pushReadReceipt(readerID int64, topic string, upToSeq uint64) {
	// Determine recipient(s) based on topic type.
	// For P2P, the other user is the recipient.
	// For Group, broadcast to all online members (simplified: skip for now).
	topicType := msgworker.ParseTopicType(topic)

	var recipientIDs []int64
	switch topicType {
	case msgworker.TopicTypeP2P:
		uid1, uid2, err := msgworker.ExtractUserIDsFromP2PTopic(topic)
		if err != nil {
			h.log.Debugf("parse p2p topic for read receipt failed: topic=%s, err=%v", topic, err)
			return
		}
		if readerID == uid1 {
			recipientIDs = append(recipientIDs, uid2)
		} else {
			recipientIDs = append(recipientIDs, uid1)
		}
	case msgworker.TopicTypeGroup:
		// For groups, we could broadcast to all online members.
		// Simplified: skip group read receipts in Phase 2.
		return
	case msgworker.TopicTypeSystem:
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

	for _, recipientID := range recipientIDs {
		sent := h.manager.BroadcastToUser(recipientID, receipt)
		h.log.Debugf("read receipt pushed: recipient_id=%d, topic=%s, up_to_seq=%d, devices=%d",
			recipientID, topic, upToSeq, sent)
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
