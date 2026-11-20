package platformdata

import (
	"context"
	"errors"
	"fmt"
	"time"

	"nonoka-im/internal/agent/domain"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// Store is the PostgreSQL repository for platform resources: the MCP server
// registry and published skills.
type Store struct{ db *gorm.DB }

func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

func (s *Store) RegisterServer(ctx context.Context, name, transport, url, tools, risk string) (*domain.MCPServer, error) {
	row := MCPServerRow{Name: name, Transport: transport, URL: url, AllowedTools: tools, RiskClass: risk, Enabled: true}
	err := s.db.WithContext(ctx).Create(&row).Error
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, domain.ErrMCPConflict
		}
		return nil, fmt.Errorf("%w: %w", domain.ErrTransient, err)
	}
	server := row.toDomain()
	return &server, nil
}

func (s *Store) GetServer(ctx context.Context, name string) (*domain.MCPServer, error) {
	var row MCPServerRow
	err := s.db.WithContext(ctx).Where("name = ?", name).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrMCPNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrTransient, err)
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
		return nil, fmt.Errorf("%w: %w", domain.ErrTransient, err)
	}
	server := row.toDomain()
	return &server, nil
}

func (s *Store) ListServers(ctx context.Context) ([]domain.MCPServer, error) {
	var rows []MCPServerRow
	err := s.db.WithContext(ctx).Order("id asc").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrTransient, err)
	}
	out := make([]domain.MCPServer, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

func (s *Store) SetServerEnabled(ctx context.Context, id uint64, enabled bool) error {
	return transient(s.db.WithContext(ctx).Model(&MCPServerRow{}).Where("id = ?", id).Update("enabled", enabled).Error)
}

func (s *Store) PublishSkill(ctx context.Context, name string, version int64, content string) (*domain.Skill, error) {
	row := SkillRow{Name: name, Version: version, Content: content, Published: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	err := s.db.WithContext(ctx).Create(&row).Error
	if err != nil {
		return nil, transient(err)
	}
	skill := row.toDomain()
	return &skill, nil
}

func (s *Store) GetSkill(ctx context.Context, name string, version int64) (*domain.Skill, error) {
	var row SkillRow
	err := s.db.WithContext(ctx).Where("name = ? AND version = ?", name, version).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrSkillNotFound
	}
	if err != nil {
		return nil, transient(err)
	}
	skill := row.toDomain()
	return &skill, nil
}

func (s *Store) ListSkills(ctx context.Context, publishedOnly bool) ([]domain.Skill, error) {
	var rows []SkillRow
	q := s.db.WithContext(ctx).Order("name asc, version desc")
	if publishedOnly {
		q = q.Where("published = ?", true)
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

func transient(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", domain.ErrTransient, err)
}
