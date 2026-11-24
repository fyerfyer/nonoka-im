package agentdata

import (
	"context"
	"fmt"
	"time"

	"nonoka-im/internal/agent/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BeginToolInvocation inserts a started ledger row for one side-effecting tool
// call, or returns the existing row. The unique (run_id, tool_call_id) index
// makes the operation safe under retries and concurrent deliveries.
func (s *Store) BeginToolInvocation(ctx context.Context, invocation *domain.ToolInvocation) (*domain.ToolInvocation, bool, error) {
	if invocation.RunID == 0 || invocation.ToolCallID == "" {
		return nil, false, fmt.Errorf("tool invocation requires run and tool call IDs")
	}
	if invocation.CreatedAt.IsZero() {
		invocation.CreatedAt = time.Now()
	}
	if invocation.UpdatedAt.IsZero() {
		invocation.UpdatedAt = invocation.CreatedAt
	}
	if invocation.Status == "" {
		invocation.Status = domain.ToolInvocationStarted
	}
	created := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := toolInvocationRowFromDomain(*invocation)
		res := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "run_id"}, {Name: "tool_call_id"}},
			DoNothing: true,
		}).Create(&row)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 1 {
			created = true
			*invocation = row.toDomain()
			return nil
		}
		var existing ToolInvocationRow
		if err := tx.Where("run_id = ? AND tool_call_id = ?", invocation.RunID, invocation.ToolCallID).First(&existing).Error; err != nil {
			return err
		}
		*invocation = existing.toDomain()
		return nil
	})
	if err != nil {
		return nil, false, fmt.Errorf("%w: %w", domain.ErrTransient, err)
	}
	return invocation, created, nil
}

// CompleteToolInvocation records a successful side effect. Replaying the tool
// call returns ResultSummary instead of executing it again.
func (s *Store) CompleteToolInvocation(ctx context.Context, runID uint64, toolCallID, resultSummary string) error {
	res := s.db.WithContext(ctx).Model(&ToolInvocationRow{}).
		Where("run_id = ? AND tool_call_id = ? AND status = ?", runID, toolCallID, domain.ToolInvocationStarted).
		Updates(map[string]interface{}{
			"status": domain.ToolInvocationCompleted, "result_summary": resultSummary, "updated_at": time.Now(),
		})
	if res.Error != nil {
		return fmt.Errorf("%w: %w", domain.ErrTransient, res.Error)
	}
	if res.RowsAffected != 1 {
		return fmt.Errorf("tool invocation %d/%s is no longer started", runID, toolCallID)
	}
	return nil
}

// FailToolInvocation records a failed side effect. The failure is cached so a
// retried tool call cannot accidentally execute the side effect a second time.
func (s *Store) FailToolInvocation(ctx context.Context, runID uint64, toolCallID, errorCode string) error {
	res := s.db.WithContext(ctx).Model(&ToolInvocationRow{}).
		Where("run_id = ? AND tool_call_id = ? AND status = ?", runID, toolCallID, domain.ToolInvocationStarted).
		Updates(map[string]interface{}{
			"status": domain.ToolInvocationFailed, "error_code": errorCode, "updated_at": time.Now(),
		})
	if res.Error != nil {
		return fmt.Errorf("%w: %w", domain.ErrTransient, res.Error)
	}
	if res.RowsAffected != 1 {
		return fmt.Errorf("tool invocation %d/%s is no longer started", runID, toolCallID)
	}
	return nil
}

// ListToolInvocations returns the durable side-effect ledger for a run.
func (s *Store) ListToolInvocations(ctx context.Context, runID uint64) ([]domain.ToolInvocation, error) {
	var rows []ToolInvocationRow
	if err := s.db.WithContext(ctx).Where("run_id = ?", runID).Order("id asc").Find(&rows).Error; err != nil {
		return nil, transientOrDomain(err)
	}
	out := make([]domain.ToolInvocation, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}
