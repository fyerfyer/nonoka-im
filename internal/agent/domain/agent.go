// Package domain holds the Agent platform business types shared by the
// application layer and its persistence adapters. It must not import data,
// eino, gorm or any transport package.
package domain

import (
	"errors"
	"time"
)

var (
	ErrDraftConflict    = errors.New("agent draft changed")
	ErrRunState         = errors.New("run state conflict")
	ErrAgentNotFound    = errors.New("agent not found")
	ErrVersionNotFound  = errors.New("agent version not found")
	ErrRunNotFound      = errors.New("agent run not found")
	ErrSessionNotFound  = errors.New("agent session not found")
	ErrApprovalNotFound = errors.New("agent approval not found")
	ErrMCPNotFound      = errors.New("mcp server not found")
	ErrMCPConflict      = errors.New("mcp server already exists")
	ErrSkillNotFound    = errors.New("skill version not found")
	ErrApprovalGone     = errors.New("approval is no longer pending")
	ErrApprovalExpired  = errors.New("approval expired")
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
	ID               int64
	BotUserID        int64
	OwnerUserID      int64
	Name             string
	Description      string
	Status           string
	CurrentVersion   int64
	DraftInstruction string
	DraftModel       string
	DraftTools       string
	DraftBaseURL     string
	DraftAPIKeyEnc   string
	DraftMaxTokens   int
	DraftMCPBindings string
	DraftSkills      string
	DraftRevision    int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// AgentVersion is an immutable published snapshot of an agent configuration.
type AgentVersion struct {
	ID          uint64
	AgentID     int64
	Version     int64
	Instruction string
	Model       string
	Tools       string
	BaseURL     string
	APIKeyEnc   string
	MaxTokens   int
	MCPBindings string
	Skills      string
	CreatedAt   time.Time
}

// AgentSpec is the editable draft configuration of an agent. APIKeyEnc is the
// sealed model credential; it is frozen into every published version.
type AgentSpec struct {
	Name, Description, Instruction string
	Model, Tools                   string
	BaseURL, APIKeyEnc             string
	MaxTokens                      int
	MCPBindings, Skills            string
}

// MCPBinding is one bound (server, tool) pair on an agent draft or version.
// The json tags define the persisted version-snapshot format, not an HTTP DTO.
type MCPBinding struct {
	Server  string `json:"server"`
	Tool    string `json:"tool"`
	Exposed string `json:"exposed"`
}

// SkillBinding binds one published skill version to an agent draft or version.
// The json tags define the persisted version-snapshot format, not an HTTP DTO.
type SkillBinding struct {
	Name    string `json:"name"`
	Version int64  `json:"version"`
}
