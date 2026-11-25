package platformdata

import (
	"time"

	"nonoka-im/internal/agent/domain"
)

// ModelCatalogRow is the local, curated projection of the Bifrost model
// catalog. Credentials and routing never live here.
type ModelCatalogRow struct {
	ID                         uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	ModelKey                   string     `gorm:"column:model_key;size:200;not null"`
	Provider                   string     `gorm:"column:provider;size:64;not null;default:''"`
	GatewayModelKey            string     `gorm:"column:gateway_model_key;size:200;not null;default:''"`
	DisplayName                string     `gorm:"column:display_name;size:200;not null"`
	Icon                       string     `gorm:"column:icon;size:64;not null;default:''"`
	ContextWindow              int        `gorm:"column:context_window;not null"`
	MaxOutputTokens            int        `gorm:"column:max_output_tokens;not null"`
	SupportsTools              bool       `gorm:"column:supports_tools;not null"`
	SupportsVision             bool       `gorm:"column:supports_vision;not null"`
	SupportsReasoning          bool       `gorm:"column:supports_reasoning;not null"`
	InputPricePerMTok          float64    `gorm:"column:input_price_per_mtok;not null"`
	OutputPricePerMTok         float64    `gorm:"column:output_price_per_mtok;not null"`
	InputCacheReadPricePerMTok float64    `gorm:"column:input_cache_read_price_per_mtok;not null"`
	Enabled                    bool       `gorm:"column:enabled;not null"`
	Curated                    bool       `gorm:"column:curated;not null"`
	Recommended                bool       `gorm:"column:recommended;not null"`
	SortOrder                  int        `gorm:"column:sort_order;not null"`
	LastSyncedAt               *time.Time `gorm:"column:last_synced_at"`
	CreatedAt                  time.Time  `gorm:"column:created_at"`
	UpdatedAt                  time.Time  `gorm:"column:updated_at"`
}

func (ModelCatalogRow) TableName() string { return "model_catalog" }

func (r ModelCatalogRow) toDomain() domain.Model {
	return domain.Model{
		ID: r.ID, ModelKey: r.ModelKey, Provider: r.Provider, GatewayModelKey: r.GatewayModelKey,
		DisplayName: r.DisplayName, Icon: r.Icon, ContextWindow: r.ContextWindow,
		MaxOutputTokens: r.MaxOutputTokens, SupportsTools: r.SupportsTools,
		SupportsVision: r.SupportsVision, SupportsReasoning: r.SupportsReasoning,
		InputPricePerMTok: r.InputPricePerMTok, OutputPricePerMTok: r.OutputPricePerMTok,
		InputCacheReadPricePerMTok: r.InputCacheReadPricePerMTok,
		Enabled:                    r.Enabled, Curated: r.Curated, Recommended: r.Recommended, SortOrder: r.SortOrder,
		LastSyncedAt: r.LastSyncedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

type AgentGatewayKeyRow struct {
	AgentID      int64     `gorm:"column:agent_id;primaryKey"`
	VirtualKeyID string    `gorm:"column:virtual_key_id;size:128;not null"`
	ValueEnc     string    `gorm:"column:value_enc;type:text;not null"`
	Status       string    `gorm:"column:status;size:24;not null"`
	LastError    string    `gorm:"column:last_error;type:text;not null"`
	SpecHash     string    `gorm:"column:spec_hash;size:64;not null"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (AgentGatewayKeyRow) TableName() string { return "agent_gateway_keys" }

func (r AgentGatewayKeyRow) toDomain() domain.AgentGatewayKey {
	return domain.AgentGatewayKey{
		AgentID: r.AgentID, VirtualKeyID: r.VirtualKeyID, ValueEnc: r.ValueEnc,
		Status: r.Status, LastError: r.LastError, SpecHash: r.SpecHash,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

type PlatformGatewayKeyRow struct {
	Kind         string    `gorm:"column:kind;primaryKey;size:24"`
	VirtualKeyID string    `gorm:"column:virtual_key_id;size:128;not null"`
	ValueEnc     string    `gorm:"column:value_enc;type:text;not null"`
	Status       string    `gorm:"column:status;size:24;not null"`
	LastError    string    `gorm:"column:last_error;type:text;not null"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (PlatformGatewayKeyRow) TableName() string { return "platform_gateway_keys" }

func (r PlatformGatewayKeyRow) toDomain() domain.PlatformGatewayKey {
	return domain.PlatformGatewayKey{
		Kind: r.Kind, VirtualKeyID: r.VirtualKeyID, ValueEnc: r.ValueEnc,
		Status: r.Status, LastError: r.LastError, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

type GatewayKeyCleanupRow struct {
	VirtualKeyID  string    `gorm:"column:virtual_key_id;primaryKey;size:128"`
	AgentID       int64     `gorm:"column:agent_id;not null"`
	Reason        string    `gorm:"column:reason;size:64;not null"`
	Attempts      int       `gorm:"column:attempts;not null"`
	NextAttemptAt time.Time `gorm:"column:next_attempt_at;not null"`
	LastError     string    `gorm:"column:last_error;type:text;not null"`
	CreatedAt     time.Time `gorm:"column:created_at"`
}

func (GatewayKeyCleanupRow) TableName() string { return "gateway_key_cleanup" }

func (r GatewayKeyCleanupRow) toDomain() domain.GatewayKeyCleanup {
	return domain.GatewayKeyCleanup{
		VirtualKeyID: r.VirtualKeyID, AgentID: r.AgentID, Reason: r.Reason,
		Attempts: r.Attempts, NextAttemptAt: r.NextAttemptAt, LastError: r.LastError,
		CreatedAt: r.CreatedAt,
	}
}
