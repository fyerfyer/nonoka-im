package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"nonoka-im/internal/agent/tools"
	"nonoka-im/internal/authz"
	"nonoka-im/internal/filestore"
	"nonoka-im/internal/msgworker"
	"nonoka-im/internal/topic"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Directory resolves display metadata (usernames, group names and members)
// for the get_topic_info tool. It is implemented over the existing data
// repositories in cmd/agent-worker.
type Directory interface {
	Username(ctx context.Context, id int64) (string, error)
	GroupName(ctx context.Context, groupID string) (string, error)
	GroupMemberIDs(ctx context.Context, groupID string) ([]int64, error)
}

// MessageTools is a narrow read/write adapter over the existing IM storage and
// send API. Every call checks the triggering user's authorization; bot access
// is checked where the bot acts on the topic.
type MessageTools struct {
	Storage    *msgworker.MessageStorage
	Authorizer authz.Authorizer
	Sender     ReplySender
	Files      *mongo.Database
	Directory  Directory
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

// ReadFile loads an attachment after checking the actor's read permission on
// the file resource. Content is truncated to maxBytes.
func (b *MessageTools) ReadFile(ctx context.Context, rc tools.RunContext, fileID string, maxBytes int) (string, string, []byte, bool, error) {
	if b.Files == nil || b.Authorizer == nil {
		return "", "", nil, false, fmt.Errorf("file tools unavailable")
	}
	if strings.ContainsAny(fileID, "/\\ \t\n") {
		return "", "", nil, false, fmt.Errorf("invalid file id")
	}
	if err := b.Authorizer.CanReadFile(ctx, rc.ActorUserID, fileID); err != nil {
		return "", "", nil, false, fmt.Errorf("file access denied")
	}
	doc, err := filestore.Read(ctx, b.Files, fileID)
	if err != nil {
		return "", "", nil, false, fmt.Errorf("file unavailable")
	}
	truncated := false
	data := doc.Data
	if len(data) > maxBytes {
		data = data[:maxBytes]
		// Re-slice to a valid UTF-8 boundary so text tools never see split runes.
		for len(data) > 0 && !utf8.Valid(data) {
			data = data[:len(data)-1]
		}
		truncated = true
	}
	return doc.Name, doc.Mime, data, truncated, nil
}

// TopicInfo describes the current conversation to the model.
func (b *MessageTools) TopicInfo(ctx context.Context, rc tools.RunContext) (string, error) {
	if b.Authorizer == nil || b.Directory == nil {
		return "", fmt.Errorf("topic info unavailable")
	}
	if err := b.Authorizer.CanAccessTopic(ctx, rc.ActorUserID, rc.Topic); err != nil {
		return "", fmt.Errorf("topic access denied")
	}
	switch topic.ParseType(rc.Topic) {
	case topic.TypeGroup:
		groupID, err := topic.ExtractGroupID(rc.Topic)
		if err != nil {
			return "", err
		}
		name, err := b.Directory.GroupName(ctx, groupID)
		if err != nil {
			return "", fmt.Errorf("group unavailable")
		}
		ids, err := b.Directory.GroupMemberIDs(ctx, groupID)
		if err != nil {
			return "", fmt.Errorf("group members unavailable")
		}
		const memberCap = 50
		truncated := false
		if len(ids) > memberCap {
			ids = ids[:memberCap]
			truncated = true
		}
		names := make([]string, 0, len(ids))
		for _, id := range ids {
			username, _ := b.Directory.Username(ctx, id)
			names = append(names, fmt.Sprintf("%s(id=%d)", username, id))
		}
		suffix := ""
		if truncated {
			suffix = ", ..."
		}
		return fmt.Sprintf("type=group name=%q members(%d): %s%s", name, len(ids), strings.Join(names, ", "), suffix), nil
	case topic.TypeP2P:
		x, y, err := topic.ExtractUserIDsFromP2PTopic(rc.Topic)
		if err != nil {
			return "", err
		}
		peer := x
		if peer == rc.ActorUserID {
			peer = y
		}
		name, _ := b.Directory.Username(ctx, peer)
		return fmt.Sprintf("type=direct peer=%q(id=%d)", name, peer), nil
	default:
		return "type=system", nil
	}
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
