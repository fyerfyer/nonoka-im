package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"nonoka-im/internal/agent/domain"
	platformdata "nonoka-im/internal/data/platform"
	"nonoka-im/internal/llm"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const migrationAdminDSN = "host=127.0.0.1 user=postgres password=root dbname=postgres port=5433 sslmode=disable TimeZone=Asia/Shanghai"

type migrationGateway struct {
	created []llm.VirtualKeySpec
}

func (g *migrationGateway) CreateVirtualKey(_ context.Context, spec llm.VirtualKeySpec) (*llm.VirtualKey, error) {
	g.created = append(g.created, spec)
	return &llm.VirtualKey{ID: fmt.Sprintf("vk-%d", len(g.created)), Value: "test-virtual-key"}, nil
}

func (g *migrationGateway) UpdateVirtualKey(context.Context, string, llm.VirtualKeySpec, bool) error {
	return nil
}

func (g *migrationGateway) DeleteVirtualKey(context.Context, string) error { return nil }

func (g *migrationGateway) EnsureCustomer(context.Context, llm.CustomerSpec) (string, error) {
	return "customer-1", nil
}

func (g *migrationGateway) ListGatewayModels(context.Context, string) ([]llm.GatewayModel, error) {
	return []llm.GatewayModel{{
		ID: "deepseek/deepseek-chat", NormalizedName: "DeepSeek Chat", OwnedBy: "deepseek",
		ContextLength: 65536, MaxOutputTokens: 8192,
		SupportedParameters: []string{"tools"},
		Architecture: struct {
			InputModalities  []string `json:"input_modalities"`
			OutputModalities []string `json:"output_modalities"`
		}{InputModalities: []string{"text", "image"}},
	}}, nil
}

type migrationSealer struct{}

func (migrationSealer) Seal(plain string) (string, error)  { return "enc:" + plain, nil }
func (migrationSealer) Open(sealed string) (string, error) { return sealed, nil }

type migrationAuthorizer struct{}

func (migrationAuthorizer) CanManagePlatform(context.Context, int64) error      { return nil }
func (migrationAuthorizer) SetPlatformAdmin(context.Context, int64, bool) error { return nil }

// TestAgentMigration_LegacyCatalogUpgrade verifies the 011 -> latest upgrade path:
// legacy rows without a provider prefix are repaired from the synced Bifrost
// catalog, nested gateway keys are preserved, and agent references follow the
// merged row.
func TestAgentMigration_LegacyCatalogUpgrade(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx := t.Context()

	admin, err := sql.Open("pgx", migrationAdminDSN)
	if err != nil {
		t.Fatalf("open admin db: %v", err)
	}
	defer admin.Close()
	name := fmt.Sprintf("nonoka_mig_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create scratch database: %v", err)
	}
	defer func() {
		_, _ = admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	}()

	dsn := "host=127.0.0.1 user=postgres password=root dbname=" + name + " port=5433 sslmode=disable TimeZone=Asia/Shanghai"
	raw, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open scratch db: %v", err)
	}
	defer raw.Close()

	goose.SetBaseFS(os.DirFS("../../internal/data"))
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	goose.SetLogger(goose.NopLogger())
	if err := goose.UpToContext(ctx, raw, "migrations", 11); err != nil {
		t.Fatalf("migrate to 011: %v", err)
	}

	// Legacy state: one model service/model without a provider prefix, and an
	// agent whose draft and published version reference it.
	seed := []string{
		`INSERT INTO model_services (name, base_url) VALUES ('legacy', 'http://legacy')`,
		`INSERT INTO model_models (service_id, model_key, display_name, enabled) VALUES (1, 'deepseek-chat', 'Legacy Chat', true)`,
		`INSERT INTO agents (bot_user_id, owner_user_id, name, status, current_version, draft_instruction, draft_model_id, draft_tools, draft_revision)
		 VALUES (1001, 1001, 'Legacy Agent', 'published', 1, 'be helpful', 1, '[]', 1)`,
		`INSERT INTO agent_versions (agent_id, version, instruction, tools, model_id, model_service_id, model_key)
		 VALUES (1, 1, 'be helpful', '[]', 1, 1, 'deepseek-chat')`,
	}
	for _, stmt := range seed {
		if _, err := raw.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	if err := goose.UpContext(ctx, raw, "migrations"); err != nil {
		t.Fatalf("migrate to latest: %v", err)
	}

	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	store := platformdata.NewStore(gormDB)
	gateway := &migrationGateway{}
	svc := llm.NewService(store, migrationSealer{}, migrationAuthorizer{}, gateway, llm.Config{})

	if _, err := svc.SyncCatalog(ctx, 1); err != nil {
		t.Fatalf("sync catalog: %v", err)
	}

	var got struct {
		ID              int64
		ModelKey        string
		Provider        string
		GatewayModelKey string
		SupportsVision  bool
	}
	if err := gormDB.Raw(`SELECT id, model_key, provider, gateway_model_key, supports_vision
		FROM model_catalog WHERE model_key = 'deepseek/deepseek-chat'`).Scan(&got).Error; err != nil {
		t.Fatalf("read repaired catalog row: %v", err)
	}
	if got.ID == 0 || got.Provider != "deepseek" || got.GatewayModelKey != "deepseek-chat" || !got.SupportsVision {
		t.Fatalf("repaired catalog row = %+v", got)
	}
	var draftID, versionModelID int64
	if err := gormDB.Raw(`SELECT draft_model_id FROM agents WHERE id = 1`).Scan(&draftID).Error; err != nil {
		t.Fatalf("read agent: %v", err)
	}
	if err := gormDB.Raw(`SELECT model_id FROM agent_versions WHERE agent_id = 1`).Scan(&versionModelID).Error; err != nil {
		t.Fatalf("read version: %v", err)
	}
	if draftID != got.ID || versionModelID != got.ID {
		t.Fatalf("agent references not merged: draft=%d version=%d want=%d", draftID, versionModelID, got.ID)
	}

	model, err := svc.Lookup(ctx, got.ID)
	if err != nil {
		t.Fatalf("lookup repaired model: %v", err)
	}
	if err := svc.EnsureAgentKey(ctx, 1, []domain.Model{*model}); err != nil {
		t.Fatalf("ensure agent key with repaired model: %v", err)
	}
	if len(gateway.created) == 0 || gateway.created[len(gateway.created)-1].ProviderModels["deepseek"] == nil {
		t.Fatalf("agent key did not receive the repaired provider/model: %+v", gateway.created)
	}

	if sqlDB, err := gormDB.DB(); err == nil {
		_ = sqlDB.Close()
	}
}
