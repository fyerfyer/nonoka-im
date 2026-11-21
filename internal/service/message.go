package service

import (
	"context"
	"errors"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/im/app"
	"nonoka-im/internal/im/domain"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	jwtMiddleware "github.com/go-kratos/kratos/v2/middleware/auth/jwt"
	jwt5 "github.com/golang-jwt/jwt/v5"
)

// MessageService exposes the HTTP message fallback. All business logic lives
// in internal/im/app so the WebSocket gateway and HTTP API cannot drift.
type MessageService struct {
	v1.UnimplementedMessageServiceServer

	sender *app.MessageService
	query  *app.QueryService
	log    *log.Helper
}

func NewMessageService(sender *app.MessageService, query *app.QueryService, logger log.Logger) *MessageService {
	return &MessageService{sender: sender, query: query, log: log.NewHelper(logger)}
}

func (s *MessageService) SendMessage(ctx context.Context, req *v1.SendMessageRequest) (*v1.SendMessageReply, error) {
	if req.Topic == "" {
		return nil, kerrors.BadRequest("TOPIC_REQUIRED", "topic is required")
	}
	if req.ClientMsgId == "" {
		return nil, kerrors.BadRequest("CLIENT_MSG_ID_REQUIRED", "client_msg_id is required")
	}
	userID := extractUserIDFromContext(ctx)
	result, err := s.sender.Send(ctx, app.SendRequest{
		ActorID: userID, Topic: req.Topic, Type: domain.MessageType(req.MsgType),
		Content: req.Content, ClientMsgID: req.ClientMsgId, MentionedIDs: req.MentionedUserIds,
	})
	if err != nil {
		return nil, messageError(err)
	}
	return &v1.SendMessageReply{ClientMsgId: result.ClientMsgID, Timestamp: result.Timestamp}, nil
}

func (s *MessageService) PullMessages(ctx context.Context, req *v1.PullRequest) (*v1.PullReply, error) {
	if req.Topic == "" {
		return &v1.PullReply{}, nil
	}
	result, err := s.query.Pull(ctx, app.PullRequest{
		UserID: extractUserIDFromContext(ctx), Topic: req.Topic,
		LastSeq: req.LastSeq, EndSeq: req.EndSeq, Limit: int(req.Limit),
	})
	if err != nil {
		if errors.Is(err, app.ErrAuthRequired) {
			return nil, kerrors.Unauthorized("AUTH_REQUIRED", "authentication required")
		}
		if errors.Is(err, app.ErrTopicDenied) || errors.Is(err, app.ErrInvalidTopic) {
			return nil, kerrors.Forbidden("FORBIDDEN", "topic access denied")
		}
		s.log.Warnf("pull messages failed: user_id=%d topic=%s err=%v", extractUserIDFromContext(ctx), req.Topic, err)
		return &v1.PullReply{}, nil
	}
	messages := make([]*v1.PullMessage, len(result.Messages))
	for i, msg := range result.Messages {
		messages[i] = &v1.PullMessage{
			MsgId: msg.ID, Topic: msg.Topic, SenderId: msg.SenderID, MsgType: int32(msg.Type),
			Content: msg.Content, Timestamp: msg.Timestamp, TopicSeq: msg.TopicSeq,
			ClientMsgId: msg.ClientMsgID, Recalled: msg.Recalled,
		}
	}
	return &v1.PullReply{Messages: messages, HasMore: result.HasMore, NextSeq: result.NextSeq}, nil
}

func messageError(err error) error {
	switch {
	case errors.Is(err, app.ErrAuthRequired):
		return kerrors.Unauthorized("AUTH_REQUIRED", "authentication required")
	case errors.Is(err, app.ErrClientMsgIDReq):
		return kerrors.BadRequest("CLIENT_MSG_ID_REQUIRED", "client_msg_id is required")
	case errors.Is(err, app.ErrInvalidTopic):
		return kerrors.BadRequest("INVALID_TOPIC", "invalid topic")
	case errors.Is(err, app.ErrTopicDenied):
		return kerrors.Forbidden("FORBIDDEN", "topic access denied")
	case errors.Is(err, app.ErrMessageTooLarge):
		return kerrors.BadRequest("MESSAGE_TOO_LARGE", "message too large")
	case errors.Is(err, app.ErrDeliveryFailed):
		return kerrors.InternalServer("DELIVERY_FAILED", "message delivery failed")
	default:
		return kerrors.InternalServer("INTERNAL", err.Error())
	}
}

// extractUserIDFromContext extracts user_id from JWT claims in context.
func extractUserIDFromContext(ctx context.Context) int64 {
	claims, ok := jwtMiddleware.FromContext(ctx)
	if !ok {
		return 0
	}
	if mapClaims, ok := claims.(jwt5.MapClaims); ok {
		if uid, ok := mapClaims["user_id"].(float64); ok {
			return int64(uid)
		}
	}
	return 0
}
