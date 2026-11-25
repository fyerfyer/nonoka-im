package agentdata

import (
	"context"
	"time"
	"unicode/utf8"

	"nonoka-im/internal/agent/domain"

	"gorm.io/gorm/clause"
)

// RecordRunCitations appends the sources consulted by a run. The unique
// (run_id, topic, msg_id) index makes repeated tool calls and retries
// idempotent.
func (s *Store) RecordRunCitations(ctx context.Context, runID uint64, citations []domain.AgentRunCitation) error {
	if runID == 0 || len(citations) == 0 {
		return nil
	}
	rows := make([]AgentRunCitationRow, 0, len(citations))
	now := time.Now()
	for i, c := range citations {
		if c.MsgID == 0 || c.Topic == "" {
			continue
		}
		ordinal := c.Ordinal
		if ordinal == 0 {
			ordinal = i
		}
		rows = append(rows, AgentRunCitationRow{
			RunID: runID, Ordinal: ordinal, Topic: c.Topic, MsgID: c.MsgID,
			TopicSeq: c.TopicSeq, SenderID: c.SenderID, Snippet: trimSnippet(c.Snippet),
			CreatedAt: now,
		})
	}
	if len(rows) == 0 {
		return nil
	}
	return transientOrDomain(s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "run_id"}, {Name: "topic"}, {Name: "msg_id"}},
		DoNothing: true,
	}).Create(&rows).Error)
}

// ListRunCitations returns the run's sources in first-seen order.
func (s *Store) ListRunCitations(ctx context.Context, runID uint64) ([]domain.AgentRunCitation, error) {
	var rows []AgentRunCitationRow
	if err := s.db.WithContext(ctx).Where("run_id = ?", runID).Order("id asc").Find(&rows).Error; err != nil {
		return nil, transientOrDomain(err)
	}
	out := make([]domain.AgentRunCitation, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

// GetRunByReplyMsgID resolves the agent answer message back to its run.
func (s *Store) GetRunByReplyMsgID(ctx context.Context, msgID int64) (*domain.AgentRun, error) {
	var row AgentRunRow
	if err := s.db.WithContext(ctx).Where("reply_msg_id = ?", msgID).Order("id desc").First(&row).Error; err != nil {
		return nil, notFoundOrTransient(err, domain.ErrRunNotFound)
	}
	run := row.toDomain()
	return &run, nil
}

func trimSnippet(s string) string {
	const max = 240
	if len(s) <= max {
		return s
	}
	s = s[:max]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s + "…"
}
