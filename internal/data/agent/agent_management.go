package agentdata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"nonoka-im/internal/agent/domain"
	imdata "nonoka-im/internal/data/im"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CreateAgent creates the bot user and the agent row in one transaction. The
// grant callback writes the OpenFGA owner tuple inside the same transaction;
// a failed grant rolls the rows back.
func (s *Store) CreateAgent(ctx context.Context, ownerID int64, spec domain.AgentSpec, grant func(context.Context, int64) error) (*domain.Agent, error) {
	var result domain.Agent
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		bot := imdata.User{Username: "agent_" + uuid.NewString()[:12], Password: "!", IsBot: true}
		if err := tx.Create(&bot).Error; err != nil {
			return err
		}
		row := AgentRow{
			BotUserID: bot.ID, OwnerUserID: ownerID, Name: spec.Name, Description: spec.Description,
			Status: "draft", DraftInstruction: spec.Instruction, DraftTools: spec.Tools,
			DraftModelID:           spec.ModelID,
			DraftCapabilityPresets: spec.CapabilityPresets, DraftToolPolicies: spec.ToolPolicies,
			DraftMaxTokens: spec.MaxTokens, DraftMCPBindings: spec.MCPBindings, DraftSkills: spec.Skills,
			DraftRevision: 1,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		result = row.toDomain()
		if grant != nil {
			return grant(ctx, row.ID)
		}
		return nil
	})
	if err != nil {
		return nil, transientOrDomain(err)
	}
	return &result, nil
}

func (s *Store) GetAgent(ctx context.Context, id int64) (*domain.Agent, error) {
	var row AgentRow
	err := s.db.WithContext(ctx).First(&row, id).Error
	if err != nil {
		return nil, notFoundOrTransient(err, domain.ErrAgentNotFound)
	}
	agent := row.toDomain()
	return &agent, nil
}

func (s *Store) GetAgentByBot(ctx context.Context, botID int64) (*domain.Agent, error) {
	var row AgentRow
	err := s.db.WithContext(ctx).Where("bot_user_id = ?", botID).First(&row).Error
	if err != nil {
		return nil, notFoundOrTransient(err, domain.ErrAgentNotFound)
	}
	agent := row.toDomain()
	return &agent, nil
}

// ListAgents returns one page of agents ordered by ID, plus the total count
// of agents for pagination.
func (s *Store) ListAgents(ctx context.Context, limit, offset int) ([]domain.Agent, int64, error) {
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
	if err := s.db.WithContext(ctx).Model(&AgentRow{}).Count(&total).Error; err != nil {
		return nil, 0, transientOrDomain(err)
	}
	var rows []AgentRow
	err := s.db.WithContext(ctx).Order("id asc").Limit(limit).Offset(offset).Find(&rows).Error
	if err != nil {
		return nil, 0, transientOrDomain(err)
	}
	out := make([]domain.Agent, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, total, nil
}

// ListPublishedAgents returns agents that have at least one published version.
// The worker uses it to backfill missing Bifrost keys after migration or a
// gateway outage.
func (s *Store) ListPublishedAgents(ctx context.Context) ([]domain.Agent, error) {
	var rows []AgentRow
	if err := s.db.WithContext(ctx).Where("current_version > 0").Order("id asc").Find(&rows).Error; err != nil {
		return nil, transientOrDomain(err)
	}
	out := make([]domain.Agent, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

func (s *Store) ActiveAgentsByBot(ctx context.Context, ids []int64) ([]domain.Agent, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []AgentRow
	err := s.db.WithContext(ctx).Where("bot_user_id IN ? AND status = ? AND current_version > 0", ids, "active").Find(&rows).Error
	if err != nil {
		return nil, transientOrDomain(err)
	}
	out := make([]domain.Agent, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

func (s *Store) GetVersion(ctx context.Context, agentID, version int64) (*domain.AgentVersion, error) {
	var row AgentVersionRow
	err := s.db.WithContext(ctx).Where("agent_id = ? AND version = ?", agentID, version).First(&row).Error
	if err != nil {
		return nil, notFoundOrTransient(err, domain.ErrVersionNotFound)
	}
	versionRow := row.toDomain()
	return &versionRow, nil
}

// UpdateDraft replaces the draft configuration. updateKey=true overwrites the
// sealed API key; false keeps the existing one.
func (s *Store) UpdateDraft(ctx context.Context, id, revision int64, spec domain.AgentSpec, updateKey bool) (*domain.Agent, error) {
	updates := map[string]any{
		"name": spec.Name, "description": spec.Description,
		"draft_instruction": spec.Instruction, "draft_tools": spec.Tools,
		"draft_model_id":           spec.ModelID,
		"draft_capability_presets": spec.CapabilityPresets, "draft_tool_policies": spec.ToolPolicies,
		"draft_max_tokens": spec.MaxTokens, "draft_mcp_bindings": spec.MCPBindings,
		"draft_skills": spec.Skills, "draft_revision": gorm.Expr("draft_revision + 1"),
		"updated_at": time.Now(),
	}
	_ = updateKey
	result := s.db.WithContext(ctx).Model(&AgentRow{}).Where("id = ? AND draft_revision = ?", id, revision).Updates(updates)
	if result.Error != nil {
		return nil, transientOrDomain(result.Error)
	}
	if result.RowsAffected != 1 {
		return nil, ErrDraftConflict
	}
	return s.GetAgent(ctx, id)
}

// ListVersions returns every published version, newest first.
func (s *Store) ListVersions(ctx context.Context, agentID int64) ([]domain.AgentVersion, error) {
	var rows []AgentVersionRow
	if err := s.db.WithContext(ctx).Where("agent_id = ?", agentID).Order("version desc").Find(&rows).Error; err != nil {
		return nil, transientOrDomain(err)
	}
	out := make([]domain.AgentVersion, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

// RestoreDraft copies a published version back into the editable draft. It
// never changes current_version; the caller must publish explicitly.
func (s *Store) RestoreDraft(ctx context.Context, id, version, expectedRevision int64) (*domain.Agent, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row AgentRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrAgentNotFound
			}
			return transientOrDomain(err)
		}
		if expectedRevision > 0 && row.DraftRevision != expectedRevision {
			return domain.ErrDraftConflict
		}
		var source AgentVersionRow
		if err := tx.Where("agent_id = ? AND version = ?", id, version).First(&source).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrVersionNotFound
			}
			return transientOrDomain(err)
		}
		updates := map[string]any{
			"draft_instruction": source.Instruction, "draft_tools": source.Tools,
			"draft_model_id":           source.ModelID,
			"draft_capability_presets": source.CapabilityPresets, "draft_tool_policies": source.ToolPolicies,
			"draft_max_tokens": source.MaxTokens, "draft_mcp_bindings": source.MCPBindings,
			"draft_skills": source.Skills, "draft_revision": gorm.Expr("draft_revision + 1"),
			"updated_at": time.Now(),
		}
		return tx.Model(&AgentRow{}).Where("id = ?", id).Updates(updates).Error
	})
	if err != nil {
		return nil, err
	}
	return s.GetAgent(ctx, id)
}

func (s *Store) Publish(ctx context.Context, id, revision int64, note string, publishedBy int64) (*domain.AgentVersion, error) {
	var result domain.AgentVersion
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row AgentRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrAgentNotFound
			}
			return transientOrDomain(err)
		}
		if row.DraftRevision != revision {
			return ErrDraftConflict
		}
		// Publishing the same draft revision twice is idempotent. This makes a
		// retry after a post-commit audit/transport failure return the already
		// published version instead of creating a duplicate.
		var existing AgentVersionRow
		existingErr := tx.Where("agent_id = ? AND source_revision = ?", id, revision).First(&existing).Error
		if existingErr == nil {
			result = existing.toDomain()
			return nil
		}
		if !errors.Is(existingErr, gorm.ErrRecordNotFound) {
			return existingErr
		}
		if row.DraftModelID <= 0 {
			return fmt.Errorf("agent %d has no model selected", id)
		}
		var modelKey string
		if err := tx.Table("model_catalog").Where("id = ?", row.DraftModelID).Pluck("model_key", &modelKey).Error; err != nil {
			return err
		}
		if modelKey == "" {
			return domain.ErrModelNotFound
		}
		now := time.Now()
		version := AgentVersionRow{
			AgentID: id, Version: row.CurrentVersion + 1, Instruction: row.DraftInstruction,
			ModelID: row.DraftModelID, ModelKey: modelKey,
			Tools: row.DraftTools, CapabilityPresets: row.DraftCapabilityPresets,
			ToolPolicies: row.DraftToolPolicies, MaxTokens: row.DraftMaxTokens,
			MCPBindings: row.DraftMCPBindings, Skills: row.DraftSkills,
			PublishNote: note, PublishedBy: publishedBy, PublishedAt: &now,
			SourceRevision: row.DraftRevision,
		}
		if err := tx.Create(&version).Error; err != nil {
			return err
		}
		if err := tx.Model(&AgentRow{}).Where("id = ?", id).Updates(map[string]interface{}{"current_version": version.Version, "status": "active", "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		result = version.toDomain()
		return nil
	})
	if err != nil {
		return nil, transientOrDomain(err)
	}
	return &result, nil
}

// DeleteAgent hard-deletes the agent configuration and its session-scoped
// data while preserving IM messages and the bot user (historical messages
// still reference it). OpenFGA tuples are cleaned up by the caller after the
// transaction commits; a stale tuple is harmless because the agent row no
// longer exists.
func (s *Store) DeleteAgent(ctx context.Context, id int64) ([]string, error) {
	tools := make(map[string]struct{})
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var runIDs []uint64
		if err := tx.Model(&AgentRunRow{}).Where("agent_id = ?", id).Pluck("id", &runIDs).Error; err != nil {
			return err
		}
		var sessionIDs []uint64
		if err := tx.Model(&AgentSessionRow{}).Where("agent_id = ?", id).Pluck("id", &sessionIDs).Error; err != nil {
			return err
		}
		if len(runIDs) > 0 {
			if err := tx.Where("run_id IN ?", runIDs).Delete(&AgentApprovalRow{}).Error; err != nil {
				return err
			}
			if err := tx.Where("run_id IN ?", runIDs).Delete(&AgentCheckpointRow{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("agent_id = ?", id).Delete(&AgentTurnRow{}).Error; err != nil {
			return err
		}
		if len(sessionIDs) > 0 {
			if err := tx.Where("session_id IN ?", sessionIDs).Delete(&AgentContextSnapshotRow{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("agent_id = ?", id).Delete(&AgentSessionRow{}).Error; err != nil {
			return err
		}
		if err := tx.Where("agent_id = ?", id).Delete(&AgentRunRow{}).Error; err != nil {
			return err
		}
		var versions []AgentVersionRow
		if err := tx.Where("agent_id = ?", id).Find(&versions).Error; err != nil {
			return err
		}
		for _, version := range versions {
			var names []string
			if json.Unmarshal([]byte(version.Tools), &names) == nil {
				for _, name := range names {
					tools[name] = struct{}{}
				}
			}
			var bindings []domain.MCPBinding
			if json.Unmarshal([]byte(version.MCPBindings), &bindings) == nil {
				for _, binding := range bindings {
					tools[binding.Exposed] = struct{}{}
				}
			}
		}
		if err := tx.Where("agent_id = ?", id).Delete(&AgentVersionRow{}).Error; err != nil {
			return err
		}
		return tx.Delete(&AgentRow{}, id).Error
	})
	if err != nil {
		return nil, transientOrDomain(err)
	}
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	return names, nil
}

// RollbackPublish removes a just-published version and restores the agent row
// to its previous state. It refuses to roll back if a newer publish won the
// race.
func (s *Store) RollbackPublish(ctx context.Context, id, version int64) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row AgentRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		if row.CurrentVersion != version {
			return fmt.Errorf("agent %d advanced past version %d; refusing rollback", id, version)
		}
		if err := tx.Where("agent_id = ? AND version = ?", id, version).Delete(&AgentVersionRow{}).Error; err != nil {
			return err
		}
		updates := map[string]interface{}{"current_version": version - 1, "updated_at": time.Now()}
		if version-1 == 0 {
			updates["status"] = "draft"
		}
		return tx.Model(&AgentRow{}).Where("id = ?", id).Updates(updates).Error
	})
	return transientOrDomain(err)
}

func (s *Store) SetAgentStatus(ctx context.Context, id int64, status string) error {
	if status != "active" && status != "disabled" {
		return fmt.Errorf("invalid agent status")
	}
	return transientOrDomain(s.db.WithContext(ctx).Model(&AgentRow{}).Where("id = ? AND current_version > 0", id).Update("status", status).Error)
}

func (s *Store) ListRuns(ctx context.Context, agentID int64, limit int) ([]domain.AgentRun, error) {
	rows, _, err := s.ListRunsPage(ctx, agentID, limit, 0)
	return rows, err
}

// ListRunsPage returns one newest-first page and the total run count.
func (s *Store) ListRunsPage(ctx context.Context, agentID int64, limit, offset int) ([]domain.AgentRun, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var total int64
	if err := s.db.WithContext(ctx).Model(&AgentRunRow{}).Where("agent_id = ?", agentID).Count(&total).Error; err != nil {
		return nil, 0, transientOrDomain(err)
	}
	var rows []AgentRunRow
	err := s.db.WithContext(ctx).Where("agent_id = ?", agentID).Order("id desc").Limit(limit).Offset(offset).Find(&rows).Error
	if err != nil {
		return nil, 0, transientOrDomain(err)
	}
	out := make([]domain.AgentRun, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, total, nil
}

func (s *Store) GetRun(ctx context.Context, id uint64) (*domain.AgentRun, error) {
	var row AgentRunRow
	err := s.db.WithContext(ctx).First(&row, id).Error
	if err != nil {
		return nil, notFoundOrTransient(err, domain.ErrRunNotFound)
	}
	run := row.toDomain()
	return &run, nil
}

// SetRunAction applies an explicit operator action to a terminal run. A retry
// clears the checkpoint and attempt state so the run starts fresh (and asks for
// approval again) instead of replaying an interrupted side effect. If the run
// already committed a side effect, the operator must explicitly acknowledge it.
func (s *Store) SetRunAction(ctx context.Context, id uint64, action string, acknowledgeSideEffect bool) error {
	if action == "cancel" {
		return s.cancelRun(ctx, id)
	}
	var from, to string
	updates := map[string]interface{}{}
	switch action {
	case "retry":
		var sideEffectAt sql.NullTime
		if err := s.db.WithContext(ctx).Model(&AgentRunRow{}).Where("id = ?", id).Pluck("side_effect_at", &sideEffectAt).Error; err != nil {
			return fmt.Errorf("%w: %w", domain.ErrTransient, err)
		}
		if sideEffectAt.Valid && !acknowledgeSideEffect {
			return domain.ErrSideEffectCommitted
		}
		from, to = domain.RunFailed, domain.RunQueued
		updates["checkpoint_id"] = ""
		updates["attempt_id"] = ""
		updates["lease_until"] = nil
		updates["started_at"] = nil
		updates["finished_at"] = nil
		updates["error_code"] = ""
		updates["error_class"] = ""
		updates["last_error"] = ""
		updates["retry_after_at"] = nil
		updates["attempts"] = 0
		updates["next_attempt_at"] = time.Now()
		if acknowledgeSideEffect {
			updates["side_effect_at"] = nil
		}
	case "skip":
		from, to = domain.RunFailed, domain.RunCancelled
	default:
		return fmt.Errorf("invalid run action")
	}
	updates["status"] = to
	result := s.db.WithContext(ctx).Model(&AgentRunRow{}).Where("id = ? AND status = ?", id, from).Updates(updates)
	if result.Error != nil {
		return transientOrDomain(result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrRunState
	}
	return nil
}

// cancelRun marks any non-terminal run cancelled. Clearing attempt_id and
// lease_until makes the executor's heartbeat lose the fence, which cancels the
// in-flight model/tool context. Pending approvals are expired in the same
// transaction so they cannot be approved after cancellation.
func (s *Store) cancelRun(ctx context.Context, id uint64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&AgentRunRow{}).
			Where("id = ? AND status IN ?", id, []string{domain.RunQueued, domain.RunRunning, domain.RunWaitingApproval}).
			Updates(map[string]interface{}{
				"status": domain.RunCancelled, "error_code": "cancelled", "error_class": "",
				"attempt_id": "", "lease_until": nil, "next_attempt_at": nil,
				"checkpoint_id": "", "finished_at": time.Now(),
			})
		if res.Error != nil {
			return transientOrDomain(res.Error)
		}
		if res.RowsAffected != 1 {
			return ErrRunState
		}
		return tx.Model(&AgentApprovalRow{}).
			Where("run_id = ? AND status = ?", id, domain.ApprovalPending).
			Updates(map[string]interface{}{"status": domain.ApprovalExpired, "updated_at": time.Now()}).Error
	})
}

// BootstrapAgent idempotently creates the preset agent. toolsJSON is the
// frozen built-in tool snapshot supplied by the composition root; the data
// layer deliberately does not know the tool catalog. It returns created=false
// (without error) when the agent already exists.
func (s *Store) BootstrapAgent(ctx context.Context, id, botID, ownerID int64, name, instruction string, modelID int64, modelKey string, maxTokens int, toolsJSON string, grant func(context.Context, int64) error) (bool, error) {
	if id <= 0 || ownerID <= 0 {
		return false, fmt.Errorf("bootstrap agent and owner IDs required")
	}
	created := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing AgentRow
		if err := tx.First(&existing, id).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if botID <= 0 {
			bot := imdata.User{Username: "agent_" + uuid.NewString()[:12], Password: "!", IsBot: true}
			if err := tx.Create(&bot).Error; err != nil {
				return err
			}
			botID = bot.ID
		} else {
			// Never convert an existing human account into a bot. A misconfigured
			// AGENT_BOT_ID must fail loudly instead of overwriting a real user's
			// password and is_bot flag.
			var bot imdata.User
			if err := tx.First(&bot, botID).Error; err != nil {
				return notFoundOrTransient(err, fmt.Errorf("bootstrap bot user %d does not exist", botID))
			}
			if !bot.IsBot {
				return fmt.Errorf("bootstrap bot user %d is a human account; refusing to modify it", botID)
			}
		}
		if maxTokens <= 0 {
			maxTokens = 4096
		}
		row := AgentRow{
			ID: id, BotUserID: botID, OwnerUserID: ownerID, Name: name, Status: "active",
			CurrentVersion: 1, DraftInstruction: instruction, DraftTools: toolsJSON,
			DraftModelID:   modelID,
			DraftMaxTokens: maxTokens, DraftRevision: 1,
		}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		created = true
		now := time.Now()
		version := AgentVersionRow{
			AgentID: id, Version: 1, Instruction: instruction,
			ModelID: modelID, ModelKey: modelKey,
			Tools: row.DraftTools, MaxTokens: maxTokens, PublishedBy: ownerID,
			PublishedAt: &now, SourceRevision: 1,
		}
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

// notFoundOrTransient translates a GORM lookup error into a domain sentinel.
// Record-not-found is a deterministic condition callers may branch on; every
// other error is infrastructure noise that a retry may fix.
func notFoundOrTransient(err error, notFound error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return notFound
	}
	return fmt.Errorf("%w: %w", domain.ErrTransient, err)
}
