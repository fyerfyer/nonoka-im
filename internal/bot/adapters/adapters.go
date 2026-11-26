// Package adapters wires external-bot ports to IM, authz and realtime
// implementations. It is a composition adapter, not a business layer.
package adapters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"nonoka-im/internal/agent/credential"
	agentmcp "nonoka-im/internal/agent/mcp"
	"nonoka-im/internal/biz"
	botapp "nonoka-im/internal/bot/app"
	imapp "nonoka-im/internal/im/app"
	imdomain "nonoka-im/internal/im/domain"
	"nonoka-im/internal/realtime"
	"nonoka-im/internal/topic"
)

// Reader adapts the IM application query service to bot runtime reads.
type Reader struct {
	Query *imapp.QueryService
}

func (r *Reader) MessageByClientID(ctx context.Context, userID int64, topic, clientMsgID string) (botapp.Message, error) {
	row, err := r.Query.MessageByClientID(ctx, userID, topic, clientMsgID)
	if err != nil {
		return botapp.Message{}, mapQueryError(err)
	}
	return fromIM(row), nil
}

func (r *Reader) RecentMessages(ctx context.Context, userID int64, topic string, limit int, beforeSeq uint64) ([]botapp.Message, error) {
	rows, err := r.Query.MessageHistory(ctx, userID, topic, limit, beforeSeq)
	if err != nil {
		return nil, mapQueryError(err)
	}
	out := make([]botapp.Message, 0, len(rows))
	for i := range rows {
		out = append(out, fromIM(rows[i]))
	}
	return out, nil
}

func (r *Reader) MessageForUser(ctx context.Context, userID int64, topic string, msgID int64) (botapp.Message, error) {
	row, err := r.Query.MessageForUser(ctx, userID, topic, msgID)
	if err != nil {
		return botapp.Message{}, mapQueryError(err)
	}
	return fromIM(row), nil
}

func (r *Reader) LoadMessage(ctx context.Context, userID int64, topic string, msgID int64) (botapp.Message, error) {
	row, err := r.Query.MessageForUser(ctx, userID, topic, msgID)
	if err != nil {
		return botapp.Message{}, mapQueryError(err)
	}
	return fromIM(row), nil
}

func mapQueryError(err error) error {
	switch {
	case errors.Is(err, imdomain.ErrNotFound):
		return botapp.ErrNotFound
	case errors.Is(err, imapp.ErrTopicDenied), errors.Is(err, imapp.ErrInvalidTopic), errors.Is(err, imapp.ErrAuthRequired):
		return botapp.ErrForbidden
	default:
		return err
	}
}

func fromIM(msg imdomain.InboxMessage) botapp.Message {
	return botapp.Message{
		ID: msg.ID, Topic: msg.Topic, TopicSeq: msg.TopicSeq, SenderID: msg.SenderID,
		Type: int32(msg.Type), Content: msg.Content, Timestamp: msg.Timestamp,
	}
}

// Sender adapts the IM write service to bot replies.
type Sender struct{ Messages *imapp.MessageService }

func (s *Sender) Send(ctx context.Context, in botapp.SendInput) (botapp.SendResult, error) {
	if s.Messages == nil {
		return botapp.SendResult{}, fmt.Errorf("%w: message sender unavailable", botapp.ErrInvalidInput)
	}
	result, err := s.Messages.Send(ctx, imapp.SendRequest{
		ActorID: in.ActorID, Topic: in.Topic, Type: imdomain.MessageTypeText,
		Content: in.Content, ClientMsgID: in.ClientMsgID, MentionedIDs: in.MentionedIDs,
	})
	if err != nil {
		return botapp.SendResult{}, err
	}
	return botapp.SendResult{ClientMsgID: result.ClientMsgID, Timestamp: result.Timestamp}, nil
}

// Users adapts the auth repository to username lookups.
type Users struct{ Repo biz.AuthRepo }

func (u Users) Username(ctx context.Context, userID int64) (string, error) {
	user, err := u.Repo.GetUserByID(ctx, userID)
	if err != nil {
		return "", err
	}
	return user.Username, nil
}

// GroupMembers adapts the group repository to the member list used by stream
// recipients (without importing the data layer).
type GroupMembers struct {
	Repo interface {
		ListMembers(ctx context.Context, groupID string) ([]imdomain.GroupMember, error)
	}
}

func (g GroupMembers) GetGroupMembers(ctx context.Context, groupID string) ([]int64, error) {
	if g.Repo == nil {
		return nil, fmt.Errorf("group repository unavailable")
	}
	members, err := g.Repo.ListMembers(ctx, groupID)
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(members))
	for _, member := range members {
		if member.UserID > 0 {
			out = append(out, member.UserID)
		}
	}
	return out, nil
}

// Recipients resolves topic members for ephemeral bot stream delivery.
type Recipients struct{ Groups imapp.GroupMemberReader }

func (r Recipients) Recipients(ctx context.Context, rawTopic string, exclude int64) ([]int64, error) {
	switch topic.ParseType(rawTopic) {
	case topic.TypeP2P:
		first, second, err := topic.ExtractUserIDsFromP2PTopic(rawTopic)
		if err != nil {
			return nil, err
		}
		out := make([]int64, 0, 2)
		for _, id := range []int64{first, second} {
			if id > 0 && id != exclude {
				out = append(out, id)
			}
		}
		return out, nil
	case topic.TypeGroup:
		if r.Groups == nil {
			return nil, fmt.Errorf("group member reader unavailable")
		}
		groupID, err := imdomain.ExtractGroupID(rawTopic)
		if err != nil {
			return nil, err
		}
		members, err := r.Groups.GetGroupMembers(ctx, groupID)
		if err != nil {
			return nil, err
		}
		out := make([]int64, 0, len(members))
		for _, id := range members {
			if id > 0 && id != exclude {
				out = append(out, id)
			}
		}
		return out, nil
	default:
		return nil, nil
	}
}

// StreamPublisher adapts the realtime gateway pusher to bot stream events.
type StreamPublisher struct {
	Transport interface {
		PushBotStreamEventToUser(ctx context.Context, userID int64, event realtime.BotStreamEvent) (int32, error)
	}
}

func (p StreamPublisher) PublishBotStream(ctx context.Context, event botapp.StreamEvent, recipients []int64) error {
	if p.Transport == nil {
		return nil
	}
	converted := realtime.BotStreamEvent{
		StreamID: event.StreamID, Seq: event.Seq, Type: realtime.BotStreamEventType(event.Type),
		Topic: event.Topic, BotID: event.BotID, BotUserID: event.BotUserID,
		Delta: event.Delta, ClientMsgID: event.ClientMsgID, ReplyMsgID: event.ReplyMsgID,
		TimestampMs: event.TimestampMs,
	}
	var firstErr error
	for _, userID := range recipients {
		if userID <= 0 {
			continue
		}
		if _, err := p.Transport.PushBotStreamEventToUser(ctx, userID, converted); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Sealer stores webhook secrets with the deployment AES key.
type Sealer struct{ Key []byte }

func (s Sealer) Seal(plain []byte) ([]byte, error) { return credential.Seal(s.Key, plain) }
func (s Sealer) Open(ciphertext []byte) ([]byte, error) {
	return credential.Open(s.Key, ciphertext)
}

// NewSealer reads the bot seal key, falling back to the shared agent key for
// single-tenant deployments.
func NewSealer() (Sealer, error) {
	key := strings.TrimSpace(os.Getenv("BOT_SECRET_KEY"))
	if key == "" {
		key = strings.TrimSpace(os.Getenv("AGENT_SECRET_KEY"))
	}
	if len(key) < 32 {
		return Sealer{}, fmt.Errorf("BOT_SECRET_KEY or AGENT_SECRET_KEY must contain at least 32 bytes")
	}
	return Sealer{Key: []byte(key)}, nil
}

// NewWebhookGuard builds an SSRF-safe HTTP client for webhook delivery.
func NewWebhookGuard() *agentmcp.Guard {
	return &agentmcp.Guard{
		AllowPrivate:  truthy(os.Getenv("BOT_WEBHOOK_ALLOW_PRIVATE")),
		AllowInsecure: truthy(os.Getenv("BOT_WEBHOOK_ALLOW_INSECURE_HTTP")),
		AllowedHosts:  splitHosts(os.Getenv("BOT_WEBHOOK_ALLOWED_HOSTS")),
	}
}

func truthy(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func splitHosts(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if value := strings.ToLower(strings.TrimSpace(part)); value != "" {
			out = append(out, value)
		}
	}
	return out
}

// Ensure the adapter interfaces stay in sync.
var (
	_ botapp.MessageReader     = (*Reader)(nil)
	_ botapp.MessageLoader     = (*Reader)(nil)
	_ botapp.MessageSender     = (*Sender)(nil)
	_ botapp.UserDirectory     = Users{}
	_ botapp.RecipientResolver = Recipients{}
	_ botapp.EventPublisher    = StreamPublisher{}
	_ botapp.Sealer            = Sealer{}
	_ botapp.URLValidator      = (*agentmcp.Guard)(nil)
)
