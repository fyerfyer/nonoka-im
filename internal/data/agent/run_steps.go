package agentdata

import (
	"context"
	"time"

	"nonoka-im/internal/agent/domain"

	"gorm.io/gorm/clause"
)

// AppendRunStep durably records one non-delta run event. The (run_id, seq)
// unique index makes redelivery idempotent.
func (s *Store) AppendRunStep(ctx context.Context, step *domain.AgentRunStep) error {
	if step == nil || step.RunID == 0 || step.Seq == 0 {
		return nil
	}
	if step.CreatedAt.IsZero() {
		step.CreatedAt = time.Now()
	}
	row := AgentRunStepRow{
		RunID: step.RunID, Seq: step.Seq, Type: step.Type, Tool: step.Tool,
		ToolCallID: step.ToolCallID, Summary: step.Summary, Status: step.Status,
		CreatedAt: step.CreatedAt,
	}
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "run_id"}, {Name: "seq"}},
		DoNothing: true,
	}).Create(&row).Error
	return transientOrDomain(err)
}

// ListRunSteps returns the durable timeline for a run in event order.
func (s *Store) ListRunSteps(ctx context.Context, runID uint64) ([]domain.AgentRunStep, error) {
	var rows []AgentRunStepRow
	if err := s.db.WithContext(ctx).Where("run_id = ?", runID).Order("seq asc").Find(&rows).Error; err != nil {
		return nil, transientOrDomain(err)
	}
	out := make([]domain.AgentRunStep, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}
