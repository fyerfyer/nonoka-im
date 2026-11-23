package domain

import (
	"fmt"
	"time"
)

const (
	VisibilityPrivate = "private"
	VisibilityShared  = "shared"
	VisibilityPublic  = "public"

	ReviewPending  = "pending"
	ReviewApproved = "approved"
	ReviewRejected = "rejected"

	MCPHealthUnknown   = "unknown"
	MCPHealthHealthy   = "healthy"
	MCPHealthNeedsAuth = "needs_auth"
	MCPHealthError     = "error"

	MCPTransportHTTP = "streamable_http"

	MCPAuthNone   = "none"
	MCPAuthBearer = "bearer"
	MCPAuthHeader = "header"

	MCPRiskReadOnly = "read_only"
	MCPRiskWrite    = "write"

	SkillVersionPublished  = "published"
	SkillVersionDeprecated = "deprecated"
)

// MCPServer is a user- or platform-owned remote MCP server. Provider
// credentials stay encrypted and are never returned by the API.
type MCPServer struct {
	ID            uint64
	OwnerUserID   int64
	Name          string
	DisplayName   string
	Description   string
	Transport     string // streamable_http
	URL           string
	AuthType      string // none | bearer | header
	CredentialEnc string
	Visibility    string // private | shared | public
	ReviewStatus  string // pending | approved | rejected
	ReviewReason  string
	ReviewBy      int64
	ReviewAt      *time.Time
	HealthStatus  string // unknown | healthy | needs_auth | error
	LastCheckedAt *time.Time
	LastError     string
	Enabled       bool
	ToolsRevision int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// MCPTool is one discovered tool under an MCP server.
type MCPTool struct {
	ID           uint64
	ServerID     uint64
	Name         string
	Description  string
	InputSchema  string
	OutputSchema string
	Annotations  string
	Risk         string // read_only | write
	SchemaHash   string
	Enabled      bool
	DiscoveredAt time.Time
	UpdatedAt    time.Time
}

// MCPExposedName is the model-facing name of one MCP tool. The server ID makes
// it stable across owners with identical server names.
func MCPExposedName(serverID uint64, tool string) string {
	return fmt.Sprintf("mcp_%d_%s", serverID, tool)
}

// MCPTarget is one concrete MCP connection target (server-level or per-user).
type MCPTarget struct {
	URL        string
	Transport  string
	AuthType   string
	Credential string
}

// MCPDiscoveryResult is one tools/list snapshot plus the health outcome.
type MCPDiscoveryResult struct {
	Tools     []MCPTool
	NeedsAuth bool
}

// MCPConnection is one user's credential for one MCP server. It is deliberately
// separate from the server-level credential so shared servers never reuse the
// owner's identity for another actor.
type MCPConnection struct {
	ID            uint64
	ServerID      uint64
	UserID        int64
	AuthType      string
	CredentialEnc string
	Status        string // connected | needs_auth | error
	ExpiresAt     *time.Time
	LastUsedAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Skill is the stable, versioned skill identity. Versions are immutable.
type Skill struct {
	ID            uint64
	OwnerUserID   int64
	Slug          string
	DisplayName   string
	Description   string
	Visibility    string // private | shared | public
	ReviewStatus  string // pending | approved | rejected
	LatestVersion int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// SkillVersion is one immutable SKILL.md snapshot.
type SkillVersion struct {
	ID           uint64
	SkillID      uint64
	Version      int64
	Content      string
	FrontMatter  string
	Checksum     string
	Status       string // published | deprecated
	ReviewStatus string // pending | approved | rejected
	ReviewReason string
	ReviewBy     int64
	ReviewAt     *time.Time
	CreatedBy    int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
