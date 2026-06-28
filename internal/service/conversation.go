package service

import (
	"context"
	"fmt"

	pb "nonoka-im/api/im/v1"
	"nonoka-im/internal/data"
	"nonoka-im/internal/msgworker"

	"github.com/go-kratos/kratos/v2/errors"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ConversationService provides conversation-related HTTP APIs.
type ConversationService struct {
	pb.UnimplementedConversationServiceServer

	repo data.ConversationRepo
}

// NewConversationService creates a new ConversationService.
func NewConversationService(repo data.ConversationRepo) *ConversationService {
	return &ConversationService{repo: repo}
}

// ListConversations returns the authenticated user's conversation list.
func (s *ConversationService) ListConversations(ctx context.Context, req *pb.ListConversationsRequest) (*pb.ListConversationsReply, error) {
	userID := extractUserIDFromContext(ctx)
	if userID == 0 {
		return nil, errors.Unauthorized("AUTH_REQUIRED", "authentication required")
	}

	limit := req.GetLimit()
	offset := req.GetOffset()

	rows, err := s.repo.ListConversations(ctx, userID, limit, offset)
	if err != nil {
		return nil, errors.InternalServer("DB_ERROR", fmt.Sprintf("list conversations failed: %v", err))
	}

	conversations := make([]*pb.Conversation, len(rows))
	for i, r := range rows {
		conversations[i] = &pb.Conversation{
			Topic:          r.Topic,
			Type:           r.Type,
			PeerId:         r.PeerID,
			PeerUsername:   r.PeerUsername,
			LastMsgPreview: r.LastMsgPreview,
			LastMsgAt:      r.LastMsgAt,
			LastSeq:        r.LastSeq,
			LastReadSeq:    r.LastReadSeq,
			UnreadCount:    r.UnreadCount,
		}
	}

	return &pb.ListConversationsReply{
		Conversations: conversations,
		HasMore:       len(rows) >= int(limit) && limit > 0,
	}, nil
}

// MarkConversationRead marks a conversation as read up to its current last_seq.
func (s *ConversationService) MarkConversationRead(ctx context.Context, req *pb.MarkConversationReadRequest) (*emptypb.Empty, error) {
	userID := extractUserIDFromContext(ctx)
	if userID == 0 {
		return nil, errors.Unauthorized("AUTH_REQUIRED", "authentication required")
	}

	topic := req.GetTopic()
	if topic == "" {
		return nil, errors.BadRequest("TOPIC_REQUIRED", "topic is required")
	}

	topicType := msgworker.ParseTopicType(topic)
	if topicType == msgworker.TopicTypeUnknown {
		return nil, errors.BadRequest("INVALID_TOPIC", "invalid topic format")
	}

	// Normalize P2P topics so both sides use the same conversation key.
	if topicType == msgworker.TopicTypeP2P {
		normalized, err := msgworker.NormalizeTopic(topic)
		if err != nil {
			return nil, errors.BadRequest("INVALID_TOPIC", err.Error())
		}
		topic = normalized
	}

	if err := s.repo.MarkRead(ctx, userID, topic, 0); err != nil {
		return nil, errors.InternalServer("DB_ERROR", fmt.Sprintf("mark read failed: %v", err))
	}
	return &emptypb.Empty{}, nil
}

