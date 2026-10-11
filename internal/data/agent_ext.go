package data

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

var ErrMCPNotFound = errors.New("mcp server not found")

// MCPServer is a platform-administered trusted MCP server registry entry.
type MCPServer struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	Name         string `gorm:"uniqueIndex;size:100;not null"`
	Transport    string `gorm:"size:16;not null;default:http"`
	URL          string `gorm:"size:500;not null"`
	AllowedTools string `gorm:"type:text;not null"` // JSON array of tool names
	RiskClass    string `gorm:"size:24;not null;default:read_only"`
	Enabled      bool   `gorm:"not null;default:true"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// MCPBinding is one bound (server, tool) pair on an agent draft or version
// snapshot. Tool is the MCP tool name; Exposed is the runtime tool name
// "mcp_<server>_<tool>".
type MCPBinding struct {
	Server  string `json:"server"`
	Tool    string `json:"tool"`
	Exposed string `json:"exposed"`
}

// SkillBinding binds one published skill version to an agent draft or
// version snapshot.
type SkillBinding struct {
	Name    string `json:"name"`
	Version int64  `json:"version"`
}

// Skill is a platform-published, reviewed SKILL.md version.
type Skill struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	Name      string `gorm:"index:idx_skill_name_version,unique;priority:1;size:100;not null"`
	Version   int64  `gorm:"index:idx_skill_name_version,unique;priority:2;not null"`
	Content   string `gorm:"type:text;not null"`
	Published bool   `gorm:"not null;default:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type MCPStore struct{ db *gorm.DB }

func NewMCPStore(d *Data) *MCPStore { return &MCPStore{db: d.db} }

func (s *MCPStore) RegisterServer(ctx context.Context, name, transport, url, tools, risk string) (*MCPServer, error) {
	row := MCPServer{Name: name, Transport: transport, URL: url, AllowedTools: tools, RiskClass: risk, Enabled: true}
	err := s.db.WithContext(ctx).Create(&row).Error
	return &row, err
}

func (s *MCPStore) GetServer(ctx context.Context, name string) (*MCPServer, error) {
	var row MCPServer
	err := s.db.WithContext(ctx).Where("name = ?", name).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMCPNotFound
	}
	return &row, err
}

func (s *MCPStore) GetServerByID(ctx context.Context, id uint64) (*MCPServer, error) {
	var row MCPServer
	err := s.db.WithContext(ctx).First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMCPNotFound
	}
	return &row, err
}

func (s *MCPStore) ListServers(ctx context.Context) ([]MCPServer, error) {
	var rows []MCPServer
	err := s.db.WithContext(ctx).Order("id asc").Find(&rows).Error
	return rows, err
}

func (s *MCPStore) SetServerEnabled(ctx context.Context, id uint64, enabled bool) error {
	return s.db.WithContext(ctx).Model(&MCPServer{}).Where("id = ?", id).Update("enabled", enabled).Error
}

func (s *MCPStore) PublishSkill(ctx context.Context, name string, version int64, content string) (*Skill, error) {
	row := Skill{Name: name, Version: version, Content: content, Published: true}
	err := s.db.WithContext(ctx).Create(&row).Error
	return &row, err
}

func (s *MCPStore) GetSkill(ctx context.Context, name string, version int64) (*Skill, error) {
	var row Skill
	err := s.db.WithContext(ctx).Where("name = ? AND version = ?", name, version).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, gorm.ErrRecordNotFound
	}
	return &row, err
}

func (s *MCPStore) ListSkills(ctx context.Context, publishedOnly bool) ([]Skill, error) {
	var rows []Skill
	q := s.db.WithContext(ctx).Order("name asc, version desc")
	if publishedOnly {
		q = q.Where("published = ?", true)
	}
	err := q.Find(&rows).Error
	return rows, err
}
