package botsdk

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// NewStream starts an ephemeral streaming reply.
func (c *Client) NewStream(ctx context.Context, topic string, opts ...SendOption) (*Stream, error) {
	options := sendOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	if strings.TrimSpace(topic) == "" {
		return nil, fmt.Errorf("botsdk: topic is required")
	}
	clientMsgID := options.clientMsgID
	if clientMsgID == "" {
		clientMsgID = newClientMsgID()
	}
	var response struct {
		StreamID    string `json:"stream_id"`
		ClientMsgID string `json:"client_msg_id"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/v1/bot/streams", map[string]string{
		"topic": topic, "client_msg_id": clientMsgID,
	}, &response); err != nil {
		return nil, err
	}
	return &Stream{
		client: c, id: response.StreamID, topic: topic, clientMsgID: clientMsgID,
	}, nil
}

// Stream buffers deltas and posts them in small batches. Call Finalize with the
// complete text when generation is done.
type Stream struct {
	client      *Client
	id          string
	topic       string
	clientMsgID string

	mu     sync.Mutex
	seq    uint64
	buffer strings.Builder
	closed bool
}

func (s *Stream) ID() string          { return s.id }
func (s *Stream) ClientMsgID() string { return s.clientMsgID }

// Write appends a delta. It flushes automatically once the buffer reaches
// about 256 bytes.
func (s *Stream) Write(ctx context.Context, delta string) error {
	if delta == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("botsdk: stream is closed")
	}
	s.buffer.WriteString(delta)
	if s.buffer.Len() >= 256 {
		return s.flushLocked(ctx)
	}
	return nil
}

// Flush posts the buffered deltas immediately.
func (s *Stream) Flush(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("botsdk: stream is closed")
	}
	return s.flushLocked(ctx)
}

func (s *Stream) flushLocked(ctx context.Context) error {
	if s.buffer.Len() == 0 {
		return nil
	}
	delta := s.buffer.String()
	s.seq++
	var response struct {
		StreamID string `json:"stream_id"`
		Seq      uint64 `json:"seq"`
	}
	if err := s.client.doJSON(ctx, http.MethodPost, "/v1/bot/streams/"+s.id+"/chunks", map[string]any{
		"seq": s.seq, "delta": delta,
	}, &response); err != nil {
		s.seq--
		return err
	}
	s.buffer.Reset()
	return nil
}

// Finalize flushes pending deltas and persists the final message. If the
// stream expired, it falls back to a normal send with the same client message
// id, so the operation stays idempotent.
func (s *Stream) Finalize(ctx context.Context, content string) (SendResult, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return SendResult{}, errors.New("botsdk: stream is closed")
	}
	if err := s.flushLocked(ctx); err != nil {
		s.mu.Unlock()
		return SendResult{}, err
	}
	s.closed = true
	s.mu.Unlock()

	var accepted struct {
		ClientMsgID string `json:"client_msg_id"`
		AcceptedAt  string `json:"accepted_at"`
	}
	err := s.client.doJSON(ctx, http.MethodPost, "/v1/bot/streams/"+s.id+"/finalize", map[string]string{
		"content": content,
	}, &accepted)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Code == "STREAM_EXPIRED" {
			return s.client.Send(ctx, SendRequest{
				Topic: s.topic, ClientMsgID: s.clientMsgID, Content: content,
			})
		}
		return SendResult{}, err
	}
	result := SendResult{ClientMsgID: accepted.ClientMsgID}
	if result.ClientMsgID == "" {
		result.ClientMsgID = s.clientMsgID
	}
	if s.client.receiptTimeout > 0 {
		if receipt, err := s.client.waitReceipt(ctx, s.topic, result.ClientMsgID, s.client.receiptTimeout); err == nil && receipt != nil {
			result.MessageID = receipt.MessageID
			result.TopicSeq = receipt.TopicSeq
		}
	}
	return result, nil
}

// Abort removes the streaming bubble on connected clients.
func (s *Stream) Abort(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	resp, err := s.client.do(ctx, http.MethodDelete, "/v1/bot/streams/"+s.id, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent || (resp.StatusCode >= 200 && resp.StatusCode < 300) {
		return nil
	}
	return decodeAPIError(resp)
}
