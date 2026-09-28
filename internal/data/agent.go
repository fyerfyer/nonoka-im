package data

import (
	"context"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type AgentRun struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	EventID     string `gorm:"uniqueIndex;size:128;not null"`
	AgentID     int64  `gorm:"index;not null"`
	ActorUserID int64  `gorm:"index;not null"`
	Topic       string `gorm:"index;size:128;not null"`
	MsgID       int64  `gorm:"index;not null"`
	Status      string `gorm:"index;size:24;not null"`
	ErrorCode   string `gorm:"size:64"`
	Attempts    int    `gorm:"not null;default:0"`
	ReplyMsgID  int64
	CreatedAt   time.Time
	StartedAt   *time.Time
	FinishedAt  *time.Time
}

type AgentTurn struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	AgentID     int64  `gorm:"index;not null"`
	ActorUserID int64  `gorm:"index;not null"`
	Topic       string `gorm:"index;size:128;not null"`
	RunID       uint64 `gorm:"uniqueIndex;not null"`
	Input       string `gorm:"type:text;not null"`
	Output      string `gorm:"type:text"`
	CreatedAt   time.Time
}

type AgentSession struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	AgentID     int64  `gorm:"index:idx_agent_session,unique;priority:1;not null"`
	ActorUserID int64  `gorm:"index:idx_agent_session,unique;priority:2;not null"`
	Topic       string `gorm:"index:idx_agent_session,unique;priority:3;size:128;not null"`
	UpdatedAt   time.Time
	CreatedAt   time.Time
}

type AgentStore struct{ db *gorm.DB }

func NewAgentStore(d *Data) *AgentStore { return &AgentStore{db: d.db} }
func (s *AgentStore) ClaimRun(ctx context.Context, eventID string, run *AgentRun) (*AgentRun, bool, error) {
	var existing AgentRun
	if err := s.db.WithContext(ctx).Where("event_id = ?", eventID).First(&existing).Error; err == nil {
		staleRunning := existing.Status == "running" && ((existing.StartedAt != nil && existing.StartedAt.Before(time.Now().Add(-3*time.Minute))) || (existing.StartedAt == nil && existing.CreatedAt.Before(time.Now().Add(-3*time.Minute))))
		if existing.Status == "failed" || staleRunning {
			result := s.db.WithContext(ctx).Model(&AgentRun{}).Where("event_id = ? AND status = ?", eventID, "failed").Updates(map[string]interface{}{"status": "running", "attempts": gorm.Expr("attempts + 1"), "error_code": ""})
			if existing.Status == "running" {
				cutoff := time.Now().Add(-3 * time.Minute)
				result = s.db.WithContext(ctx).Model(&AgentRun{}).Where("event_id = ? AND status = ? AND ((started_at IS NOT NULL AND started_at < ?) OR (started_at IS NULL AND created_at < ?))", eventID, "running", cutoff, cutoff).Updates(map[string]interface{}{"attempts": gorm.Expr("attempts + 1"), "error_code": ""})
			}
			if result.Error != nil {
				return nil, false, result.Error
			}
			if result.RowsAffected == 1 {
				if err := s.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("event_id = ?", eventID).First(&existing).Error; err != nil {
					return nil, false, err
				}
				return &existing, true, nil
			}
			if err := s.db.WithContext(ctx).Where("event_id = ?", eventID).First(&existing).Error; err != nil {
				return nil, false, err
			}
		}
		return &existing, false, nil
	}
	if err := s.db.WithContext(ctx).Create(run).Error; err != nil {
		if err := s.db.WithContext(ctx).Where("event_id = ?", eventID).First(&existing).Error; err == nil {
			return &existing, false, nil
		}
		return nil, false, err
	}
	return run, true, nil
}
func (s *AgentStore) UpdateRun(ctx context.Context, id uint64, values map[string]interface{}) error {
	return s.db.WithContext(ctx).Model(&AgentRun{}).Where("id = ?", id).Updates(values).Error
}
func (s *AgentStore) TouchSession(ctx context.Context, agentID, actorID int64, topic string) error {
	var row AgentSession
	err := s.db.WithContext(ctx).Where("agent_id=? AND actor_user_id=? AND topic=?", agentID, actorID, topic).First(&row).Error
	if err == nil {
		return s.db.WithContext(ctx).Model(&row).Update("updated_at", time.Now()).Error
	}
	row = AgentSession{AgentID: agentID, ActorUserID: actorID, Topic: topic, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		// Concurrent event processing may have created the row; retry as update.
		return s.db.WithContext(ctx).Model(&AgentSession{}).Where("agent_id=? AND actor_user_id=? AND topic=?", agentID, actorID, topic).Update("updated_at", time.Now()).Error
	}
	return nil
}
func (s *AgentStore) AddTurn(ctx context.Context, turn *AgentTurn) error {
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "run_id"}}, DoNothing: true}).Create(turn).Error
}
func (s *AgentStore) RecentTurns(ctx context.Context, agentID, actorID int64, topic string, limit int) ([]AgentTurn, error) {
	if limit <= 0 {
		limit = 10
	}
	var rows []AgentTurn
	err := s.db.WithContext(ctx).Where("agent_id=? AND actor_user_id=? AND topic=?", agentID, actorID, topic).Order("created_at desc").Limit(limit).Find(&rows).Error
	return rows, err
}
