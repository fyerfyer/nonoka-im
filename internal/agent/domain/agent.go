// Package domain holds the Agent platform business types shared by the
// application layer and its persistence adapters. It must not import data,
// eino, gorm or any transport package.
package domain

import (
	"errors"
	"time"
)

var (
	ErrDraftConflict         = errors.New("agent draft changed")
	ErrRunState              = errors.New("run state conflict")
	ErrAgentNotFound         = errors.New("agent not found")
	ErrVersionNotFound       = errors.New("agent version not found")
	ErrRunNotFound           = errors.New("agent run not found")
	ErrSessionNotFound       = errors.New("agent session not found")
	ErrApprovalNotFound      = errors.New("agent approval not found")
	ErrMCPNotFound           = errors.New("mcp server not found")
	ErrMCPConflict           = errors.New("mcp server already exists")
	ErrMCPToolNotFound       = errors.New("mcp tool not found")
	ErrMCPConnectionNotFound = errors.New("mcp connection not found")
	ErrMCPReviewRequired     = errors.New("mcp server is not approved")
	ErrMCPAuthRequired       = errors.New("mcp server requires a user connection")
	ErrInvalidMCPTarget      = errors.New("invalid or blocked mcp target")
	ErrSkillNotFound         = errors.New("skill not found")
	ErrSkillVersionNotFound  = errors.New("skill version not found")
	ErrSkillConflict         = errors.New("skill already exists")
	ErrInvalidSkill          = errors.New("invalid skill")
	ErrModelServiceNotFound  = errors.New("model service not found")
	ErrModelNotFound         = errors.New("model not found")
	ErrModelServiceConflict  = errors.New("model service already exists")
	ErrModelServiceInUse     = errors.New("model service is referenced by a published agent")
	ErrApprovalGone          = errors.New("approval is no longer pending")
	ErrApprovalExpired       = errors.New("approval expired")
	// ErrTransient marks an infrastructure failure that may succeed on retry.
	// Persistence adapters wrap DB/network errors with it so the run executor
	// does not mistake a temporary outage for a permanent configuration error.
	ErrTransient = errors.New("transient infrastructure error")
	// ErrSideEffectCommitted marks a run that already executed a write tool.
	// Such a run must not be retried automatically because the external side
	// effect cannot be undone safely.
	ErrSideEffectCommitted = errors.New("run has committed a side effect")
)

// Agent is the identity and lifecycle record of an agent. Draft* fields are
// the editable configuration; a published AgentVersion is the immutable
// runtime snapshot.
type Agent struct {
	ID                     int64
	BotUserID              int64
	OwnerUserID            int64
	Name                   string
	Description            string
	Status                 string
	CurrentVersion         int64
	DraftInstruction       string
	DraftTools             string
	DraftModelServiceID    int64
	DraftModelID           int64
	DraftCapabilityPresets string
	DraftToolPolicies      string
	DraftMaxTokens         int
	DraftMCPBindings       string
	DraftSkills            string
	DraftRevision          int64
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// AgentVersion is an immutable published snapshot of an agent configuration.
type AgentVersion struct {
	ID                uint64
	AgentID           int64
	Version           int64
	Instruction       string
	ModelServiceID    int64
	ModelID           int64
	ModelKey          string
	Tools             string
	CapabilityPresets string
	ToolPolicies      string
	MaxTokens         int
	MCPBindings       string
	Skills            string
	CreatedAt         time.Time
}

// AgentSpec is the editable draft configuration of an agent. It references a
// platform ModelService/Model and freezes those references on publish.
type AgentSpec struct {
	Name, Description, Instruction  string
	ModelServiceID, ModelID         int64
	Tools                           string
	CapabilityPresets, ToolPolicies string
	MaxTokens                       int
	MCPBindings, Skills             string
}

// MCPBinding is one bound (server, tool) pair on an agent draft or version.
// SchemaHash freezes the discovered schema so a changed server tool cannot
// silently change the model-facing contract. The json tags define the
// persisted version-snapshot format, not an HTTP DTO.
type MCPBinding struct {
	ServerID   uint64 `json:"serverId"`
	Server     string `json:"server,omitempty"`
	Tool       string `json:"tool"`
	Exposed    string `json:"exposed"`
	SchemaHash string `json:"schemaHash,omitempty"`
	Policy     string `json:"policy,omitempty"`
}

// SkillBinding binds one immutable skill version to an agent draft or version.
// The checksum freezes the content that was validated at publish time.
type SkillBinding struct {
	SkillID  uint64 `json:"skillId"`
	Name     string `json:"name,omitempty"`
	Version  int64  `json:"version"`
	Checksum string `json:"checksum,omitempty"`
}
