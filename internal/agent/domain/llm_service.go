package domain

import "time"

// ModelService is a platform-managed model access point. A Bifrost-backed
// service maps to one Bifrost virtual key: the gateway owns provider keys and
// routing, while this row keeps only the inference endpoint and the sealed
// virtual key value needed by the agent runtime.
type ModelService struct {
	ID           uint64
	Name         string
	DisplayName  string
	GatewayType  string // "bifrost" or "openai_compatible"
	BaseURL      string
	GatewayKeyID string
	APIKeyEnc    string
	Enabled      bool
	CreatedBy    int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Model is one selectable model under a ModelService.
type Model struct {
	ID                 uint64
	ServiceID          uint64
	ModelKey           string
	DisplayName        string
	ContextWindow      int
	MaxOutputTokens    int
	SupportsTools      bool
	SupportsVision     bool
	SupportsReasoning  bool
	InputPricePerMTok  float64
	OutputPricePerMTok float64
	Enabled            bool
	LastSyncedAt       *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// ModelServiceSpec is the editable configuration of a model service.
type ModelServiceSpec struct {
	Name         string
	DisplayName  string
	GatewayType  string
	BaseURL      string
	GatewayKeyID string
	APIKey       string
	UpdateAPIKey bool
	Enabled      *bool
	// ProviderModels selects the Bifrost provider/model allowlist when a
	// Bifrost virtual key is created. When empty, allow_all_providers is used.
	ProviderModels map[string][]string
	AllowAll       bool
}

// ModelSpec is one manually configured model entry (openai_compatible mode or
// a Bifrost model that is not returned by the gateway catalog).
type ModelSpec struct {
	ServiceID          uint64
	ModelKey           string
	DisplayName        string
	ContextWindow      int
	MaxOutputTokens    int
	SupportsTools      bool
	SupportsVision     bool
	SupportsReasoning  bool
	InputPricePerMTok  float64
	OutputPricePerMTok float64
	Enabled            bool
}
