package app

import (
	"context"
	"errors"
	"sort"
	"time"

	"nonoka-im/internal/authz"
	"nonoka-im/internal/im/domain"

	"github.com/go-kratos/kratos/v2/log"
)

// PullRequest describes the two pull modes supported by the protocol.
type PullRequest struct {
	UserID  int64
	Topic   string
	LastSeq uint64
	EndSeq  uint64
	Limit   int
}

// PullResult is the transport-neutral result of a pull.
type PullResult struct {
	Messages []domain.InboxMessage
	HasMore  bool
	NextSeq  uint64
}

// QueryService owns the authorized read side of the IM bounded context.
type QueryService struct {
	Reader     MessageReader
	Authorizer authz.Authorizer
	Log        *log.Helper
}

func (s *QueryService) log() *log.Helper {
	if s.Log != nil {
		return s.Log
	}
	return log.NewHelper(log.DefaultLogger)
}

func (s *QueryService) authorize(ctx context.Context, userID int64, topic string) (string, error) {
	if userID <= 0 {
		return "", ErrAuthRequired
	}
	normalized, err := domain.NormalizeTopic(topic)
	if err != nil {
		return "", ErrInvalidTopic
	}
	if s.Authorizer == nil {
		return "", ErrTopicDenied
	}
	if err := s.Authorizer.CanAccessTopic(ctx, userID, normalized); err != nil {
		return "", ErrTopicDenied
	}
	return normalized, nil
}

// Pull returns a forward incremental page or a backward history page.
func (s *QueryService) Pull(ctx context.Context, req PullRequest) (PullResult, error) {
	if s.Reader == nil {
		return PullResult{}, errors.New("message reader unavailable")
	}
	topic, err := s.authorize(ctx, req.UserID, req.Topic)
	if err != nil {
		return PullResult{}, err
	}
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	topicType := domain.ParseTopicType(topic)
	backward := req.EndSeq > 0 || req.LastSeq == 0

	var messages []domain.InboxMessage
	var hasMore bool
	if backward {
		messages, hasMore, err = s.Reader.GetHistoryMessages(ctx, req.UserID, topic, req.EndSeq, limit)
		if err != nil {
			return PullResult{}, err
		}
	} else {
		switch topicType {
		case domain.TopicTypeP2P, domain.TopicTypeSystem:
			messages, err = s.Reader.GetOfflineMessages(ctx, req.UserID, topic, req.LastSeq, limit)
			if err != nil {
				return PullResult{}, err
			}
			hasMore = len(messages) >= limit
		case domain.TopicTypeGroup:
			groupMsgs, err := s.Reader.GetGroupMessages(ctx, topic, req.LastSeq, limit)
			if err != nil {
				return PullResult{}, err
			}
			mentionMsgs, err := s.Reader.GetMentionMessages(ctx, req.UserID, topic, req.LastSeq, limit)
			if err != nil {
				s.log().Warnf("get mention messages failed: user_id=%d topic=%s err=%v", req.UserID, topic, err)
				mentionMsgs = nil
			}
			messages = mergeGroupMessages(req.UserID, groupMsgs, mentionMsgs)
			if len(messages) > limit {
				messages = messages[:limit]
			}
			hasMore = len(messages) >= limit
		default:
			return PullResult{}, ErrInvalidTopic
		}
	}

	var nextSeq uint64
	if len(messages) > 0 {
		if backward {
			nextSeq = messages[0].TopicSeq
		} else {
			nextSeq = messages[len(messages)-1].TopicSeq + 1
		}
	}
	return PullResult{Messages: messages, HasMore: hasMore, NextSeq: nextSeq}, nil
}

func mergeGroupMessages(userID int64, groupMsgs []domain.Message, mentions []domain.MentionMessage) []domain.InboxMessage {
	merged := make(map[uint64]domain.InboxMessage, len(groupMsgs)+len(mentions))
	for _, msg := range groupMsgs {
		merged[msg.TopicSeq] = domain.InboxMessage{Message: msg, UserID: userID}
	}
	for _, mention := range mentions {
		if _, exists := merged[mention.TopicSeq]; !exists {
			merged[mention.TopicSeq] = domain.InboxMessage{Message: mention.Message, UserID: userID, Read: mention.Read, DeliveredAt: mention.DeliveredAt}
		}
	}
	out := make([]domain.InboxMessage, 0, len(merged))
	for _, msg := range merged {
		out = append(out, msg)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TopicSeq < out[j].TopicSeq })
	return out
}

// RecentMessages returns a bounded recent window for agent tools.
func (s *QueryService) RecentMessages(ctx context.Context, userID int64, topic string, limit int) ([]domain.InboxMessage, error) {
	topic, err := s.authorize(ctx, userID, topic)
	if err != nil {
		return nil, err
	}
	return s.Reader.GetRecentMessages(ctx, userID, topic, limit)
}

// MessageForUser loads one message after authorization.
func (s *QueryService) MessageForUser(ctx context.Context, userID int64, topic string, msgID int64) (domain.InboxMessage, error) {
	topic, err := s.authorize(ctx, userID, topic)
	if err != nil {
		return domain.InboxMessage{}, err
	}
	return s.Reader.GetMessageForUser(ctx, userID, topic, msgID)
}

// Recall removes a message within the configured window.
func (s *QueryService) Recall(ctx context.Context, userID int64, topic string, topicSeq uint64, msgID int64, window time.Duration) error {
	topic, err := s.authorize(ctx, userID, topic)
	if err != nil {
		return err
	}
	return s.Reader.RecallMessage(ctx, topic, topicSeq, msgID, userID, window)
}

// AckDelivery records a client delivery acknowledgement.
func (s *QueryService) AckDelivery(ctx context.Context, userID int64, topic string, topicSeq uint64) error {
	if userID <= 0 {
		return ErrAuthRequired
	}
	return s.Reader.UpdateDeliveryStatus(ctx, userID, topic, topicSeq)
}

// MarkRead updates the per-topic read position.
func (s *QueryService) MarkRead(ctx context.Context, userID int64, topic string, upToSeq uint64) error {
	if userID <= 0 {
		return ErrAuthRequired
	}
	return s.Reader.UpdateReadStatus(ctx, userID, topic, upToSeq)
}

// MessageSender resolves the sender for a delivery/read receipt.
func (s *QueryService) MessageSender(ctx context.Context, topic string, topicSeq uint64) (int64, error) {
	return s.Reader.GetMessageSender(ctx, topic, topicSeq)
}

// GetUserPresence is intentionally not part of this service; presence is a
// gateway/session concern.
