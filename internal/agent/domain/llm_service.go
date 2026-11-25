package domain

import "time"

// Model is one curated entry in the platform model catalog. The catalog is a
// local, read-only projection of the Bifrost /v1/models response: provider
// credentials and routing stay in Bifrost, while display/capability/pricing
// curation and enablement live here.
//
// ModelKey is the full gateway model identifier used in inference requests
// (for example "deepseek/deepseek-flash"). GatewayModelKey is the provider
// local name used in a Bifrost virtual-key allowlist (for example
// "deepseek-flash").
type Model struct {
	ID                         uint64
	ModelKey                   string
	Provider                   string
	GatewayModelKey            string
	DisplayName                string
	Icon                       string
	ContextWindow              int
	MaxOutputTokens            int
	SupportsTools              bool
	SupportsVision             bool
	SupportsReasoning          bool
	InputPricePerMTok          float64
	OutputPricePerMTok         float64
	InputCacheReadPricePerMTok float64
	Enabled                    bool
	Curated                    bool
	Recommended                bool
	SortOrder                  int
	LastSyncedAt               *time.Time
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}

// ModelCuration is the editable, platform-owned part of a catalog entry.
// Provider routing, pricing and context limits come from Bifrost sync.
type ModelCuration struct {
	DisplayName       string
	Icon              string
	SupportsTools     *bool
	SupportsVision    *bool
	SupportsReasoning *bool
	Enabled           *bool
	Recommended       *bool
	SortOrder         *int
}

const (
	GatewayKeyActive         = "active"
	GatewayKeyDegraded       = "degraded"
	GatewayKeyRevokedPending = "revoked_pending"
	GatewayKeyRevoked        = "revoked"
)

// AgentGatewayKey is the per-agent Bifrost virtual key. The sealed value is
// internal-only and must never cross the API/SDK boundary.
type AgentGatewayKey struct {
	AgentID      int64
	VirtualKeyID string
	ValueEnc     string
	Status       string
	LastError    string
	SpecHash     string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// PlatformGatewayKey is a platform-owned key such as the shared playground
// sandbox key.
type PlatformGatewayKey struct {
	Kind         string
	VirtualKeyID string
	ValueEnc     string
	Status       string
	LastError    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// GatewayKeyCleanup is a failed virtual-key revocation that the worker retries.
type GatewayKeyCleanup struct {
	VirtualKeyID  string
	AgentID       int64
	Reason        string
	Attempts      int
	NextAttemptAt time.Time
	LastError     string
	CreatedAt     time.Time
}
