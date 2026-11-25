package platformdata

import (
	"context"
	"errors"
	"time"

	"nonoka-im/internal/agent/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Store) ListModels(ctx context.Context, includeDisabled bool) ([]domain.Model, error) {
	var rows []ModelCatalogRow
	q := s.db.WithContext(ctx).Order("enabled desc, sort_order asc, id asc")
	if !includeDisabled {
		q = q.Where("enabled = ?", true)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, transient(err)
	}
	out := make([]domain.Model, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

func (s *Store) GetModel(ctx context.Context, id uint64) (*domain.Model, error) {
	var row ModelCatalogRow
	err := s.db.WithContext(ctx).First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrModelNotFound
	}
	if err != nil {
		return nil, transient(err)
	}
	m := row.toDomain()
	return &m, nil
}

func (s *Store) GetModelByKey(ctx context.Context, modelKey string) (*domain.Model, error) {
	var row ModelCatalogRow
	err := s.db.WithContext(ctx).Where("model_key = ?", modelKey).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrModelNotFound
	}
	if err != nil {
		return nil, transient(err)
	}
	m := row.toDomain()
	return &m, nil
}

// UpsertModels refreshes the Bifrost-owned fields while preserving local
// curation (display name, icon, capabilities, enabled, recommended, order).
func (s *Store) UpsertModels(ctx context.Context, models []domain.Model) error {
	if len(models) == 0 {
		return nil
	}
	now := time.Now()
	return transient(s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range models {
			m := models[i]
			if m.MaxOutputTokens <= 0 {
				m.MaxOutputTokens = 4096
			}
			if m.DisplayName == "" {
				m.DisplayName = m.ModelKey
			}
			if m.GatewayModelKey == "" {
				m.GatewayModelKey = m.ModelKey
			}
			err := tx.Exec(`
				INSERT INTO model_catalog (
					model_key, provider, gateway_model_key, display_name, icon,
					context_window, max_output_tokens, supports_tools, supports_vision,
					supports_reasoning, input_price_per_mtok, output_price_per_mtok,
					input_cache_read_price_per_mtok, enabled, curated, recommended,
					sort_order, last_synced_at, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT (model_key) DO UPDATE SET
					provider = EXCLUDED.provider,
					gateway_model_key = EXCLUDED.gateway_model_key,
					context_window = EXCLUDED.context_window,
					max_output_tokens = EXCLUDED.max_output_tokens,
					input_price_per_mtok = EXCLUDED.input_price_per_mtok,
					output_price_per_mtok = EXCLUDED.output_price_per_mtok,
					input_cache_read_price_per_mtok = EXCLUDED.input_cache_read_price_per_mtok,
					-- Capabilities are gateway facts; never regress them when a
					-- provider temporarily reports a narrower capability set.
					supports_tools = model_catalog.supports_tools OR EXCLUDED.supports_tools,
					supports_vision = model_catalog.supports_vision OR EXCLUDED.supports_vision,
					supports_reasoning = model_catalog.supports_reasoning OR EXCLUDED.supports_reasoning,
					display_name = CASE WHEN model_catalog.curated THEN model_catalog.display_name ELSE EXCLUDED.display_name END,
					last_synced_at = EXCLUDED.last_synced_at,
					updated_at = EXCLUDED.updated_at
			`,
				m.ModelKey, m.Provider, m.GatewayModelKey, m.DisplayName, m.Icon,
				m.ContextWindow, m.MaxOutputTokens, m.SupportsTools, m.SupportsVision,
				m.SupportsReasoning, m.InputPricePerMTok, m.OutputPricePerMTok,
				m.InputCacheReadPricePerMTok, m.Enabled, m.Curated, m.Recommended,
				m.SortOrder, now, now, now,
			).Error
			if err != nil {
				return err
			}
		}
		return nil
	}))
}

func (s *Store) UpdateModelRouting(ctx context.Context, id uint64, modelKey, provider, gatewayModelKey string) error {
	res := s.db.WithContext(ctx).Model(&ModelCatalogRow{}).Where("id = ?", id).Updates(map[string]interface{}{
		"model_key": modelKey, "provider": provider, "gateway_model_key": gatewayModelKey,
		"updated_at": time.Now(),
	})
	if res.Error != nil {
		return transient(res.Error)
	}
	return nil
}

// MergeModel moves every agent/version reference from one catalog row to
// another and preserves operator curation flags before deleting the source.
func (s *Store) MergeModel(ctx context.Context, fromID, toID uint64) error {
	if fromID == toID {
		return nil
	}
	return transient(s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("agents").Where("draft_model_id = ?", fromID).
			Update("draft_model_id", toID).Error; err != nil {
			return err
		}
		if err := tx.Table("agent_versions").Where("model_id = ?", fromID).
			Update("model_id", toID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE model_catalog t SET
				enabled = t.enabled OR s.enabled,
				curated = t.curated OR s.curated,
				recommended = t.recommended OR s.recommended,
				sort_order = CASE WHEN t.sort_order = 0 THEN s.sort_order ELSE t.sort_order END
			FROM model_catalog s WHERE t.id = ? AND s.id = ?`, toID, fromID).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", fromID).Delete(&ModelCatalogRow{}).Error
	}))
}

func (s *Store) UpdateModelCuration(ctx context.Context, id uint64, curation domain.ModelCuration) (*domain.Model, error) {
	values := map[string]interface{}{"curated": true, "updated_at": time.Now()}
	if curation.DisplayName != "" {
		values["display_name"] = curation.DisplayName
	}
	if curation.Icon != "" {
		values["icon"] = curation.Icon
	}
	if curation.SupportsTools != nil {
		values["supports_tools"] = *curation.SupportsTools
	}
	if curation.SupportsVision != nil {
		values["supports_vision"] = *curation.SupportsVision
	}
	if curation.SupportsReasoning != nil {
		values["supports_reasoning"] = *curation.SupportsReasoning
	}
	if curation.Enabled != nil {
		values["enabled"] = *curation.Enabled
	}
	if curation.Recommended != nil {
		values["recommended"] = *curation.Recommended
	}
	if curation.SortOrder != nil {
		values["sort_order"] = *curation.SortOrder
	}
	res := s.db.WithContext(ctx).Model(&ModelCatalogRow{}).Where("id = ?", id).Updates(values)
	if res.Error != nil {
		return nil, transient(res.Error)
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrModelNotFound
	}
	return s.GetModel(ctx, id)
}

func (s *Store) GetAgentGatewayKey(ctx context.Context, agentID int64) (*domain.AgentGatewayKey, error) {
	var row AgentGatewayKeyRow
	err := s.db.WithContext(ctx).Where("agent_id = ?", agentID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrGatewayKeyNotFound
	}
	if err != nil {
		return nil, transient(err)
	}
	k := row.toDomain()
	return &k, nil
}

func (s *Store) UpsertAgentGatewayKey(ctx context.Context, key domain.AgentGatewayKey) error {
	row := AgentGatewayKeyRow{
		AgentID: key.AgentID, VirtualKeyID: key.VirtualKeyID, ValueEnc: key.ValueEnc,
		Status: key.Status, LastError: key.LastError, SpecHash: key.SpecHash,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	return transient(s.db.WithContext(ctx).Clauses(
		clause.OnConflict{
			Columns:   []clause.Column{{Name: "agent_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"virtual_key_id", "value_enc", "status", "last_error", "spec_hash", "updated_at"}),
		},
	).Create(&row).Error)
}

func (s *Store) SetAgentGatewayKeyStatus(ctx context.Context, agentID int64, status, lastErr string) error {
	return s.setAgentGatewayKeyStatus(ctx, agentID, status, lastErr, "")
}

func (s *Store) SetAgentGatewayKeySynced(ctx context.Context, agentID int64, specHash string) error {
	return s.setAgentGatewayKeyStatus(ctx, agentID, domain.GatewayKeyActive, "", specHash)
}

func (s *Store) setAgentGatewayKeyStatus(ctx context.Context, agentID int64, status, lastErr, specHash string) error {
	values := map[string]interface{}{"status": status, "last_error": lastErr, "updated_at": time.Now()}
	if specHash != "" {
		values["spec_hash"] = specHash
	}
	res := s.db.WithContext(ctx).Model(&AgentGatewayKeyRow{}).Where("agent_id = ?", agentID).
		Updates(values)
	if res.Error != nil {
		return transient(res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrGatewayKeyNotFound
	}
	return nil
}

func (s *Store) DeleteAgentGatewayKey(ctx context.Context, agentID int64) error {
	err := s.db.WithContext(ctx).Where("agent_id = ?", agentID).Delete(&AgentGatewayKeyRow{}).Error
	return transient(err)
}

func (s *Store) GetPlatformGatewayKey(ctx context.Context, kind string) (*domain.PlatformGatewayKey, error) {
	var row PlatformGatewayKeyRow
	err := s.db.WithContext(ctx).Where("kind = ?", kind).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrGatewayKeyNotFound
	}
	if err != nil {
		return nil, transient(err)
	}
	k := row.toDomain()
	return &k, nil
}

func (s *Store) UpsertPlatformGatewayKey(ctx context.Context, key domain.PlatformGatewayKey) error {
	row := PlatformGatewayKeyRow{
		Kind: key.Kind, VirtualKeyID: key.VirtualKeyID, ValueEnc: key.ValueEnc,
		Status: key.Status, LastError: key.LastError, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	return transient(s.db.WithContext(ctx).Clauses(
		clause.OnConflict{
			Columns:   []clause.Column{{Name: "kind"}},
			DoUpdates: clause.AssignmentColumns([]string{"virtual_key_id", "value_enc", "status", "last_error", "updated_at"}),
		},
	).Create(&row).Error)
}

func (s *Store) EnqueueGatewayKeyCleanup(ctx context.Context, cleanup domain.GatewayKeyCleanup) error {
	row := GatewayKeyCleanupRow{
		VirtualKeyID: cleanup.VirtualKeyID, AgentID: cleanup.AgentID, Reason: cleanup.Reason,
		Attempts: cleanup.Attempts, NextAttemptAt: cleanup.NextAttemptAt,
		LastError: cleanup.LastError, CreatedAt: time.Now(),
	}
	if row.NextAttemptAt.IsZero() {
		row.NextAttemptAt = time.Now()
	}
	return transient(s.db.WithContext(ctx).Clauses(
		clause.OnConflict{
			Columns:   []clause.Column{{Name: "virtual_key_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"attempts", "next_attempt_at", "last_error"}),
		},
	).Create(&row).Error)
}

func (s *Store) DueGatewayKeyCleanups(ctx context.Context, limit int) ([]domain.GatewayKeyCleanup, error) {
	if limit <= 0 {
		limit = 20
	}
	var rows []GatewayKeyCleanupRow
	err := s.db.WithContext(ctx).
		Where("next_attempt_at <= now()").
		Order("next_attempt_at asc").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, transient(err)
	}
	out := make([]domain.GatewayKeyCleanup, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

func (s *Store) CompleteGatewayKeyCleanup(ctx context.Context, virtualKeyID string) error {
	return transient(s.db.WithContext(ctx).Where("virtual_key_id = ?", virtualKeyID).
		Delete(&GatewayKeyCleanupRow{}).Error)
}

func (s *Store) FailGatewayKeyCleanup(ctx context.Context, virtualKeyID string, attempts int, next time.Time, lastErr string) error {
	res := s.db.WithContext(ctx).Model(&GatewayKeyCleanupRow{}).Where("virtual_key_id = ?", virtualKeyID).
		Updates(map[string]interface{}{
			"attempts": attempts, "next_attempt_at": next, "last_error": lastErr,
		})
	if res.Error != nil {
		return transient(res.Error)
	}
	return nil
}
