// Package adapters connects the agent application ports to the shared IM,
// platform and credential packages. Keeping the adapters here lets
// cmd/agent-worker stay a thin composition root and keeps internal/agent/app
// free of infrastructure imports.
package adapters

import (
	"context"
	"errors"
	"fmt"

	"nonoka-im/internal/agent/app"
	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/tools"
	"nonoka-im/internal/biz"
	imdata "nonoka-im/internal/data/im"
	"nonoka-im/internal/messaging"
	"nonoka-im/internal/messaging/httpclient"
)

// Messages adapts messaging.Reader to app.MessageReader.
type Messages struct{ Reader messaging.Reader }

func (m Messages) RecentMessages(ctx context.Context, userID int64, topic string, limit int) ([]app.Message, error) {
	rows, err := m.Reader.RecentMessages(ctx, userID, topic, limit)
	if err != nil {
		return nil, err
	}
	out := make([]app.Message, 0, len(rows))
	for _, row := range rows {
		out = append(out, app.Message{
			ID: row.ID, SenderID: row.SenderID, Timestamp: row.Timestamp,
			Type: row.Type, Content: row.Content, FileID: row.FileID,
			FileName: row.FileName, Mime: row.Mime, Size: row.Size,
		})
	}
	return out, nil
}

func (m Messages) SearchMessages(ctx context.Context, userID int64, topic, query string, filter app.SearchFilter, limit, offset int) ([]app.SearchHit, bool, error) {
	rows, hasMore, err := m.Reader.SearchMessages(ctx, userID, topic, query, messaging.SearchFilter{
		SenderID: filter.SenderID, Since: filter.Since, Until: filter.Until, HasFile: filter.HasFile,
	}, limit, offset)
	if err != nil {
		return nil, false, err
	}
	out := make([]app.SearchHit, 0, len(rows))
	for _, row := range rows {
		out = append(out, app.SearchHit{
			MsgID: row.MsgID, Topic: row.Topic, TopicSeq: row.TopicSeq, SenderID: row.SenderID,
			Timestamp: row.Timestamp, MsgType: row.MsgType, HasFile: row.HasFile, Snippet: row.Snippet,
		})
	}
	return out, hasMore, nil
}

func (m Messages) MessageForUser(ctx context.Context, userID int64, topic string, msgID int64) (app.Message, error) {
	row, err := m.Reader.MessageForUser(ctx, userID, topic, msgID)
	if err != nil {
		if errors.Is(err, messaging.ErrNotFound) {
			return app.Message{}, app.ErrMessageNotFound
		}
		return app.Message{}, err
	}
	return app.Message{
		ID: row.ID, SenderID: row.SenderID, Timestamp: row.Timestamp,
		Type: row.Type, Content: row.Content, FileID: row.FileID,
		FileName: row.FileName, Mime: row.Mime, Size: row.Size,
	}, nil
}

// CitationStore is the narrow persistence surface for run citations.
type CitationStore interface {
	RecordRunCitations(ctx context.Context, runID uint64, citations []domain.AgentRunCitation) error
}

// Citations adapts app.SearchHit to the agent domain citation row.
type Citations struct{ Store CitationStore }

func (c Citations) RecordRunCitations(ctx context.Context, runID uint64, hits []app.SearchHit) error {
	const maxCitations = 8
	if len(hits) > maxCitations {
		hits = hits[:maxCitations]
	}
	rows := make([]domain.AgentRunCitation, 0, len(hits))
	for i, hit := range hits {
		rows = append(rows, domain.AgentRunCitation{
			RunID: runID, Ordinal: i, Topic: hit.Topic, MsgID: hit.MsgID,
			TopicSeq: hit.TopicSeq, SenderID: hit.SenderID, Snippet: hit.Snippet,
		})
	}
	return c.Store.RecordRunCitations(ctx, runID, rows)
}

// Files adapts messaging.FileReader to app.FileReader.
type Files struct{ Reader messaging.FileReader }

func (f Files) ReadFile(ctx context.Context, fileID string) (app.File, error) {
	doc, err := f.Reader.ReadFile(ctx, fileID)
	if err != nil {
		return app.File{}, err
	}
	return app.File{Name: doc.Name, Mime: doc.Mime, Data: doc.Data}, nil
}

// Sender adapts the internal HTTP client to app.ReplySender. It supplies the
// bot credential for the current run context.
type Sender struct {
	Client      *httpclient.Client
	TokenIssuer func(agentID, botID int64, topic string) (string, error)
}

func (s *Sender) token(ctx context.Context, topic string) (string, error) {
	if s.TokenIssuer == nil {
		return "", fmt.Errorf("token issuer unavailable")
	}
	rc, ok := tools.GetRunContext(ctx)
	if !ok {
		return "", fmt.Errorf("run context missing for bot credential")
	}
	return s.TokenIssuer(rc.AgentID, rc.BotID, topic)
}

func (s *Sender) Send(ctx context.Context, topic, content, clientID string) error {
	_, err := s.send(ctx, topic, content, clientID, httpclient.MsgTypeText)
	return err
}

func (s *Sender) SendWithReceipt(ctx context.Context, topic, content, clientID string) (int64, error) {
	return s.send(ctx, topic, content, clientID, httpclient.MsgTypeText)
}

func (s *Sender) SendCard(ctx context.Context, topic, content, clientID string) (int64, error) {
	return s.send(ctx, topic, content, clientID, httpclient.MsgTypeAgentCard)
}

func (s *Sender) send(ctx context.Context, topic, content, clientID string, msgType int32) (int64, error) {
	client := *s.Client
	client.Token = func(ctx context.Context) (string, error) { return s.token(ctx, topic) }
	if msgType == httpclient.MsgTypeAgentCard {
		return client.SendCard(ctx, topic, content, clientID)
	}
	return client.SendWithReceipt(ctx, topic, content, clientID)
}

// Directory adapts the PostgreSQL repositories to app.Directory. Members is
// optional and is normally the shared cached group-member service; GroupName
// and usernames stay on the repositories.
type Directory struct {
	Groups  imdata.GroupRepo
	Members interface {
		GetGroupMembers(ctx context.Context, groupID string) ([]int64, error)
	}
	Users biz.AuthRepo
}

func (d *Directory) Username(ctx context.Context, id int64) (string, error) {
	user, err := d.Users.GetUserByID(ctx, id)
	if err != nil {
		return fmt.Sprintf("user-%d", id), nil
	}
	return user.Username, nil
}

func (d *Directory) GroupName(ctx context.Context, groupID string) (string, error) {
	group, err := d.Groups.GetGroup(ctx, groupID)
	if err != nil {
		return "", err
	}
	return group.Name, nil
}

func (d *Directory) GroupMemberIDs(ctx context.Context, groupID string) ([]int64, error) {
	if d.Members != nil {
		return d.Members.GetGroupMembers(ctx, groupID)
	}
	members, err := d.Groups.ListMembers(ctx, groupID)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.UserID)
	}
	return ids, nil
}

func (d *Directory) UserProfile(ctx context.Context, id int64) (app.UserProfile, error) {
	user, err := d.Users.GetUserByID(ctx, id)
	if err != nil {
		return app.UserProfile{}, err
	}
	return app.UserProfile{ID: user.ID, Username: user.Username, CreatedAt: user.CreatedAt.Unix()}, nil
}

func (d *Directory) MyGroups(ctx context.Context, userID int64) ([]app.GroupRef, error) {
	groups, err := d.Groups.ListMyGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	refs := make([]app.GroupRef, 0, len(groups))
	for _, g := range groups {
		members, err := d.Groups.ListMembers(ctx, g.GroupID)
		if err != nil {
			return nil, err
		}
		refs = append(refs, app.GroupRef{ID: g.GroupID, Name: g.Name, MemberCount: len(members)})
	}
	return refs, nil
}
