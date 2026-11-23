package platformdata

import (
	"context"
	"errors"
	"fmt"
	"time"

	"nonoka-im/internal/agent/domain"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Store is the PostgreSQL repository for platform resources: model services,
// MCP registries/connections and skills.
type Store struct{ db *gorm.DB }

func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

func (s *Store) CreateServer(ctx context.Context, spec domain.MCPServer) (*domain.MCPServer, error) {
	row := MCPServerRow{
		OwnerUserID: spec.OwnerUserID, Name: spec.Name, DisplayName: spec.DisplayName,
		Description: spec.Description, Transport: spec.Transport, URL: spec.URL,
		AuthType: spec.AuthType, CredentialEnc: spec.CredentialEnc, Visibility: spec.Visibility,
		ReviewStatus: spec.ReviewStatus, Enabled: spec.Enabled, HealthStatus: domain.MCPHealthUnknown,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if row.DisplayName == "" {
		row.DisplayName = row.Name
	}
	if row.Transport == "" {
		row.Transport = "streamable_http"
	}
	if row.AuthType == "" {
		row.AuthType = "none"
	}
	if row.Visibility == "" {
		row.Visibility = "private"
	}
	if row.ReviewStatus == "" {
		row.ReviewStatus = "approved"
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, domain.ErrMCPConflict
		}
		return nil, transient(err)
	}
	server := row.toDomain()
	return &server, nil
}

func (s *Store) GetServerByID(ctx context.Context, id uint64) (*domain.MCPServer, error) {
	var row MCPServerRow
	err := s.db.WithContext(ctx).First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrMCPNotFound
	}
	if err != nil {
		return nil, transient(err)
	}
	server := row.toDomain()
	return &server, nil
}

func (s *Store) ListServersForUser(ctx context.Context, userID int64, includeAll bool) ([]domain.MCPServer, error) {
	var rows []MCPServerRow
	q := s.db.WithContext(ctx).Order("id asc")
	if !includeAll {
		q = q.Where("owner_user_id = ? OR (visibility IN ? AND review_status = ? AND enabled = ?)",
			userID, []string{"shared", "public"}, "approved", true)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, transient(err)
	}
	out := make([]domain.MCPServer, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

// ListAllServers is used by the health checker and admin review queue.
func (s *Store) ListAllServers(ctx context.Context, enabledOnly bool) ([]domain.MCPServer, error) {
	var rows []MCPServerRow
	q := s.db.WithContext(ctx).Order("id asc")
	if enabledOnly {
		q = q.Where("enabled = ?", true)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, transient(err)
	}
	out := make([]domain.MCPServer, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

func (s *Store) UpdateServer(ctx context.Context, server domain.MCPServer) error {
	updates := map[string]interface{}{
		"display_name": server.DisplayName,
		"description":  server.Description,
		"transport":    server.Transport,
		"url":          server.URL,
		"auth_type":    server.AuthType,
		"visibility":   server.Visibility,
		"enabled":      server.Enabled,
		"updated_at":   time.Now(),
	}
	if server.CredentialEnc != "" {
		updates["credential_enc"] = server.CredentialEnc
	}
	result := s.db.WithContext(ctx).Model(&MCPServerRow{}).Where("id = ?", server.ID).Updates(updates)
	if result.Error != nil {
		return transient(result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrMCPNotFound
	}
	return nil
}

func (s *Store) SetServerEnabled(ctx context.Context, id uint64, enabled bool) error {
	result := s.db.WithContext(ctx).Model(&MCPServerRow{}).Where("id = ?", id).
		Updates(map[string]interface{}{"enabled": enabled, "updated_at": time.Now()})
	if result.Error != nil {
		return transient(result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrMCPNotFound
	}
	return nil
}

func (s *Store) SetServerReview(ctx context.Context, id uint64, status, reason string, reviewer int64) error {
	now := time.Now()
	result := s.db.WithContext(ctx).Model(&MCPServerRow{}).Where("id = ?", id).Updates(map[string]interface{}{
		"review_status": status, "review_reason": reason, "review_by": reviewer,
		"review_at": &now, "updated_at": now,
	})
	if result.Error != nil {
		return transient(result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrMCPNotFound
	}
	return nil
}

func (s *Store) SetServerHealth(ctx context.Context, id uint64, status, lastErr string) error {
	now := time.Now()
	return transient(s.db.WithContext(ctx).Model(&MCPServerRow{}).Where("id = ?", id).Updates(map[string]interface{}{
		"health_status": status, "last_error": lastErr, "last_checked_at": &now, "updated_at": now,
	}).Error)
}

func (s *Store) DeleteServer(ctx context.Context, id uint64) error {
	result := s.db.WithContext(ctx).Delete(&MCPServerRow{}, id)
	if result.Error != nil {
		return transient(result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrMCPNotFound
	}
	return nil
}

// ReplaceTools writes a discovery snapshot. It bumps tools_revision only when
// a tool's schema hash changed (added/removed tools also count).
func (s *Store) ReplaceTools(ctx context.Context, serverID uint64, tools []domain.MCPTool) (bool, error) {
	changed := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing []MCPToolRow
		if err := tx.Where("server_id = ?", serverID).Find(&existing).Error; err != nil {
			return err
		}
		prev := make(map[string]MCPToolRow, len(existing))
		for _, row := range existing {
			prev[row.Name] = row
		}
		seen := make(map[string]bool, len(tools))
		for _, t := range tools {
			seen[t.Name] = true
			row, ok := prev[t.Name]
			if !ok || row.SchemaHash != t.SchemaHash || row.Description != t.Description || row.Risk != t.Risk {
				changed = true
			}
			updated := MCPToolRow{
				ID: row.ID, ServerID: serverID, Name: t.Name, Description: t.Description,
				InputSchema: t.InputSchema, OutputSchema: t.OutputSchema, Annotations: t.Annotations,
				Risk: t.Risk, SchemaHash: t.SchemaHash, Enabled: true,
				DiscoveredAt: time.Now(), UpdatedAt: time.Now(),
			}
			if ok {
				if err := tx.Model(&MCPToolRow{}).Where("id = ?", row.ID).Updates(map[string]interface{}{
					"description": updated.Description, "input_schema": updated.InputSchema,
					"output_schema": updated.OutputSchema, "annotations": updated.Annotations,
					"risk": updated.Risk, "schema_hash": updated.SchemaHash,
					"discovered_at": updated.DiscoveredAt, "updated_at": updated.UpdatedAt,
				}).Error; err != nil {
					return err
				}
			} else if err := tx.Create(&updated).Error; err != nil {
				return err
			}
		}
		for name := range prev {
			if !seen[name] {
				changed = true
				if err := tx.Where("server_id = ? AND name = ?", serverID, name).Delete(&MCPToolRow{}).Error; err != nil {
					return err
				}
			}
		}
		if changed {
			return tx.Model(&MCPServerRow{}).Where("id = ?", serverID).Updates(map[string]interface{}{
				"tools_revision": gorm.Expr("tools_revision + 1"), "updated_at": time.Now(),
			}).Error
		}
		return nil
	})
	return changed, transient(err)
}

func (s *Store) ListTools(ctx context.Context, serverID uint64) ([]domain.MCPTool, error) {
	var rows []MCPToolRow
	if err := s.db.WithContext(ctx).Where("server_id = ? AND enabled = ?", serverID, true).Order("name asc").Find(&rows).Error; err != nil {
		return nil, transient(err)
	}
	out := make([]domain.MCPTool, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

func (s *Store) GetTool(ctx context.Context, serverID uint64, name string) (*domain.MCPTool, error) {
	var row MCPToolRow
	err := s.db.WithContext(ctx).Where("server_id = ? AND name = ? AND enabled = ?", serverID, name, true).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrMCPToolNotFound
	}
	if err != nil {
		return nil, transient(err)
	}
	tool := row.toDomain()
	return &tool, nil
}

func (s *Store) UpsertConnection(ctx context.Context, conn domain.MCPConnection) (*domain.MCPConnection, error) {
	row := MCPConnectionRow{
		ServerID: conn.ServerID, UserID: conn.UserID, AuthType: conn.AuthType,
		CredentialEnc: conn.CredentialEnc, Status: conn.Status, ExpiresAt: conn.ExpiresAt,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "server_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"auth_type", "credential_enc", "status", "expires_at", "updated_at",
		}),
	}).Create(&row).Error
	if err != nil {
		return nil, transient(err)
	}
	return s.GetConnection(ctx, conn.ServerID, conn.UserID)
}

func (s *Store) GetConnection(ctx context.Context, serverID uint64, userID int64) (*domain.MCPConnection, error) {
	var row MCPConnectionRow
	err := s.db.WithContext(ctx).Where("server_id = ? AND user_id = ?", serverID, userID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrMCPConnectionNotFound
	}
	if err != nil {
		return nil, transient(err)
	}
	conn := row.toDomain()
	return &conn, nil
}

func (s *Store) ListConnections(ctx context.Context, userID int64) ([]domain.MCPConnection, error) {
	var rows []MCPConnectionRow
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("id asc").Find(&rows).Error; err != nil {
		return nil, transient(err)
	}
	out := make([]domain.MCPConnection, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

func (s *Store) TouchConnection(ctx context.Context, id uint64) error {
	now := time.Now()
	return transient(s.db.WithContext(ctx).Model(&MCPConnectionRow{}).Where("id = ?", id).Updates(map[string]interface{}{
		"last_used_at": &now, "updated_at": now,
	}).Error)
}

func (s *Store) DeleteConnection(ctx context.Context, serverID uint64, userID int64) error {
	result := s.db.WithContext(ctx).Where("server_id = ? AND user_id = ?", serverID, userID).Delete(&MCPConnectionRow{})
	if result.Error != nil {
		return transient(result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrMCPConnectionNotFound
	}
	return nil
}

func (s *Store) CreateSkill(ctx context.Context, ownerID int64, slug, displayName, description, visibility, reviewStatus string) (*domain.Skill, error) {
	row := SkillRow{
		OwnerUserID: ownerID, Slug: slug, DisplayName: displayName, Description: description,
		Visibility: visibility, ReviewStatus: reviewStatus, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if row.DisplayName == "" {
		row.DisplayName = slug
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, domain.ErrSkillConflict
		}
		return nil, transient(err)
	}
	skill := row.toDomain()
	return &skill, nil
}

func (s *Store) GetSkillByID(ctx context.Context, id uint64) (*domain.Skill, error) {
	var row SkillRow
	err := s.db.WithContext(ctx).First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrSkillNotFound
	}
	if err != nil {
		return nil, transient(err)
	}
	skill := row.toDomain()
	return &skill, nil
}

func (s *Store) GetSkillByOwnerSlug(ctx context.Context, ownerID int64, slug string) (*domain.Skill, error) {
	var row SkillRow
	err := s.db.WithContext(ctx).Where("owner_user_id = ? AND slug = ?", ownerID, slug).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrSkillNotFound
	}
	if err != nil {
		return nil, transient(err)
	}
	skill := row.toDomain()
	return &skill, nil
}

func (s *Store) ListSkillsForUser(ctx context.Context, userID int64, includeAll bool) ([]domain.Skill, error) {
	var rows []SkillRow
	q := s.db.WithContext(ctx).Order("id asc")
	if !includeAll {
		q = q.Where("owner_user_id = ? OR (visibility IN ? AND review_status = ?)",
			userID, []string{"shared", "public"}, "approved")
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, transient(err)
	}
	out := make([]domain.Skill, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

func (s *Store) UpdateSkillMeta(ctx context.Context, id uint64, displayName, description, visibility, reviewStatus string) error {
	result := s.db.WithContext(ctx).Model(&SkillRow{}).Where("id = ?", id).Updates(map[string]interface{}{
		"display_name": displayName, "description": description,
		"visibility": visibility, "review_status": reviewStatus, "updated_at": time.Now(),
	})
	if result.Error != nil {
		return transient(result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrSkillNotFound
	}
	return nil
}

func (s *Store) SetSkillReview(ctx context.Context, id uint64, status string) error {
	result := s.db.WithContext(ctx).Model(&SkillRow{}).Where("id = ?", id).
		Updates(map[string]interface{}{"review_status": status, "updated_at": time.Now()})
	if result.Error != nil {
		return transient(result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrSkillNotFound
	}
	return nil
}

func (s *Store) DeleteSkill(ctx context.Context, id uint64) error {
	result := s.db.WithContext(ctx).Delete(&SkillRow{}, id)
	if result.Error != nil {
		return transient(result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrSkillNotFound
	}
	return nil
}

// CreateSkillVersion inserts an immutable version and advances latest_version
// in one transaction.
func (s *Store) CreateSkillVersion(ctx context.Context, version domain.SkillVersion) (*domain.SkillVersion, error) {
	var result domain.SkillVersion
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var skill SkillRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&skill, version.SkillID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrSkillNotFound
			}
			return err
		}
		if version.Version <= skill.LatestVersion {
			return fmt.Errorf("%w: version must be greater than %d", domain.ErrInvalidSkill, skill.LatestVersion)
		}
		row := SkillVersionRow{
			SkillID: version.SkillID, Version: version.Version, Content: version.Content,
			FrontMatter: version.FrontMatter, Checksum: version.Checksum, Status: version.Status,
			ReviewStatus: version.ReviewStatus, ReviewReason: version.ReviewReason,
			CreatedBy: version.CreatedBy, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if err := tx.Model(&SkillRow{}).Where("id = ?", skill.ID).Updates(map[string]interface{}{
			"latest_version": row.Version, "updated_at": time.Now(),
		}).Error; err != nil {
			return err
		}
		result = row.toDomain()
		return nil
	})
	if err != nil {
		return nil, transient(err)
	}
	return &result, nil
}

func (s *Store) GetSkillVersion(ctx context.Context, skillID uint64, version int64) (*domain.SkillVersion, error) {
	var row SkillVersionRow
	err := s.db.WithContext(ctx).Where("skill_id = ? AND version = ?", skillID, version).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrSkillVersionNotFound
	}
	if err != nil {
		return nil, transient(err)
	}
	out := row.toDomain()
	return &out, nil
}

func (s *Store) ListSkillVersions(ctx context.Context, skillID uint64) ([]domain.SkillVersion, error) {
	var rows []SkillVersionRow
	if err := s.db.WithContext(ctx).Where("skill_id = ?", skillID).Order("version desc").Find(&rows).Error; err != nil {
		return nil, transient(err)
	}
	out := make([]domain.SkillVersion, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

func (s *Store) SetSkillVersionReview(ctx context.Context, skillID uint64, version int64, status, reason string, reviewer int64) error {
	now := time.Now()
	result := s.db.WithContext(ctx).Model(&SkillVersionRow{}).
		Where("skill_id = ? AND version = ?", skillID, version).
		Updates(map[string]interface{}{
			"review_status": status, "review_reason": reason, "review_by": reviewer,
			"review_at": &now, "updated_at": now,
		})
	if result.Error != nil {
		return transient(result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrSkillVersionNotFound
	}
	return nil
}

func transient(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", domain.ErrTransient, err)
}
