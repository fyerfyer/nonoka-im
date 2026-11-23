package platformdata

import (
	"context"
	"errors"
	"time"

	"nonoka-im/internal/agent/domain"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ListModelServices returns model services ordered by name. includeDisabled is
// only set by the platform-admin management surface.
func (s *Store) ListModelServices(ctx context.Context, includeDisabled bool) ([]domain.ModelService, error) {
	var rows []ModelServiceRow
	q := s.db.WithContext(ctx).Order("name asc")
	if !includeDisabled {
		q = q.Where("enabled = ?", true)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, transient(err)
	}
	out := make([]domain.ModelService, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

func (s *Store) GetModelService(ctx context.Context, id uint64) (*domain.ModelService, error) {
	var row ModelServiceRow
	err := s.db.WithContext(ctx).First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrModelServiceNotFound
	}
	if err != nil {
		return nil, transient(err)
	}
	service := row.toDomain()
	return &service, nil
}

func (s *Store) GetModelServiceByName(ctx context.Context, name string) (*domain.ModelService, error) {
	var row ModelServiceRow
	err := s.db.WithContext(ctx).Where("name = ?", name).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrModelServiceNotFound
	}
	if err != nil {
		return nil, transient(err)
	}
	service := row.toDomain()
	return &service, nil
}

func (s *Store) CreateModelService(ctx context.Context, spec domain.ModelServiceSpec, apiKeyEnc string) (*domain.ModelService, error) {
	row := ModelServiceRow{
		Name: spec.Name, DisplayName: spec.DisplayName, GatewayType: spec.GatewayType,
		BaseURL: spec.BaseURL, GatewayKeyID: spec.GatewayKeyID, APIKeyEnc: apiKeyEnc,
		Enabled: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if spec.Enabled != nil {
		row.Enabled = *spec.Enabled
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, domain.ErrModelServiceConflict
		}
		return nil, transient(err)
	}
	service := row.toDomain()
	return &service, nil
}

func (s *Store) UpdateModelService(ctx context.Context, id uint64, spec domain.ModelServiceSpec, apiKeyEnc string, updateKey bool) (*domain.ModelService, error) {
	updates := map[string]interface{}{
		"name":           spec.Name,
		"display_name":   spec.DisplayName,
		"gateway_type":   spec.GatewayType,
		"base_url":       spec.BaseURL,
		"gateway_key_id": spec.GatewayKeyID,
		"updated_at":     time.Now(),
	}
	if spec.Enabled != nil {
		updates["enabled"] = *spec.Enabled
	}
	if updateKey {
		updates["api_key_enc"] = apiKeyEnc
	}
	result := s.db.WithContext(ctx).Model(&ModelServiceRow{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return nil, transient(result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, domain.ErrModelServiceNotFound
	}
	return s.GetModelService(ctx, id)
}

// DeleteModelService removes a service only when no published agent version
// references it, preserving historical version reproducibility.
func (s *Store) DeleteModelService(ctx context.Context, id uint64) error {
	var refs int64
	if err := s.db.WithContext(ctx).Table("agent_versions").Where("model_service_id = ?", id).Count(&refs).Error; err != nil {
		return transient(err)
	}
	if refs > 0 {
		return domain.ErrModelServiceInUse
	}
	result := s.db.WithContext(ctx).Delete(&ModelServiceRow{}, id)
	if result.Error != nil {
		return transient(result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrModelServiceNotFound
	}
	return nil
}

func (s *Store) ListModels(ctx context.Context, serviceID uint64, includeDisabled bool) ([]domain.Model, error) {
	var rows []ModelRow
	q := s.db.WithContext(ctx).Where("service_id = ?", serviceID).Order("model_key asc")
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

// ListAllModels returns models for every service in one query; the transport
// layer groups them by service to avoid N+1 list calls.
func (s *Store) ListAllModels(ctx context.Context, includeDisabled bool) ([]domain.Model, error) {
	var rows []ModelRow
	q := s.db.WithContext(ctx).Order("service_id asc, model_key asc")
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
	var row ModelRow
	err := s.db.WithContext(ctx).First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrModelNotFound
	}
	if err != nil {
		return nil, transient(err)
	}
	model := row.toDomain()
	return &model, nil
}

func (s *Store) CreateModel(ctx context.Context, spec domain.ModelSpec) (*domain.Model, error) {
	row := modelRowFromDomain(domain.Model{
		ServiceID: spec.ServiceID, ModelKey: spec.ModelKey, DisplayName: spec.DisplayName,
		ContextWindow: spec.ContextWindow, MaxOutputTokens: spec.MaxOutputTokens,
		SupportsTools: spec.SupportsTools, SupportsVision: spec.SupportsVision, SupportsReasoning: spec.SupportsReasoning,
		InputPricePerMTok: spec.InputPricePerMTok, OutputPricePerMTok: spec.OutputPricePerMTok,
		Enabled: spec.Enabled, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, transient(err)
	}
	model := row.toDomain()
	return &model, nil
}

func (s *Store) UpdateModel(ctx context.Context, id uint64, spec domain.ModelSpec) (*domain.Model, error) {
	result := s.db.WithContext(ctx).Model(&ModelRow{}).Where("id = ?", id).Updates(map[string]interface{}{
		"display_name":          spec.DisplayName,
		"context_window":        spec.ContextWindow,
		"max_output_tokens":     spec.MaxOutputTokens,
		"supports_tools":        spec.SupportsTools,
		"supports_vision":       spec.SupportsVision,
		"supports_reasoning":    spec.SupportsReasoning,
		"input_price_per_mtok":  spec.InputPricePerMTok,
		"output_price_per_mtok": spec.OutputPricePerMTok,
		"enabled":               spec.Enabled,
		"updated_at":            time.Now(),
	})
	if result.Error != nil {
		return nil, transient(result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, domain.ErrModelNotFound
	}
	return s.GetModel(ctx, id)
}

// UpsertModels writes the gateway catalog snapshot. Existing rows keep their
// local enabled flag so an admin can disable a model without the next sync
// re-enabling it.
func (s *Store) UpsertModels(ctx context.Context, serviceID uint64, models []domain.Model) error {
	if len(models) == 0 {
		return nil
	}
	now := time.Now()
	rows := make([]ModelRow, 0, len(models))
	for _, m := range models {
		row := modelRowFromDomain(m)
		row.ServiceID = serviceID
		row.LastSyncedAt = &now
		if row.MaxOutputTokens <= 0 {
			row.MaxOutputTokens = 4096
		}
		if row.DisplayName == "" {
			row.DisplayName = row.ModelKey
		}
		rows = append(rows, row)
	}
	return transient(s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "service_id"}, {Name: "model_key"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"display_name", "context_window", "max_output_tokens", "supports_tools",
			"supports_vision", "supports_reasoning", "input_price_per_mtok",
			"output_price_per_mtok", "last_synced_at", "updated_at",
		}),
	}).Create(&rows).Error)
}

func (s *Store) DeleteModel(ctx context.Context, id uint64) error {
	result := s.db.WithContext(ctx).Delete(&ModelRow{}, id)
	if result.Error != nil {
		return transient(result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrModelNotFound
	}
	return nil
}

// ModelServiceReferencedByAgent reports whether any published version pins the
// service. Used by management usecases before destructive edits.
func (s *Store) ModelServiceReferencedByAgent(ctx context.Context, id uint64) (bool, error) {
	var refs int64
	if err := s.db.WithContext(ctx).Table("agent_versions").Where("model_service_id = ?", id).Count(&refs).Error; err != nil {
		return false, transient(err)
	}
	return refs > 0, nil
}
