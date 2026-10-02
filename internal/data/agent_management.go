package data

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrDraftConflict = errors.New("agent draft changed")
var ErrRunState = errors.New("run state conflict")
var ErrAgentNotFound = errors.New("agent not found")

func (s *AgentStore) CreateAgent(ctx context.Context, ownerID int64, name, description, instruction, model, tools string, grant func(context.Context, int64) error) (*Agent, error) {
	var row Agent
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		bot := User{Username: "agent_" + uuid.NewString()[:12], Password: "!", IsBot: true}
		if err := tx.Create(&bot).Error; err != nil {
			return err
		}
		row = Agent{BotUserID: bot.ID, OwnerUserID: ownerID, Name: name, Description: description, Status: "draft", DraftInstruction: instruction, DraftModel: model, DraftTools: tools, DraftRevision: 1}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if grant != nil {
			return grant(ctx, row.ID)
		}
		return nil
	})
	return &row, err
}

func (s *AgentStore) GetAgent(ctx context.Context, id int64) (*Agent, error) {
	var row Agent
	err := s.db.WithContext(ctx).First(&row, id).Error
	return &row, err
}

func (s *AgentStore) GetAgentByBot(ctx context.Context, botID int64) (*Agent, error) {
	var row Agent
	err := s.db.WithContext(ctx).Where("bot_user_id = ?", botID).First(&row).Error
	return &row, err
}

// ListAgents returns one page of agents ordered by ID, plus the total count
// of agents for pagination.
func (s *AgentStore) ListAgents(ctx context.Context, limit, offset int) ([]Agent, int64, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	var total int64
	if err := s.db.WithContext(ctx).Model(&Agent{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Agent
	err := s.db.WithContext(ctx).Order("id asc").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

func (s *AgentStore) ActiveAgentsByBot(ctx context.Context, ids []int64) ([]Agent, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []Agent
	err := s.db.WithContext(ctx).Where("bot_user_id IN ? AND status = ? AND current_version > 0", ids, "active").Find(&rows).Error
	return rows, err
}

func (s *AgentStore) GetVersion(ctx context.Context, agentID, version int64) (*AgentVersion, error) {
	var row AgentVersion
	err := s.db.WithContext(ctx).Where("agent_id = ? AND version = ?", agentID, version).First(&row).Error
	return &row, err
}

func (s *AgentStore) UpdateDraft(ctx context.Context, id, revision int64, name, description, instruction, model, tools string) (*Agent, error) {
	result := s.db.WithContext(ctx).Model(&Agent{}).Where("id = ? AND draft_revision = ?", id, revision).Updates(map[string]interface{}{"name": name, "description": description, "draft_instruction": instruction, "draft_model": model, "draft_tools": tools, "draft_revision": gorm.Expr("draft_revision + 1"), "updated_at": time.Now()})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, ErrDraftConflict
	}
	return s.GetAgent(ctx, id)
}

func (s *AgentStore) Publish(ctx context.Context, id, revision int64) (*AgentVersion, error) {
	var version AgentVersion
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row Agent
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		if row.DraftRevision != revision {
			return ErrDraftConflict
		}
		version = AgentVersion{AgentID: id, Version: row.CurrentVersion + 1, Instruction: row.DraftInstruction, Model: row.DraftModel, Tools: row.DraftTools}
		if err := tx.Create(&version).Error; err != nil {
			return err
		}
		return tx.Model(&row).Updates(map[string]interface{}{"current_version": version.Version, "status": "active", "updated_at": time.Now()}).Error
	})
	return &version, err
}

func (s *AgentStore) SetAgentStatus(ctx context.Context, id int64, status string) error {
	if status != "active" && status != "disabled" {
		return fmt.Errorf("invalid agent status")
	}
	return s.db.WithContext(ctx).Model(&Agent{}).Where("id = ? AND current_version > 0", id).Update("status", status).Error
}

func (s *AgentStore) ListRuns(ctx context.Context, agentID int64, limit int) ([]AgentRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var rows []AgentRun
	err := s.db.WithContext(ctx).Where("agent_id = ?", agentID).Order("id desc").Limit(limit).Find(&rows).Error
	return rows, err
}

func (s *AgentStore) GetRun(ctx context.Context, id uint64) (*AgentRun, error) {
	var row AgentRun
	err := s.db.WithContext(ctx).First(&row, id).Error
	return &row, err
}

func (s *AgentStore) SetRunAction(ctx context.Context, id uint64, action string) error {
	var from, to string
	switch action {
	case "retry":
		from, to = "failed", "queued"
	case "skip":
		from, to = "failed", "cancelled"
	default:
		return fmt.Errorf("invalid run action")
	}
	result := s.db.WithContext(ctx).Model(&AgentRun{}).Where("id = ? AND status = ?", id, from).Update("status", to)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrRunState
	}
	return nil
}

func (s *AgentStore) QueuedRuns(ctx context.Context, limit int) ([]AgentRun, error) {
	if limit <= 0 {
		limit = 32
	}
	var rows []AgentRun
	err := s.db.WithContext(ctx).Table("agent_runs AS r").Select("r.*").Where("r.status = ? AND NOT EXISTS (SELECT 1 FROM agent_runs p WHERE p.session_id = r.session_id AND p.id <> r.id AND p.topic_seq < r.topic_seq AND p.status IN ('queued','running'))", "queued").Order("r.topic_seq asc, r.id asc").Limit(limit).Scan(&rows).Error
	return rows, err
}

func (s *AgentStore) BootstrapAgent(ctx context.Context, id, botID, ownerID int64, name, instruction, model string, grant func(context.Context, int64) error) error {
	if id <= 0 || ownerID <= 0 {
		return fmt.Errorf("bootstrap agent and owner IDs required")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing Agent
		if err := tx.First(&existing, id).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if botID <= 0 {
			bot := User{Username: "agent_" + uuid.NewString()[:12], Password: "!", IsBot: true}
			if err := tx.Create(&bot).Error; err != nil {
				return err
			}
			botID = bot.ID
		} else {
			result := tx.Model(&User{}).Where("id = ?", botID).Updates(map[string]any{"is_bot": true, "password": "!"})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("bootstrap bot user %d does not exist", botID)
			}
		}
		if model == "" {
			model = "deepseek-chat"
		}
		row := Agent{ID: id, BotUserID: botID, OwnerUserID: ownerID, Name: name, Status: "active", CurrentVersion: 1, DraftInstruction: instruction, DraftModel: model, DraftTools: `["search_messages","get_conversation_context","send_message"]`, DraftRevision: 1}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		version := AgentVersion{AgentID: id, Version: 1, Instruction: instruction, Model: model, Tools: row.DraftTools}
		if err := tx.Create(&version).Error; err != nil {
			return err
		}
		if grant != nil {
			return grant(ctx, id)
		}
		return nil
	})
}
