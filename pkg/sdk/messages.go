package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	v1 "nonoka-im/api/im/v1"

	"github.com/google/uuid"
)

type cachedSend struct {
	result  SendResult
	expires time.Time
}

type MessagesResource struct {
	http   *apiTransport
	client *Client

	mu    sync.Mutex
	cache map[string]cachedSend
}

// Send persists one message with a stable client message ID. Network/429/5xx
// failures are retried with the same ID, so the server deduplicates them.
// NewClientMessageID returns a stable idempotency key for a send.
func NewClientMessageID() string { return uuid.NewString() }

func (r *MessagesResource) Send(ctx context.Context, req SendRequest) (*SendResult, error) {
	clientID := req.ClientMessageID
	if clientID == "" {
		clientID = NewClientMessageID()
	}
	if req.Type == MessageTypeUnspecified {
		req.Type = MessageTypeText
	}
	if cached := r.cached(clientID); cached != nil {
		return cached, nil
	}
	var lastErr error
	if r.client != nil && r.client.Realtime != nil && r.client.Realtime.IsConnected() {
		result, err := r.sendWebSocket(ctx, req, clientID)
		if err == nil && result != nil && result.MessageID != 0 {
			r.remember(clientID, result)
			return result, nil
		}
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, &SendError{ClientMessageID: clientID, Err: err}
			}
			// Only connection-class failures fall back to HTTP. A server
			// validation/authorization error must be returned as-is.
			if !errors.Is(err, ErrNotConnected) && !errors.Is(err, ErrConnectionClosed) && !errors.Is(err, ErrRequestTimeout) {
				return nil, &SendError{ClientMessageID: clientID, Err: err}
			}
			// The WebSocket may have dropped after the client observed it as
			// connected. The same client message ID makes the HTTP fallback
			// idempotent, so retry through the durable API instead of failing.
			lastErr = err
		}
	}
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<attempt) * 250 * time.Millisecond
			var apiErr *APIError
			if errors.As(lastErr, &apiErr) && apiErr.RetryAfter > 0 {
				delay = apiErr.RetryAfter
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
		var reply v1.SendMessageReply
		err := r.http.doProto(ctx, http.MethodPost, "/v1/message/send", nil, &v1.SendMessageRequest{
			Topic: req.Topic, MsgType: v1.MsgType(req.Type), Content: req.Content,
			ClientMsgId: clientID, MentionedUserIds: req.MentionedUserIDs,
		}, &reply)
		if err == nil {
			result := &SendResult{
				ClientMessageID: reply.GetClientMsgId(), MessageID: reply.GetMsgId(),
				Timestamp: reply.GetTimestamp(), TopicSeq: reply.GetTopicSeq(),
				Confirmed: reply.GetMsgId() != 0,
			}
			r.remember(clientID, result)
			return result, nil
		}
		lastErr = err
		var apiErr *APIError
		if !errors.As(lastErr, &apiErr) || !apiErr.Retryable {
			return nil, &SendError{ClientMessageID: clientID, Err: lastErr}
		}
	}
	return nil, &SendError{ClientMessageID: clientID, Err: lastErr}
}

func (r *MessagesResource) SendText(ctx context.Context, topic, text string) (*SendResult, error) {
	return r.Send(ctx, SendRequest{Topic: topic, Type: MessageTypeText, Content: []byte(text)})
}

func (r *MessagesResource) SendImage(ctx context.Context, topic, url string) (*SendResult, error) {
	return r.SendMedia(ctx, topic, MessageTypeImage, MediaContent{URL: url})
}

func (r *MessagesResource) SendFile(ctx context.Context, topic, url string) (*SendResult, error) {
	return r.SendMedia(ctx, topic, MessageTypeFile, MediaContent{URL: url})
}

// SendMedia sends an IMAGE/FILE message using the server's JSON media
// contract. Prefer the metadata returned by Files.Upload so file_id is
// present and attachment authorization can be established.
func (r *MessagesResource) SendMedia(ctx context.Context, topic string, typ MessageType, media MediaContent) (*SendResult, error) {
	content, err := json.Marshal(media)
	if err != nil {
		return nil, err
	}
	return r.Send(ctx, SendRequest{Topic: topic, Type: typ, Content: content})
}

func (r *MessagesResource) Pull(ctx context.Context, req PullRequest) (*PullResult, error) {
	if req.Limit <= 0 {
		req.Limit = 20
	}
	query := url.Values{}
	query.Set("topic", req.Topic)
	query.Set("lastSeq", strconv.FormatUint(req.LastSeq, 10))
	query.Set("limit", strconv.Itoa(int(req.Limit)))
	query.Set("endSeq", strconv.FormatUint(req.EndSeq, 10))
	var reply v1.PullReply
	if err := r.http.doProto(ctx, http.MethodGet, "/v1/message/pull", query, nil, &reply); err != nil {
		return nil, err
	}
	out := make([]*Message, 0, len(reply.GetMessages()))
	for _, item := range reply.GetMessages() {
		out = append(out, &Message{
			ID: item.GetMsgId(), Topic: item.GetTopic(), SenderID: item.GetSenderId(),
			Type: MessageType(item.GetMsgType()), Content: item.GetContent(),
			Timestamp: item.GetTimestamp(), TopicSeq: item.GetTopicSeq(),
			ClientMessageID: item.GetClientMsgId(), Recalled: item.GetRecalled(),
			Status: MessageStatusDelivered,
		})
	}
	return &PullResult{Messages: out, HasMore: reply.GetHasMore(), NextSeq: reply.GetNextSeq()}, nil
}

// Search performs a keyword search inside one topic. It returns
// ErrSearchUnavailable when the deployment has no search backend.
func (r *MessagesResource) Search(ctx context.Context, req SearchRequest) (*SearchResult, error) {
	if req.Limit <= 0 {
		req.Limit = 20
	}
	query := url.Values{}
	query.Set("topic", req.Topic)
	query.Set("query", req.Query)
	query.Set("senderId", strconv.FormatInt(req.SenderID, 10))
	query.Set("since", strconv.FormatInt(req.Since, 10))
	query.Set("until", strconv.FormatInt(req.Until, 10))
	query.Set("hasFile", strconv.FormatBool(req.HasFile))
	query.Set("limit", strconv.Itoa(int(req.Limit)))
	query.Set("offset", strconv.Itoa(int(req.Offset)))
	var reply v1.SearchMessagesReply
	if err := r.http.doProto(ctx, http.MethodGet, "/v1/message/search", query, nil, &reply); err != nil {
		if IsCode(err, "SEARCH_UNAVAILABLE") {
			return nil, fmt.Errorf("%w: %v", ErrSearchUnavailable, err)
		}
		return nil, err
	}
	out := &SearchResult{HasMore: reply.GetHasMore(), Hits: make([]*SearchHit, 0, len(reply.GetMessages()))}
	for _, hit := range reply.GetMessages() {
		out.Hits = append(out.Hits, &SearchHit{
			MessageID: hit.GetMsgId(), Topic: hit.GetTopic(), TopicSeq: hit.GetTopicSeq(),
			SenderID: hit.GetSenderId(), Timestamp: hit.GetTimestamp(),
			MsgType: MessageType(hit.GetMsgType()), HasFile: hit.GetHasFile(), Snippet: hit.GetSnippet(),
		})
	}
	return out, nil
}

// sendWebSocket publishes through the live connection and waits for the
// server's send receipt, which carries the durable msg_id and topic_seq.
func (r *MessagesResource) sendWebSocket(ctx context.Context, req SendRequest, clientID string) (*SendResult, error) {
	receipts, cancel := r.client.Realtime.waitSendReceipt(clientID)
	defer cancel()
	packet := &v1.Packet{
		Cmd: v1.Command_CMD_PUBLISH, Seq: r.client.Realtime.nextSeq(),
		Payload: &v1.Packet_SendReq{SendReq: &v1.SendMessageRequest{
			Topic: req.Topic, MsgType: v1.MsgType(req.Type), Content: req.Content,
			ClientMsgId: clientID, MentionedUserIds: req.MentionedUserIDs,
		}},
	}
	if _, err := r.client.Realtime.request(ctx, packet, 5*time.Second); err != nil {
		return nil, err
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			if result := r.lookupSent(ctx, req.Topic, clientID); result != nil {
				return result, nil
			}
			return &SendResult{ClientMessageID: clientID, Timestamp: time.Now().Unix()}, nil
		case receipt, ok := <-receipts:
			if !ok {
				return nil, ErrConnectionClosed
			}
			if receipt != nil {
				return &SendResult{
					ClientMessageID: receipt.ClientMessageID,
					MessageID:       receipt.MessageID,
					TopicSeq:        receipt.TopicSeq,
					Timestamp:       receipt.Timestamp,
					Confirmed:       true,
				}, nil
			}
		}
	}
}

func (r *MessagesResource) cached(clientID string) *SendResult {
	if clientID == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.cache[clientID]
	if !ok || time.Now().After(entry.expires) {
		if ok {
			delete(r.cache, clientID)
		}
		return nil
	}
	copy := entry.result
	return &copy
}

func (r *MessagesResource) remember(clientID string, result *SendResult) {
	if clientID == "" || result == nil || result.MessageID == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cache == nil {
		r.cache = map[string]cachedSend{}
	}
	if len(r.cache) > 4096 {
		now := time.Now()
		for key, entry := range r.cache {
			if now.After(entry.expires) {
				delete(r.cache, key)
			}
		}
	}
	r.cache[clientID] = cachedSend{result: *result, expires: time.Now().Add(30 * time.Second)}
}

// lookupSent is a fallback for a lost send receipt. The message may already be
// durably persisted even when the realtime receipt was dropped.
func (r *MessagesResource) lookupSent(ctx context.Context, topic, clientID string) *SendResult {
	for attempt := 0; attempt < 25; attempt++ {
		page, err := r.Pull(ctx, PullRequest{Topic: topic, Limit: 50})
		if err == nil {
			for _, message := range page.Messages {
				if message.ClientMessageID == clientID && message.ID != 0 {
					return &SendResult{
						ClientMessageID: clientID, MessageID: message.ID,
						TopicSeq: message.TopicSeq, Timestamp: message.Timestamp, Confirmed: true,
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(200 * time.Millisecond):
		}
	}
	return nil
}
