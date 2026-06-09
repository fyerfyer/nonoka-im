package service

import (
	"context"
	"sort"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/msgworker"

	"github.com/go-kratos/kratos/v2/log"
)

// MessageService provides message-related HTTP APIs including pull fallback.
type MessageService struct {
	v1.UnimplementedMessageServiceServer
	storage *msgworker.MessageStorage
	log     *log.Helper
}

// NewMessageService creates a new MessageService.
func NewMessageService(storage *msgworker.MessageStorage, logger log.Logger) *MessageService {
	return &MessageService{
		storage: storage,
		log:     log.NewHelper(logger),
	}
}

// SendMessage forwards send requests to the gateway via Kafka.
func (s *MessageService) SendMessage(ctx context.Context, req *v1.SendMessageRequest) (*v1.SendMessageReply, error) {
	// TODO: integrate with Kafka producer for HTTP-based message sending.
	// For now, this is a placeholder that returns unimplemented.
	return nil, nil
}

// PullMessages retrieves offline messages via HTTP fallback.
// It handles both P2P/system (inbox) and group (read扩散) topics.
func (s *MessageService) PullMessages(ctx context.Context, req *v1.PullRequest) (*v1.PullReply, error) {
	if req.Topic == "" {
		return &v1.PullReply{}, nil
	}

	// Extract user_id from context (set by JWT middleware)
	userID := extractUserIDFromContext(ctx)
	if userID == 0 {
		return &v1.PullReply{}, nil
	}

	limit := int(req.Limit)
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	topicType := msgworker.ParseTopicType(req.Topic)
	var messages []*v1.PullMessage
	var hasMore bool

	switch topicType {
	case msgworker.TopicTypeP2P, msgworker.TopicTypeSystem:
		msgs, err := s.storage.GetOfflineMessages(ctx, userID, req.Topic, req.LastSeq, limit)
		if err != nil {
			s.log.Warnf("get offline messages failed: user_id=%d, topic=%s, err=%v", userID, req.Topic, err)
			return &v1.PullReply{}, nil
		}
		messages = inboxToPullMessages(msgs)
		hasMore = len(msgs) >= limit && limit > 0

	case msgworker.TopicTypeGroup:
		groupMsgs, err := s.storage.GetGroupMessages(ctx, req.Topic, req.LastSeq, limit)
		if err != nil {
			s.log.Warnf("get group messages failed: user_id=%d, topic=%s, err=%v", userID, req.Topic, err)
			return &v1.PullReply{}, nil
		}
		mentionMsgs, err := s.storage.GetMentionMessages(ctx, userID, req.Topic, req.LastSeq, limit)
		if err != nil {
			mentionMsgs = nil // Non-fatal
		}
		messages = mergeAndSortMessages(groupMsgs, mentionMsgs)
		if len(messages) > limit && limit > 0 {
			messages = messages[:limit]
		}
		hasMore = len(messages) >= limit && limit > 0

	default:
		s.log.Warnf("invalid topic type for pull: topic=%s", req.Topic)
		return &v1.PullReply{}, nil
	}

	var nextSeq uint64
	if len(messages) > 0 {
		nextSeq = messages[len(messages)-1].TopicSeq + 1
	}

	return &v1.PullReply{
		Messages: messages,
		HasMore:  hasMore,
		NextSeq:  nextSeq,
	}, nil
}

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

func mergeAndSortMessages(groupMsgs []*msgworker.StoredMessage, mentionMsgs []*msgworker.MentionMessage) []*v1.PullMessage {
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
	sort.Slice(result, func(i, j int) bool {
		return result[i].TopicSeq < result[j].TopicSeq
	})
	return result
}

// extractUserIDFromContext extracts user_id from JWT claims in context.
func extractUserIDFromContext(ctx context.Context) int64 {
	// The JWT middleware in kratos stores claims in context.
	// We attempt to extract the user_id claim.
	// This is a best-effort extraction; production code should use typed context values.
	type claimsKey struct{}
	if claims, ok := ctx.Value(claimsKey{}).(map[string]interface{}); ok {
		if uid, ok := claims["user_id"].(float64); ok {
			return int64(uid)
		}
	}
	return 0
}
