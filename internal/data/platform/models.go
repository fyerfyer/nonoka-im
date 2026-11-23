package platformdata

import (
	"time"

	"nonoka-im/internal/agent/domain"
)

type MCPServerRow struct {
	ID            uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	OwnerUserID   int64      `gorm:"column:owner_user_id;not null"`
	Name          string     `gorm:"column:name;size:100;not null"`
	DisplayName   string     `gorm:"column:display_name;size:200;not null"`
	Description   string     `gorm:"column:description;type:text"`
	Transport     string     `gorm:"column:transport;size:32;not null"`
	URL           string     `gorm:"column:url;size:500;not null"`
	AuthType      string     `gorm:"column:auth_type;size:32;not null"`
	CredentialEnc string     `gorm:"column:credential_enc;type:text"`
	Visibility    string     `gorm:"column:visibility;size:16;not null"`
	ReviewStatus  string     `gorm:"column:review_status;size:16;not null"`
	ReviewReason  string     `gorm:"column:review_reason;type:text"`
	ReviewBy      int64      `gorm:"column:review_by"`
	ReviewAt      *time.Time `gorm:"column:review_at"`
	HealthStatus  string     `gorm:"column:health_status;size:24;not null"`
	LastCheckedAt *time.Time `gorm:"column:last_checked_at"`
	LastError     string     `gorm:"column:last_error;type:text"`
	Enabled       bool       `gorm:"column:enabled;not null"`
	ToolsRevision int64      `gorm:"column:tools_revision;not null"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at"`
}

func (MCPServerRow) TableName() string { return "mcp_servers" }

func (r MCPServerRow) toDomain() domain.MCPServer {
	return domain.MCPServer{
		ID: r.ID, OwnerUserID: r.OwnerUserID, Name: r.Name, DisplayName: r.DisplayName,
		Description: r.Description, Transport: r.Transport, URL: r.URL, AuthType: r.AuthType,
		CredentialEnc: r.CredentialEnc, Visibility: r.Visibility, ReviewStatus: r.ReviewStatus,
		ReviewReason: r.ReviewReason, ReviewBy: r.ReviewBy, ReviewAt: r.ReviewAt,
		HealthStatus: r.HealthStatus, LastCheckedAt: r.LastCheckedAt, LastError: r.LastError,
		Enabled: r.Enabled, ToolsRevision: r.ToolsRevision, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

type MCPToolRow struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	ServerID     uint64    `gorm:"column:server_id;not null"`
	Name         string    `gorm:"column:name;size:200;not null"`
	Description  string    `gorm:"column:description;type:text"`
	InputSchema  string    `gorm:"column:input_schema;type:jsonb"`
	OutputSchema string    `gorm:"column:output_schema;type:jsonb"`
	Annotations  string    `gorm:"column:annotations;type:jsonb"`
	Risk         string    `gorm:"column:risk;size:24;not null"`
	SchemaHash   string    `gorm:"column:schema_hash;size:64;not null"`
	Enabled      bool      `gorm:"column:enabled;not null"`
	DiscoveredAt time.Time `gorm:"column:discovered_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (MCPToolRow) TableName() string { return "mcp_tools" }

func (r MCPToolRow) toDomain() domain.MCPTool {
	return domain.MCPTool{
		ID: r.ID, ServerID: r.ServerID, Name: r.Name, Description: r.Description,
		InputSchema: r.InputSchema, OutputSchema: r.OutputSchema, Annotations: r.Annotations,
		Risk: r.Risk, SchemaHash: r.SchemaHash, Enabled: r.Enabled,
		DiscoveredAt: r.DiscoveredAt, UpdatedAt: r.UpdatedAt,
	}
}

type MCPConnectionRow struct {
	ID            uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	ServerID      uint64     `gorm:"column:server_id;not null"`
	UserID        int64      `gorm:"column:user_id;not null"`
	AuthType      string     `gorm:"column:auth_type;size:32;not null"`
	CredentialEnc string     `gorm:"column:credential_enc;type:text;not null"`
	Status        string     `gorm:"column:status;size:24;not null"`
	ExpiresAt     *time.Time `gorm:"column:expires_at"`
	LastUsedAt    *time.Time `gorm:"column:last_used_at"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at"`
}

func (MCPConnectionRow) TableName() string { return "mcp_connections" }

func (r MCPConnectionRow) toDomain() domain.MCPConnection {
	return domain.MCPConnection{
		ID: r.ID, ServerID: r.ServerID, UserID: r.UserID, AuthType: r.AuthType,
		CredentialEnc: r.CredentialEnc, Status: r.Status, ExpiresAt: r.ExpiresAt,
		LastUsedAt: r.LastUsedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

type SkillRow struct {
	ID            uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	OwnerUserID   int64     `gorm:"column:owner_user_id;not null"`
	Slug          string    `gorm:"column:slug;size:100;not null"`
	DisplayName   string    `gorm:"column:display_name;size:200;not null"`
	Description   string    `gorm:"column:description;type:text"`
	Visibility    string    `gorm:"column:visibility;size:16;not null"`
	ReviewStatus  string    `gorm:"column:review_status;size:16;not null"`
	LatestVersion int64     `gorm:"column:latest_version;not null"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (SkillRow) TableName() string { return "skills" }

func (r SkillRow) toDomain() domain.Skill {
	return domain.Skill{
		ID: r.ID, OwnerUserID: r.OwnerUserID, Slug: r.Slug, DisplayName: r.DisplayName,
		Description: r.Description, Visibility: r.Visibility, ReviewStatus: r.ReviewStatus,
		LatestVersion: r.LatestVersion, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

type SkillVersionRow struct {
	ID           uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	SkillID      uint64     `gorm:"column:skill_id;not null"`
	Version      int64      `gorm:"column:version;not null"`
	Content      string     `gorm:"column:content;type:text;not null"`
	FrontMatter  string     `gorm:"column:front_matter;type:jsonb"`
	Checksum     string     `gorm:"column:checksum;size:64;not null"`
	Status       string     `gorm:"column:status;size:24;not null"`
	ReviewStatus string     `gorm:"column:review_status;size:16;not null"`
	ReviewReason string     `gorm:"column:review_reason;type:text"`
	ReviewBy     int64      `gorm:"column:review_by"`
	ReviewAt     *time.Time `gorm:"column:review_at"`
	CreatedBy    int64      `gorm:"column:created_by;not null"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at"`
}

func (SkillVersionRow) TableName() string { return "skill_versions" }

func (r SkillVersionRow) toDomain() domain.SkillVersion {
	return domain.SkillVersion{
		ID: r.ID, SkillID: r.SkillID, Version: r.Version, Content: r.Content,
		FrontMatter: r.FrontMatter, Checksum: r.Checksum, Status: r.Status,
		ReviewStatus: r.ReviewStatus, ReviewReason: r.ReviewReason, ReviewBy: r.ReviewBy,
		ReviewAt: r.ReviewAt, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
