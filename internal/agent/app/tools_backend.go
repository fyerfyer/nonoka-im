package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"nonoka-im/internal/agent/tools"
	"nonoka-im/internal/authz"
	"nonoka-im/internal/msgworker"
)

// MessageTools is a narrow read/write adapter over the existing IM storage and
// send API. Every call checks both the triggering user and the Bot's access.
type MessageTools struct {
	Storage    *msgworker.MessageStorage
	Authorizer authz.Authorizer
	Sender     ReplySender
}

func (b *MessageTools) messages(ctx context.Context, rc tools.RunContext, limit int) ([]*msgworker.InboxMessage, error) {
	if b.Storage == nil || b.Authorizer == nil {
		return nil, fmt.Errorf("message tools unavailable")
	}
	if err := b.Authorizer.CanAccessTopic(ctx, rc.ActorUserID, rc.Topic); err != nil {
		return nil, fmt.Errorf("topic access denied")
	}
	if err := b.Authorizer.CanAccessTopic(ctx, rc.BotID, rc.Topic); err != nil {
		return nil, fmt.Errorf("bot topic access denied")
	}
	return b.Storage.GetRecentMessages(ctx, rc.ActorUserID, rc.Topic, limit)
}

func (b *MessageTools) SearchMessages(ctx context.Context, rc tools.RunContext, query string, limit int) ([]string, error) {
	if b.Storage == nil || b.Authorizer == nil {
		return nil, fmt.Errorf("message tools unavailable")
	}
	if err := b.Authorizer.CanAccessTopic(ctx, rc.ActorUserID, rc.Topic); err != nil {
		return nil, fmt.Errorf("topic access denied")
	}
	if err := b.Authorizer.CanAccessTopic(ctx, rc.BotID, rc.Topic); err != nil {
		return nil, fmt.Errorf("bot topic access denied")
	}
	rows, err := b.messages(ctx, rc, 100)
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	result := make([]string, 0, limit)
	for i := len(rows) - 1; i >= 0 && len(result) < limit; i-- {
		row := rows[i]
		content := strings.ToValidUTF8(string(row.Content), "�")
		if strings.Contains(strings.ToLower(content), query) {
			result = append(result, formatToolMessage(row.SenderID, row.Timestamp, content))
		}
	}
	return boundToolResults(result, 8000), nil
}

func (b *MessageTools) ConversationContext(ctx context.Context, rc tools.RunContext, limit int) ([]string, error) {
	rows, err := b.messages(ctx, rc, limit)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(rows))
	for _, row := range rows {
		result = append(result, formatToolMessage(row.SenderID, row.Timestamp, strings.ToValidUTF8(string(row.Content), "�")))
	}
	return boundToolResults(result, 8000), nil
}

func (b *MessageTools) SendMessage(ctx context.Context, rc tools.RunContext, content string) error {
	if b.Sender == nil || b.Authorizer == nil {
		return fmt.Errorf("message sending unavailable")
	}
	if err := b.Authorizer.CanAccessTopic(ctx, rc.ActorUserID, rc.Topic); err != nil {
		return fmt.Errorf("topic access denied")
	}
	if err := b.Authorizer.CanAccessTopic(ctx, rc.BotID, rc.Topic); err != nil {
		return fmt.Errorf("bot topic access denied")
	}
	if len(content) > 64*1024 {
		return fmt.Errorf("message too large")
	}
	h := sha256.Sum256([]byte(rc.EventID + "\x00tool\x00" + content))
	return b.Sender.Send(ctx, rc.Topic, content, "agent-tool-"+hex.EncodeToString(h[:16]))
}

func formatToolMessage(senderID, timestamp int64, content string) string {
	if len(content) > 1200 {
		content = content[:1200]
	}
	return fmt.Sprintf("sender=%d time=%d: %s", senderID, timestamp, content)
}

func boundToolResults(values []string, maxChars int) []string {
	result := make([]string, 0, len(values))
	used := 0
	for _, value := range values {
		remaining := maxChars - used
		if remaining <= 0 {
			break
		}
		if len(value) > remaining {
			value = value[:remaining]
		}
		result = append(result, value)
		used += len(value)
	}
	return result
}
