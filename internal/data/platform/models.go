package platformdata

import (
	"time"

	"nonoka-im/internal/agent/domain"
)

type MCPServerRow struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Name         string    `gorm:"column:name;size:100;not null"`
	Transport    string    `gorm:"column:transport;size:16;not null;default:http"`
	URL          string    `gorm:"column:url;size:500;not null"`
	AllowedTools string    `gorm:"column:allowed_tools;type:text;not null"`
	RiskClass    string    `gorm:"column:risk_class;size:24;not null;default:read_only"`
	Enabled      bool      `gorm:"column:enabled;not null;default:true"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (MCPServerRow) TableName() string { return "mcp_servers" }

func (r MCPServerRow) toDomain() domain.MCPServer {
	return domain.MCPServer{
		ID: r.ID, Name: r.Name, Transport: r.Transport, URL: r.URL, AllowedTools: r.AllowedTools,
		RiskClass: r.RiskClass, Enabled: r.Enabled, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

type SkillRow struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Name      string    `gorm:"column:name;size:100;not null"`
	Version   int64     `gorm:"column:version;not null"`
	Content   string    `gorm:"column:content;type:text;not null"`
	Published bool      `gorm:"column:published;not null;default:false"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (SkillRow) TableName() string { return "skills" }

func (r SkillRow) toDomain() domain.Skill {
	return domain.Skill{
		ID: r.ID, Name: r.Name, Version: r.Version, Content: r.Content,
		Published: r.Published, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
