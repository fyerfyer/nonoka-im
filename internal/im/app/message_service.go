package app

import (
	"context"
	"fmt"
	"time"

	"nonoka-im/internal/im/domain"
	"nonoka-im/internal/messaging"
	"nonoka-im/internal/metrics"

	"github.com/go-kratos/kratos/v2/log"
	"go.opentelemetry.io/otel/trace"
)

const maxContentSize = 64 * 1024

// SendRequest is the transport-neutral input for sending a client message.
type SendRequest struct {
	ActorID      int64
	Topic        string
	Type         domain.MessageType
	Content      []byte
	ClientMsgID  string
	Timestamp    int64
	MentionedIDs []int64
}

// SendResult is returned after Kafka accepted the message.
type SendResult struct {
	ClientMsgID string
	Timestamp   int64
}

// HandleResult describes one persisted Kafka message.
type HandleResult struct {
	Message      domain.Message
	RecipientIDs []int64
	IsGroup      bool
	Duplicate    bool
}

// MessageService owns the write side of the IM bounded context: validation,
// topic sequencing, durable persistence and conversation fan-out.
type MessageService struct {
	Writer        MessageWriter
	IDs           IDGenerator
	Seqs          SeqGenerator
	Conversations ConversationRepository
	GroupMembers  GroupMemberReader
	Bots          BotDirectory
	Authorizer    TopicAuthorizer
	Producer      MessageProducer
	Attachments   AttachmentSharer
	AgentEvents   bool
	Metrics       *metrics.Metrics
	Log           *log.Helper
}

func (s *MessageService) log() *log.Helper {
	if s.Log != nil {
		return s.Log
	}
	return log.NewHelper(log.DefaultLogger)
}

// Send validates and forwards a client message to Kafka. Both the WebSocket
// gateway and the HTTP fallback call this method so their behavior cannot drift.
func (s *MessageService) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	if req.ActorID <= 0 {
		return SendResult{}, ErrAuthRequired
	}
	if req.ClientMsgID == "" {
		return SendResult{}, ErrClientMsgIDReq
	}
	normalized, err := domain.NormalizeTopic(req.Topic)
	if err != nil {
		return SendResult{}, fmt.Errorf("%w: %v", ErrInvalidTopic, err)
	}
	if len(req.Content) > maxContentSize {
		return SendResult{}, ErrMessageTooLarge
	}
	if allowed, handled := domain.AuthorizeTopicDirect(req.ActorID, normalized); handled {
		if !allowed {
			return SendResult{}, ErrTopicDenied
		}
	} else {
		if s.Authorizer == nil {
			return SendResult{}, fmt.Errorf("%w: authorization unavailable", ErrTopicDenied)
		}
		if err := s.Authorizer.CanAccessTopic(ctx, req.ActorID, normalized); err != nil {
			return SendResult{}, mapAuthzError(err)
		}
	}

	now := req.Timestamp
	if now == 0 {
		now = time.Now().Unix()
	}
	msg := &domain.Message{
		Topic: normalized, SenderID: req.ActorID, Type: req.Type,
		Content: req.Content, ClientMsgID: req.ClientMsgID, Timestamp: now,
		MentionedUserIDs: req.MentionedIDs,
	}
	if s.Producer == nil {
		return SendResult{}, ErrDeliveryFailed
	}
	// Establish attachment authorization before the message is accepted, so a
	// recipient who receives the push can open the file immediately. The
	// consumer repeats this idempotently to repair a crash between the share
	// and the Kafka write.
	if s.Attachments != nil {
		s.Attachments.ShareToTopic(ctx, req.ActorID, req.Type, req.Content, normalized)
	}
	if err := s.Producer.Produce(ctx, msg); err != nil {
		s.log().Errorf("produce to kafka failed: user_id=%d client_msg_id=%s err=%v", req.ActorID, req.ClientMsgID, err)
		return SendResult{}, fmt.Errorf("%w: %v", ErrDeliveryFailed, err)
	}
	return SendResult{ClientMsgID: req.ClientMsgID, Timestamp: now}, nil
}

func mapAuthzError(err error) error {
	// The application owns the denial contract; adapters may use any concrete
	// error type. Context cancellation is also surfaced as a denial because the
	// caller must not retry an unauthorized/aborted send.
	return fmt.Errorf("%w: %v", ErrTopicDenied, err)
}

// HandleIncoming persists one message consumed from Kafka and returns the
// delivery task for the async push pipeline.
func (s *MessageService) HandleIncoming(ctx context.Context, in domain.Message) (HandleResult, error) {
	start := time.Now()
	if s.Metrics != nil {
		s.Metrics.IncMessagesConsumed()
	}
	if s.IDs == nil || s.Seqs == nil || s.Writer == nil {
		return HandleResult{}, fmt.Errorf("message service dependencies unavailable")
	}

	msgID := s.IDs.NextID()
	topicSeq, err := s.Seqs.NextSeq(ctx, in.Topic)
	if err != nil {
		return HandleResult{}, fmt.Errorf("generate topic seq: %w", err)
	}
	in.ID = msgID
	in.TopicSeq = topicSeq
	if in.CreatedAt.IsZero() {
		in.CreatedAt = time.Now()
	}

	topicType := domain.ParseTopicType(in.Topic)
	// A group message may mention arbitrary user IDs supplied by the client.
	// Only actual group members may receive a mention notification or a
	// mention inbox copy; otherwise any member could leak the message content
	// to an outsider by @-mentioning them. Filter before the agent outbox so
	// non-member agent mentions cannot trigger a run either.
	var groupMembers []int64
	if topicType == domain.TopicTypeGroup {
		groupMembers, in.MentionedUserIDs = s.groupMentions(ctx, &in)
	}

	outbox, err := s.agentOutbox(ctx, &in)
	if err != nil {
		return HandleResult{}, err
	}

	var recipients []int64
	var duplicate bool
	result := HandleResult{IsGroup: topicType == domain.TopicTypeGroup}
	switch topicType {
	case domain.TopicTypeP2P:
		recipients, duplicate, err = s.Writer.SaveP2P(ctx, &in, outbox)
		if err != nil {
			return HandleResult{}, fmt.Errorf("save p2p message: %w", err)
		}
	case domain.TopicTypeGroup:
		duplicate, err = s.Writer.SaveGroup(ctx, &in, outbox)
		if err != nil {
			return HandleResult{}, fmt.Errorf("save group message: %w", err)
		}
		if len(in.MentionedUserIDs) > 0 {
			if err := s.Writer.SaveMentionInbox(ctx, &in, in.MentionedUserIDs); err != nil {
				return HandleResult{}, fmt.Errorf("save mention inbox: %w", err)
			}
		}
	case domain.TopicTypeSystem:
		recipients, duplicate, err = s.Writer.SaveSystem(ctx, &in)
		if err != nil {
			return HandleResult{}, fmt.Errorf("save system message: %w", err)
		}
	default:
		return HandleResult{}, fmt.Errorf("unknown topic type: %s", in.Topic)
	}

	// Attachment authorization is derived from the persisted message. Doing it
	// in the consumer (not the publish request) means a client disconnect after
	// Kafka acceptance cannot lose the topic share. Re-applying it on a
	// duplicate also repairs a share from an earlier partial attempt.
	if s.Attachments != nil {
		s.Attachments.ShareToTopic(ctx, in.SenderID, in.Type, in.Content, in.Topic)
	}

	if duplicate {
		if s.Metrics != nil {
			s.Metrics.IncMessagesDuplicate()
			s.Metrics.ObserveProcessingLatency(time.Since(start).Seconds())
		}
		result.Message = in
		result.RecipientIDs = recipients
		result.Duplicate = true
		return result, nil
	}

	// Redis owns the live sequence. The Mongo backup exists only for
	// recovery, so write it at a coarse watermark instead of on every message;
	// this removes one MongoDB write from the hot path.
	if in.TopicSeq <= 1 || in.TopicSeq%100 == 0 {
		if err := s.Writer.BackupTopicSeq(ctx, in.Topic, in.TopicSeq); err != nil {
			s.log().Warnf("backup topic seq failed: %v", err)
		}
	}
	s.upsertConversations(ctx, &in, recipients, topicType == domain.TopicTypeGroup, groupMembers)
	if s.Metrics != nil {
		s.Metrics.IncMessagesProcessed()
		s.Metrics.ObserveProcessingLatency(time.Since(start).Seconds())
	}
	result.Message = in
	result.RecipientIDs = recipients
	return result, nil
}

// agentOutbox decides whether this message can trigger an agent and builds the
// transactional outbox row with the shared messaging event contract.
func (s *MessageService) agentOutbox(ctx context.Context, msg *domain.Message) (*domain.OutboxEvent, error) {
	if !s.AgentEvents {
		return nil, nil
	}
	if s.Bots != nil && s.Bots.Ready() {
		trigger, err := s.messageCanTriggerAgent(ctx, msg)
		if err != nil {
			s.log().Warnf("bot lookup failed (topic=%s): %v", msg.Topic, err)
			trigger = true // fail open; the agent worker still re-checks authorization
		}
		if !trigger {
			return nil, nil
		}
	}
	ev := messaging.NewMessageEvent(msg.ID, msg.Topic, msg.SenderID, int32(msg.Type), msg.Timestamp, msg.MentionedUserIDs, msg.ClientMsgID)
	ev.TopicSeq = msg.TopicSeq
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		ev.TraceParent = fmt.Sprintf("00-%s-%s-%s", sc.TraceID(), sc.SpanID(), sc.TraceFlags())
	}
	payload, err := ev.Marshal()
	if err != nil {
		return nil, fmt.Errorf("marshal agent event: %w", err)
	}
	return &domain.OutboxEvent{
		EventID: ev.EventID, MsgID: msg.ID, Topic: msg.Topic, TopicSeq: msg.TopicSeq,
		Payload: payload, Status: domain.OutboxPending, NextAttemptAt: time.Now(), CreatedAt: time.Now(),
	}, nil
}

func (s *MessageService) messageCanTriggerAgent(ctx context.Context, msg *domain.Message) (bool, error) {
	switch domain.ParseTopicType(msg.Topic) {
	case domain.TopicTypeP2P:
		uid1, uid2, err := domain.ExtractUserIDsFromP2PTopic(msg.Topic)
		if err != nil {
			return false, nil
		}
		peer := uid1
		if msg.SenderID == uid1 {
			peer = uid2
		} else if msg.SenderID != uid2 {
			return false, nil
		}
		senderBot, err := s.Bots.IsBot(ctx, msg.SenderID)
		if err != nil {
			return false, err
		}
		if senderBot {
			return false, nil
		}
		return s.Bots.IsBot(ctx, peer)
	case domain.TopicTypeGroup:
		return s.Bots.AnyBot(ctx, msg.MentionedUserIDs)
	default:
		return false, nil
	}
}

func (s *MessageService) upsertConversations(ctx context.Context, msg *domain.Message, recipientIDs []int64, isGroup bool, groupMembers []int64) {
	if s.Conversations == nil {
		return
	}
	var userIDs []int64
	if isGroup {
		userIDs = append(userIDs, groupMembers...)
		// The sender may have left the group after publishing; keep their own
		// conversation summary consistent as well.
		userIDs = append(userIDs, msg.SenderID)
	} else {
		userIDs = append(userIDs, msg.SenderID)
		userIDs = append(userIDs, recipientIDs...)
	}
	if err := s.Conversations.UpsertConversations(ctx, userIDs, msg, msg.TopicSeq); err != nil {
		s.log().Warnf("upsert conversations failed: topic=%s users=%d err=%v", msg.Topic, len(userIDs), err)
	}
}

// groupMentions fetches the current member list once and filters mentions to
// members. It fails closed: if membership cannot be resolved, no mention
// notification is delivered. The returned member list is reused by the
// conversation fan-out so a group message triggers only one membership read.
func (s *MessageService) groupMentions(ctx context.Context, msg *domain.Message) ([]int64, []int64) {
	groupID, err := domain.ExtractGroupID(msg.Topic)
	if err != nil {
		s.log().Warnf("extract group id for mention filtering failed: %v", err)
		return nil, nil
	}
	if s.GroupMembers == nil {
		// Tests and simple embedders may not provide a membership reader. The
		// production msgworker always wires one; without it we cannot verify
		// mentions, so preserve the legacy pass-through instead of silently
		// dropping notifications.
		return nil, msg.MentionedUserIDs
	}
	members, err := s.GroupMembers.GetGroupMembers(ctx, groupID)
	if err != nil {
		s.log().Warnf("load group members for mention filtering failed: group=%s err=%v", groupID, err)
		return nil, nil
	}
	return members, filterMentions(members, msg.MentionedUserIDs)
}

// filterMentions keeps only IDs present in members, preserving order and
// removing duplicates.
func filterMentions(members, mentioned []int64) []int64 {
	if len(mentioned) == 0 {
		return nil
	}
	memberSet := make(map[int64]struct{}, len(members))
	for _, id := range members {
		if id > 0 {
			memberSet[id] = struct{}{}
		}
	}
	out := make([]int64, 0, len(mentioned))
	seen := make(map[int64]struct{}, len(mentioned))
	for _, id := range mentioned {
		if id <= 0 {
			continue
		}
		if _, ok := memberSet[id]; !ok {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
