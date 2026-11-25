package data

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"nonoka-im/internal/conf"
	imdata "nonoka-im/internal/data/im"

	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// ProviderSet is the root data provider. Repository constructors live in the
// data/im, data/agent and data/platform subpackages.
var ProviderSet = wire.NewSet(NewData)

// Data holds the shared PostgreSQL handle and the Redis client.
type Data struct {
	db *gorm.DB
	// Redis client for distributed session and caching.
	Redis redis.UniversalClient
}

// DB exposes the shared PostgreSQL handle for subpackage repositories.
func (d *Data) DB() *gorm.DB { return d.db }

// ListBotUserIDs returns the IDs of all bot users (agent accounts).
func (d *Data) ListBotUserIDs(ctx context.Context) ([]int64, error) {
	var ids []int64
	err := d.db.WithContext(ctx).Model(&imdata.User{}).Where("is_bot = ?", true).Pluck("id", &ids).Error
	return ids, err
}

// IsBotUser reports whether the given user is a bot account. Users that do
// not exist count as non-bots.
func (d *Data) IsBotUser(ctx context.Context, id int64) (bool, error) {
	var n int64
	err := d.db.WithContext(ctx).Model(&imdata.User{}).Where("id = ? AND is_bot = ?", id, true).Count(&n).Error
	return n > 0, err
}

// CleanTestData truncates all domain tables for integration test isolation.
// This should ONLY be called in test environments.
func (d *Data) CleanTestData() error {
	return d.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL nonoka.audit_purge = 'on'").Error; err != nil {
			return err
		}
		return tx.Exec(`TRUNCATE TABLE users, group_members, conversations, groups,
		agent_runs, agent_turns, agent_sessions, agent_context_snapshots,
		agent_versions, agents, mcp_servers, mcp_tools, mcp_connections, skills, skill_versions, agent_checkpoints,
		agent_approvals, agent_tool_invocations, idempotency_keys,
		model_catalog, agent_gateway_keys, platform_gateway_keys, gateway_key_cleanup,
		agent_quotas, agent_usage_daily, agent_usage_events, agent_quota_notices,
		audit_events RESTART IDENTITY CASCADE`).Error
	})
}

// NewData opens PostgreSQL and Redis and applies versioned migrations. The
// schema is never inferred from GORM models at runtime.
func NewData(c *conf.Data) (*Data, func(), error) {
	if c == nil || c.Database == nil {
		return nil, nil, fmt.Errorf("database configuration is required")
	}
	dsn := c.Database.Source

	// Configure GORM logger: suppress record-not-found errors to reduce noise in tests
	gormLog := gormlogger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		gormlogger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  gormlogger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  true,
		},
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormLog})
	if err != nil {
		return nil, nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, err
	}

	maxOpenConns := 200
	maxIdleConns := 50
	connMaxLifetime := 30 * time.Minute
	if c.Database != nil {
		if c.Database.MaxOpenConns > 0 {
			maxOpenConns = int(c.Database.MaxOpenConns)
		}
		if c.Database.MaxIdleConns > 0 {
			maxIdleConns = int(c.Database.MaxIdleConns)
		}
		if c.Database.ConnMaxLifetime != nil {
			connMaxLifetime = c.Database.ConnMaxLifetime.AsDuration()
		}
	}
	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetConnMaxLifetime(connMaxLifetime)

	migrateCtx, migrateCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer migrateCancel()
	if err := Migrate(migrateCtx, sqlDB); err != nil {
		return nil, nil, err
	}

	// Initialize Redis client
	var redisClient redis.UniversalClient
	if c.Redis != nil && c.Redis.Addr != "" {
		redisClient = redis.NewClient(&redis.Options{Addr: c.Redis.Addr})
	}

	d := &Data{db: db, Redis: redisClient}

	cleanup := func() {
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		if redisClient != nil {
			_ = redisClient.Close()
		}
	}

	return d, cleanup, nil
}
