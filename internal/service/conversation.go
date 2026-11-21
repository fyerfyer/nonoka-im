package service

import (
	"context"
	stderrors "errors"

	pb "nonoka-im/api/im/v1"
	"nonoka-im/internal/im/app"

	"github.com/go-kratos/kratos/v2/errors"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ConversationService provides conversation-related HTTP APIs.
type ConversationService struct {
	pb.UnimplementedConversationServiceServer
	uc *app.ConversationService
}

func NewConversationService(uc *app.ConversationService) *ConversationService {
	return &ConversationService{uc: uc}
}

func (s *ConversationService) ListConversations(ctx context.Context, req *pb.ListConversationsRequest) (*pb.ListConversationsReply, error) {
	userID := extractUserIDFromContext(ctx)
	rows, err := s.uc.List(ctx, userID, req.GetLimit(), req.GetOffset())
	if err != nil {
		if stderrors.Is(err, app.ErrAuthRequired) {
			return nil, errors.Unauthorized("AUTH_REQUIRED", "authentication required")
		}
		return nil, errors.InternalServer("DB_ERROR", "list conversations failed")
	}
	conversations := make([]*pb.Conversation, len(rows))
	for i, row := range rows {
		conversations[i] = &pb.Conversation{
			Topic: row.Topic, Type: row.Type, PeerId: row.PeerID, PeerUsername: row.PeerUsername,
			LastMsgPreview: row.LastMsgPreview, LastMsgAt: row.LastMsgAt, LastSeq: row.LastSeq,
			LastReadSeq: row.LastReadSeq, UnreadCount: row.UnreadCount,
		}
	}
	return &pb.ListConversationsReply{
		Conversations: conversations,
		HasMore:       len(rows) >= int(req.GetLimit()) && req.GetLimit() > 0,
	}, nil
}

func (s *ConversationService) MarkConversationRead(ctx context.Context, req *pb.MarkConversationReadRequest) (*emptypb.Empty, error) {
	if req.GetTopic() == "" {
		return nil, errors.BadRequest("TOPIC_REQUIRED", "topic is required")
	}
	if err := s.uc.MarkRead(ctx, extractUserIDFromContext(ctx), req.GetTopic(), 0); err != nil {
		switch {
		case stderrors.Is(err, app.ErrAuthRequired):
			return nil, errors.Unauthorized("AUTH_REQUIRED", "authentication required")
		case stderrors.Is(err, app.ErrInvalidTopic):
			return nil, errors.BadRequest("INVALID_TOPIC", "invalid topic format")
		default:
			return nil, errors.InternalServer("DB_ERROR", "mark read failed")
		}
	}
	return &emptypb.Empty{}, nil
}
