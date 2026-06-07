package sdk

import (
	"context"

	v1 "nonoka-im/api/im/v1"
)

// MessageService provides message-related HTTP APIs.
// It can be used independently without WebSocket, suitable for bots and server-side use.
type MessageService struct {
	client v1.MessageServiceHTTPClient
}

// newMessageService creates a new MessageService.
func newMessageService(svc *serviceClient) *MessageService {
	return &MessageService{
		client: v1.NewMessageServiceHTTPClient(svc.httpCli),
	}
}

// SendMessage sends a message via HTTP API.
// Use this when WebSocket is not available or for server-to-server communication.
func (s *MessageService) SendMessage(ctx context.Context, req *SendMessageRequest) (*SendResult, error) {
	reply, err := s.client.SendMessage(ctx, &v1.SendMessageRequest{
		Topic:            req.Topic,
		MsgType:          req.MsgType,
		Content:          req.Content,
		ClientMsgId:      req.ClientMsgID,
		MentionedUserIds: req.MentionedUserIDs,
	})
	if err != nil {
		return nil, err
	}
	return &SendResult{
		ClientMsgID: reply.ClientMsgId,
		MsgID:       reply.MsgId,
		Timestamp:   reply.Timestamp,
		TopicSeq:    reply.TopicSeq,
	}, nil
}

// SendMessageRequest is the request for sending a message via HTTP.
type SendMessageRequest struct {
	Topic            string
	MsgType          v1.MsgType
	Content          []byte
	ClientMsgID      string
	MentionedUserIDs []int64
}
