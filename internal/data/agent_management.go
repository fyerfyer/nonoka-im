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

// AgentSpec is the editable draft configuration of an agent. APIKeyEnc is the
// sealed model credential; it is frozen into every published version so a run
// always uses the key that was current when its version was published.
type AgentSpec struct {
	Name, Description, Instruction string
	Model, Tools                   string
	BaseURL, APIKeyEnc             string
	MaxTokens                      int
	MCPBindings, Skills            string
}

func (s *AgentStore) CreateAgent(ctx context.Context, ownerID int64, spec AgentSpec, grant func(context.Context, int64) error) (*Agent, error) {
	var row Agent
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		bot := User{Username: "agent_" + uuid.NewString()[:12], Password: "!", IsBot: true}
		if err := tx.Create(&bot).Error; err != nil {
			return err
		}
		row = Agent{BotUserID: bot.ID, OwnerUserID: ownerID, Name: spec.Name, Description: spec.Description, Status: "draft", DraftInstruction: spec.Instruction, DraftModel: spec.Model, DraftTools: spec.Tools, DraftBaseURL: spec.BaseURL, DraftAPIKeyEnc: spec.APIKeyEnc, DraftMaxTokens: spec.MaxTokens, DraftMCPBindings: spec.MCPBindings, DraftSkills: spec.Skills, DraftRevision: 1}
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

// UpdateDraft replaces the draft configuration. updateKey=true overwrites the
// sealed API key; false keeps the existing one (requests send an empty key to
// mean "unchanged").
func (s *AgentStore) UpdateDraft(ctx context.Context, id, revision int64, spec AgentSpec, updateKey bool) (*Agent, error) {
	updates := map[string]interface{}{"name": spec.Name, "description": spec.Description, "draft_instruction": spec.Instruction, "draft_model": spec.Model, "draft_tools": spec.Tools, "draft_base_url": spec.BaseURL, "draft_max_tokens": spec.MaxTokens, "draft_mcp_bindings": spec.MCPBindings, "draft_skills": spec.Skills, "draft_revision": gorm.Expr("draft_revision + 1"), "updated_at": time.Now()}
	if updateKey {
		updates["draft_api_key_enc"] = spec.APIKeyEnc
	}
	result := s.db.WithContext(ctx).Model(&Agent{}).Where("id = ? AND draft_revision = ?", id, revision).Updates(updates)
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
		version = AgentVersion{AgentID: id, Version: row.CurrentVersion + 1, Instruction: row.DraftInstruction, Model: row.DraftModel, Tools: row.DraftTools, BaseURL: row.DraftBaseURL, APIKeyEnc: row.DraftAPIKeyEnc, MaxTokens: row.DraftMaxTokens, MCPBindings: row.DraftMCPBindings, Skills: row.DraftSkills}
		if err := tx.Create(&version).Error; err != nil {
			return err
		}
		return tx.Model(&row).Updates(map[string]interface{}{"current_version": version.Version, "status": "active", "updated_at": time.Now()}).Error
	})
	return &version, err
}

// DeleteAgent removes an agent owned by the caller. The bot user is left in
// place (historical messages still reference it) but can no longer act: bot
// sends verify against the agent row. Unfinished runs are cancelled.
func (s *AgentStore) DeleteAgent(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("agent_id = ?", id).Delete(&AgentVersion{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&AgentRun{}).Where("agent_id = ? AND status IN ?", id, []string{"queued", "running", "waiting_approval"}).
			Updates(map[string]interface{}{"status": "cancelled", "error_code": "agent_deleted", "finished_at": time.Now()}).Error; err != nil {
			return err
		}
		return tx.Delete(&Agent{}, id).Error
	})
}

// RollbackPublish removes a just-published version and restores the agent row
// to its previous state. It refuses to roll back if a newer publish won the
// race, so a failed tool binding never destroys a newer valid version.
func (s *AgentStore) RollbackPublish(ctx context.Context, id, version int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row Agent
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		if row.CurrentVersion != version {
			return fmt.Errorf("agent %d advanced past version %d; refusing rollback", id, version)
		}
		if err := tx.Where("agent_id = ? AND version = ?", id, version).Delete(&AgentVersion{}).Error; err != nil {
			return err
		}
		updates := map[string]interface{}{"current_version": version - 1, "updated_at": time.Now()}
		if version-1 == 0 {
			updates["status"] = "draft"
		}
		return tx.Model(&Agent{}).Where("id = ?", id).Updates(updates).Error
	})
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

// BootstrapAgent idempotently creates the preset agent. It returns
// created=false (without error) when the agent already exists, so callers can
// tell first-time initialization apart from a restart. Concurrent bootstraps
// race safely: the insert is a no-op on conflict.
func (s *AgentStore) BootstrapAgent(ctx context.Context, id, botID, ownerID int64, name, instruction, model, baseURL, apiKeyEnc string, maxTokens int, grant func(context.Context, int64) error) (bool, error) {
	if id <= 0 || ownerID <= 0 {
		return false, fmt.Errorf("bootstrap agent and owner IDs required")
	}
	created := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
		if maxTokens <= 0 {
			maxTokens = 4096
		}
		row := Agent{ID: id, BotUserID: botID, OwnerUserID: ownerID, Name: name, Status: "active", CurrentVersion: 1, DraftInstruction: instruction, DraftModel: model, DraftTools: `["search_messages","get_conversation_context","send_message"]`, DraftBaseURL: baseURL, DraftAPIKeyEnc: apiKeyEnc, DraftMaxTokens: maxTokens, DraftRevision: 1}
		// OnConflict makes concurrent bootstraps from multiple workers safe:
		// exactly one insert wins; the losers observe RowsAffected == 0.
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// Lost the creation race to another worker.
			return nil
		}
		created = true
		version := AgentVersion{AgentID: id, Version: 1, Instruction: instruction, Model: model, Tools: row.DraftTools, BaseURL: baseURL, APIKeyEnc: apiKeyEnc, MaxTokens: maxTokens}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&version).Error; err != nil {
			return err
		}
		if grant != nil {
			return grant(ctx, id)
		}
		return nil
	})
	return created, err
}
