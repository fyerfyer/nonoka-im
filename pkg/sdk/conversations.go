package sdk

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	v1 "nonoka-im/api/im/v1"

	"google.golang.org/protobuf/types/known/emptypb"
)

type ConversationsResource struct {
	proto    v1.ConversationServiceHTTPClient
	http     *apiTransport
	messages *MessagesResource

	mu      sync.Mutex
	clients map[string]*ConversationClient
}

func (r *ConversationsResource) List(ctx context.Context, limit int32, offset int64) ([]Conversation, bool, error) {
	if limit <= 0 {
		limit = 50
	}
	reply, err := r.proto.ListConversations(ctx, &v1.ListConversationsRequest{Limit: limit, Offset: offset})
	if err != nil {
		return nil, false, apiErrorFromKratos(err)
	}
	out := make([]Conversation, 0, len(reply.GetConversations()))
	for _, conv := range reply.GetConversations() {
		out = append(out, Conversation{
			Topic: conv.GetTopic(), Type: conv.GetType(), PeerID: conv.GetPeerId(),
			PeerUsername: conv.GetPeerUsername(), LastMsgPreview: conv.GetLastMsgPreview(),
			LastMsgAt: conv.GetLastMsgAt(), LastSeq: conv.GetLastSeq(),
			LastReadSeq: conv.GetLastReadSeq(), UnreadCount: conv.GetUnreadCount(),
		})
	}
	return out, reply.GetHasMore(), nil
}

func (r *ConversationsResource) MarkRead(ctx context.Context, topic string) error {
	body := &v1.MarkConversationReadRequest{Topic: topic}
	return r.http.doProto(ctx, http.MethodPost, fmt.Sprintf("/v1/conversations/%s/read", topic), nil, body, &emptypb.Empty{})
}
