package platformdata

import (
	"time"

	"nonoka-im/internal/agent/domain"
)

// ModelServiceRow is the persistence shape of a platform model service. The
// sealed API key is a Bifrost virtual key (or, in compatibility mode, an
// OpenAI-compatible endpoint key); provider credentials never live here.
type ModelServiceRow struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Name         string    `gorm:"column:name;size:100;not null"`
	DisplayName  string    `gorm:"column:display_name;size:200;not null"`
	GatewayType  string    `gorm:"column:gateway_type;size:32;not null"`
	BaseURL      string    `gorm:"column:base_url;size:500;not null"`
	GatewayKeyID string    `gorm:"column:gateway_key_id;size:128;not null"`
	APIKeyEnc    string    `gorm:"column:api_key_enc;type:text"`
	Enabled      bool      `gorm:"column:enabled;not null;default:true"`
	CreatedBy    int64     `gorm:"column:created_by;not null"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (ModelServiceRow) TableName() string { return "model_services" }

func (r ModelServiceRow) toDomain() domain.ModelService {
	return domain.ModelService{
		ID: r.ID, Name: r.Name, DisplayName: r.DisplayName, GatewayType: r.GatewayType,
		BaseURL: r.BaseURL, GatewayKeyID: r.GatewayKeyID, APIKeyEnc: r.APIKeyEnc,
		Enabled: r.Enabled, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// ModelRow is one selectable model under a model service. Prices are USD per
// million tokens, sourced from the gateway catalog when available.
type ModelRow struct {
	ID                 uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	ServiceID          uint64     `gorm:"column:service_id;not null"`
	ModelKey           string     `gorm:"column:model_key;size:200;not null"`
	DisplayName        string     `gorm:"column:display_name;size:200;not null"`
	ContextWindow      int        `gorm:"column:context_window;not null"`
	MaxOutputTokens    int        `gorm:"column:max_output_tokens;not null"`
	SupportsTools      bool       `gorm:"column:supports_tools;not null"`
	SupportsVision     bool       `gorm:"column:supports_vision;not null"`
	SupportsReasoning  bool       `gorm:"column:supports_reasoning;not null"`
	InputPricePerMTok  float64    `gorm:"column:input_price_per_mtok;not null"`
	OutputPricePerMTok float64    `gorm:"column:output_price_per_mtok;not null"`
	Enabled            bool       `gorm:"column:enabled;not null"`
	LastSyncedAt       *time.Time `gorm:"column:last_synced_at"`
	CreatedAt          time.Time  `gorm:"column:created_at"`
	UpdatedAt          time.Time  `gorm:"column:updated_at"`
}

func (ModelRow) TableName() string { return "model_models" }

func (r ModelRow) toDomain() domain.Model {
	return domain.Model{
		ID: r.ID, ServiceID: r.ServiceID, ModelKey: r.ModelKey, DisplayName: r.DisplayName,
		ContextWindow: r.ContextWindow, MaxOutputTokens: r.MaxOutputTokens,
		SupportsTools: r.SupportsTools, SupportsVision: r.SupportsVision, SupportsReasoning: r.SupportsReasoning,
		InputPricePerMTok: r.InputPricePerMTok, OutputPricePerMTok: r.OutputPricePerMTok,
		Enabled: r.Enabled, LastSyncedAt: r.LastSyncedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func modelRowFromDomain(m domain.Model) ModelRow {
	return ModelRow{
		ID: m.ID, ServiceID: m.ServiceID, ModelKey: m.ModelKey, DisplayName: m.DisplayName,
		ContextWindow: m.ContextWindow, MaxOutputTokens: m.MaxOutputTokens,
		SupportsTools: m.SupportsTools, SupportsVision: m.SupportsVision, SupportsReasoning: m.SupportsReasoning,
		InputPricePerMTok: m.InputPricePerMTok, OutputPricePerMTok: m.OutputPricePerMTok,
		Enabled: m.Enabled, LastSyncedAt: m.LastSyncedAt, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}
