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

// PullMessages pulls messages via HTTP fallback.
func (s *MessageService) PullMessages(ctx context.Context, req *PullMessagesRequest) (*PullResult, error) {
	reply, err := s.client.PullMessages(ctx, &v1.PullRequest{
		Topic:   req.Topic,
		LastSeq: req.LastSeq,
		Limit:   req.Limit,
		EndSeq:  req.EndSeq,
	})
	if err != nil {
		return nil, err
	}
	return &PullResult{
		Messages: toSDKMessages(reply.Messages),
		HasMore:  reply.HasMore,
		NextSeq:  reply.NextSeq,
	}, nil
}

// PullMessagesRequest is the request for pulling messages via HTTP.
//
// Forward (incremental) pull: set LastSeq > 0, EndSeq == 0 to fetch messages
// with seq > LastSeq. Backward (history) page: set EndSeq > 0 (exclusive
// upper bound), or leave both zero to fetch the latest page; continue paging
// with EndSeq = oldest returned topic_seq.
type PullMessagesRequest struct {
	Topic   string
	LastSeq uint64
	EndSeq  uint64
	Limit   int32
}

// SendMessageRequest is the request for sending a message via HTTP.
type SendMessageRequest struct {
	Topic            string
	MsgType          v1.MsgType
	Content          []byte
	ClientMsgID      string
	MentionedUserIDs []int64
}
