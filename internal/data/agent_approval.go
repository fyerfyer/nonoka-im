package data

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// AgentCheckpoint persists an Eino execution checkpoint (opaque gob bytes)
// keyed by the run-scoped checkpoint ID so an interrupted run can resume
// after a worker restart.
type AgentCheckpoint struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	RunID        uint64 `gorm:"index;not null"`
	CheckpointID string `gorm:"uniqueIndex;size:64;not null"`
	State        []byte `gorm:"type:bytea;not null"`
	CreatedAt    time.Time
	ExpiresAt    time.Time `gorm:"index;not null"`
}

// AgentApproval is the business record of a tool call that interrupted for
// human confirmation. The raw tool arguments never enter this table: only a
// human-readable summary plus a SHA256 for matching against the checkpoint.
type AgentApproval struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	RunID       uint64 `gorm:"index:idx_agent_approval_run,priority:1;not null"`
	ToolCallID  string `gorm:"index:idx_agent_approval_run,priority:2;size:128;not null"`
	Tool        string `gorm:"size:128;not null"`
	ArgsSummary string `gorm:"type:text;not null"`
	ArgsHash    string `gorm:"size:64;not null"`
	ApproverID  int64  `gorm:"index;not null"`
	Status      string `gorm:"index;size:24;not null"` // pending/approved/rejected/expired/consumed
	ExpiresAt   time.Time
	ConsumedAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CreateApproval inserts a pending approval unless one already exists for the
// (run, tool call) pair, making interrupt handling idempotent under Kafka
// redelivery.
func (s *AgentStore) CreateApproval(ctx context.Context, row *AgentApproval) error {
	return s.db.WithContext(ctx).
		Where(AgentApproval{RunID: row.RunID, ToolCallID: row.ToolCallID}).
		Attrs(*row).
		FirstOrCreate(row).Error
}

func (s *AgentStore) GetApproval(ctx context.Context, id uint64) (*AgentApproval, error) {
	var row AgentApproval
	err := s.db.WithContext(ctx).First(&row, id).Error
	return &row, err
}

// ApprovalForRun returns the most recent approval of the run, if any.
func (s *AgentStore) ApprovalForRun(ctx context.Context, runID uint64) (*AgentApproval, error) {
	var row AgentApproval
	err := s.db.WithContext(ctx).Where("run_id = ?", runID).Order("id desc").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

// TransitionApproval atomically moves an approval from one status to another.
// It fails with ErrRunState when the row is no longer in the expected status,
// which makes approve/reject one-shot under double clicks and redelivery.
func (s *AgentStore) TransitionApproval(ctx context.Context, id uint64, from, to string) error {
	result := s.db.WithContext(ctx).Model(&AgentApproval{}).
		Where("id = ? AND status = ?", id, from).
		Updates(map[string]interface{}{"status": to, "updated_at": time.Now()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrRunState
	}
	return nil
}

// ExpirePendingApprovals marks pending approvals past their expiry. Returns
// the affected run IDs so the caller can requeue the waiting runs.
func (s *AgentStore) ExpirePendingApprovals(ctx context.Context, now time.Time) ([]uint64, error) {
	var rows []AgentApproval
	if err := s.db.WithContext(ctx).Where("status = ? AND expires_at < ?", "pending", now).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	runIDs := make([]uint64, 0, len(rows))
	for i := range rows {
		if err := s.TransitionApproval(ctx, rows[i].ID, "pending", "expired"); err != nil && !errors.Is(err, ErrRunState) {
			return nil, err
		}
		runIDs = append(runIDs, rows[i].RunID)
	}
	return runIDs, nil
}

// ClaimWaitingRun atomically moves a waiting_approval run back to queued for
// resume execution. Returns ErrRunState when the run is not waiting.
func (s *AgentStore) ClaimWaitingRun(ctx context.Context, id uint64) error {
	result := s.db.WithContext(ctx).Model(&AgentRun{}).
		Where("id = ? AND status = ?", id, "waiting_approval").
		Updates(map[string]interface{}{"status": "queued", "started_at": nil})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrRunState
	}
	return nil
}

// SaveCheckpoint upserts the checkpoint bytes for a checkpoint ID.
func (s *AgentStore) SaveCheckpoint(ctx context.Context, runID uint64, checkpointID string, state []byte, ttl time.Duration) error {
	row := AgentCheckpoint{RunID: runID, CheckpointID: checkpointID, State: state, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(ttl)}
	return s.db.WithContext(ctx).
		Where(AgentCheckpoint{CheckpointID: checkpointID}).
		Assign(AgentCheckpoint{RunID: runID, State: state, ExpiresAt: row.ExpiresAt}).
		FirstOrCreate(&row).Error
}

func (s *AgentStore) GetCheckpoint(ctx context.Context, checkpointID string) ([]byte, bool, error) {
	var row AgentCheckpoint
	err := s.db.WithContext(ctx).Where("checkpoint_id = ?", checkpointID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if time.Now().After(row.ExpiresAt) {
		return nil, false, nil
	}
	return row.State, true, nil
}

// PruneExpiredCheckpoints deletes checkpoints past their TTL. Called from the
// worker sweeper.
func (s *AgentStore) PruneExpiredCheckpoints(ctx context.Context, now time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = 500
	}
	result := s.db.WithContext(ctx).Where("expires_at < ?", now).Limit(limit).Delete(&AgentCheckpoint{})
	return result.RowsAffected, result.Error
}
