package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"nonoka-im/internal/bot/domain"
	"nonoka-im/internal/topic"

	"github.com/go-kratos/kratos/v2/log"
)

const (
	defaultStreamTTL      = 10 * time.Minute
	defaultStreamMaxBytes = 64 << 10
	defaultChunkLimit     = 20
)

type StreamConfig struct {
	TTL        time.Duration
	MaxBytes   int
	ChunkLimit int
}

type StreamService struct {
	BotStore   Store
	Authz      Authorizer
	Streams    StreamStore
	Messages   MessageSender
	Recipients RecipientResolver
	Publisher  EventPublisher
	Log        *log.Helper
	Config     StreamConfig
	Now        func() time.Time
}

func NewStreamService(store Store, authz Authorizer, streams StreamStore, messages MessageSender, recipients RecipientResolver, publisher EventPublisher, logger log.Logger) *StreamService {
	return &StreamService{
		BotStore: store, Authz: authz, Streams: streams, Messages: messages,
		Recipients: recipients, Publisher: publisher, Log: log.NewHelper(logger),
	}
}

func (s *StreamService) log() *log.Helper {
	if s.Log != nil {
		return s.Log
	}
	return log.NewHelper(log.DefaultLogger)
}

func (s *StreamService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *StreamService) ttl() time.Duration {
	if s.Config.TTL > 0 {
		return s.Config.TTL
	}
	return defaultStreamTTL
}

func (s *StreamService) maxBytes() int {
	if s.Config.MaxBytes > 0 {
		return s.Config.MaxBytes
	}
	return defaultStreamMaxBytes
}

func (s *StreamService) chunkLimit() int {
	if s.Config.ChunkLimit > 0 {
		return s.Config.ChunkLimit
	}
	return defaultChunkLimit
}

// Create starts an ephemeral reply stream for one topic.
func (s *StreamService) Create(ctx context.Context, bot *domain.Bot, rawTopic, clientMsgID string) (*StreamState, error) {
	if bot == nil || !bot.Active() {
		return nil, domain.ErrDisabled
	}
	normalized, err := topic.NormalizeTopic(rawTopic)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid topic", ErrInvalidInput)
	}
	if s.Authz == nil {
		return nil, ErrForbidden
	}
	if err := s.Authz.CanAccessTopic(ctx, bot.BotUserID, normalized); err != nil {
		return nil, ErrForbidden
	}
	clientMsgID = strings.TrimSpace(clientMsgID)
	if clientMsgID == "" {
		clientMsgID = "stream-" + randomStreamID()
	}
	if len(clientMsgID) > 128 || strings.ContainsAny(clientMsgID, "\r\n\t") {
		return nil, fmt.Errorf("%w: invalid client_msg_id", ErrInvalidInput)
	}
	streamID := "bst_" + randomStreamID()
	state := StreamState{
		ID: streamID, BotID: bot.ID, BotUserID: bot.BotUserID,
		Topic: normalized, ClientMsgID: clientMsgID, State: StreamStateOpen,
		CreatedAt: s.now(),
	}
	if err := s.Streams.Create(ctx, state, s.ttl()); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return nil, ErrConflict
		}
		return nil, err
	}
	s.publish(ctx, StreamEvent{
		StreamID: streamID, Seq: 0, Type: "started", Topic: normalized,
		BotID: bot.ID, BotUserID: bot.BotUserID, ClientMsgID: clientMsgID,
		TimestampMs: s.now().UnixMilli(),
	})
	return &state, nil
}

// Append adds one delta and fans it out. Older or duplicate sequences are
// ignored by the stream store.
func (s *StreamService) Append(ctx context.Context, bot *domain.Bot, streamID string, seq uint64, delta string) (*StreamState, error) {
	if bot == nil || !bot.Active() {
		return nil, domain.ErrDisabled
	}
	if streamID == "" || seq == 0 || delta == "" {
		return nil, fmt.Errorf("%w: stream_id, seq and delta are required", ErrInvalidInput)
	}
	allowed, err := s.Streams.AllowChunk(ctx, streamID, s.chunkLimit())
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrRateLimited
	}
	state, err := s.Streams.Append(ctx, streamID, bot.ID, seq, delta, s.maxBytes(), s.ttl())
	if err != nil {
		return nil, mapStreamError(err)
	}
	s.publish(ctx, StreamEvent{
		StreamID: state.ID, Seq: seq, Type: "delta", Topic: state.Topic,
		BotID: state.BotID, BotUserID: state.BotUserID, Delta: delta,
		ClientMsgID: state.ClientMsgID, TimestampMs: s.now().UnixMilli(),
	})
	return state, nil
}

// Finalize persists the final message and closes the stream. Repeating it is
// idempotent because the client message id is stable.
func (s *StreamService) Finalize(ctx context.Context, bot *domain.Bot, streamID, content string) (SendResult, error) {
	if bot == nil || !bot.Active() {
		return SendResult{}, domain.ErrDisabled
	}
	content = strings.TrimSpace(content)
	if streamID == "" || content == "" {
		return SendResult{}, fmt.Errorf("%w: stream_id and content are required", ErrInvalidInput)
	}
	if len(content) > s.maxBytes() {
		return SendResult{}, fmt.Errorf("%w: content is too large", ErrInvalidInput)
	}
	state, err := s.Streams.MarkFinal(ctx, streamID, bot.ID)
	if err != nil {
		return SendResult{}, mapStreamError(err)
	}
	if s.Messages == nil {
		return SendResult{}, fmt.Errorf("%w: message sender unavailable", ErrInvalidInput)
	}
	result, err := s.Messages.Send(ctx, SendInput{
		ActorID: bot.BotUserID, Topic: state.Topic, Content: []byte(content),
		ClientMsgID: state.ClientMsgID,
	})
	if err != nil {
		return SendResult{}, err
	}
	s.publish(ctx, StreamEvent{
		StreamID: state.ID, Seq: state.Seq + 1, Type: "final", Topic: state.Topic,
		BotID: state.BotID, BotUserID: state.BotUserID, Delta: content,
		ClientMsgID: state.ClientMsgID, TimestampMs: s.now().UnixMilli(),
	})
	return result, nil
}

func (s *StreamService) Abort(ctx context.Context, bot *domain.Bot, streamID string) error {
	if bot == nil || !bot.Active() {
		return domain.ErrDisabled
	}
	state, err := s.Streams.Delete(ctx, streamID, bot.ID)
	if err != nil {
		return mapStreamError(err)
	}
	if state.State == StreamStateFinal {
		return nil
	}
	s.publish(ctx, StreamEvent{
		StreamID: state.ID, Seq: state.Seq + 1, Type: "aborted", Topic: state.Topic,
		BotID: state.BotID, BotUserID: state.BotUserID, ClientMsgID: state.ClientMsgID,
		TimestampMs: s.now().UnixMilli(),
	})
	return nil
}
