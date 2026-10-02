package data

import (
	"context"
	"log"
	"os"
	"time"

	"nonoka-im/internal/conf"

	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(NewData, NewAuthRepo, NewGroupMemberRepo, NewConversationRepo, NewGroupRepo)

// Data .
type Data struct {
	db *gorm.DB
	// Redis client for distributed session and caching
	Redis redis.UniversalClient
}

// ListBotUserIDs returns the IDs of all bot users (agent accounts).
func (d *Data) ListBotUserIDs(ctx context.Context) ([]int64, error) {
	var ids []int64
	err := d.db.WithContext(ctx).Model(&User{}).Where("is_bot = ?", true).Pluck("id", &ids).Error
	return ids, err
}

// IsBotUser reports whether the given user is a bot account. Users that do
// not exist count as non-bots.
func (d *Data) IsBotUser(ctx context.Context, id int64) (bool, error) {
	var n int64
	err := d.db.WithContext(ctx).Model(&User{}).Where("id = ? AND is_bot = ?", id, true).Count(&n).Error
	return n > 0, err
}

// CleanTestData truncates all user tables for integration test isolation.
// This should ONLY be called in test environments.
func (d *Data) CleanTestData() error {
	return d.db.Exec("TRUNCATE TABLE users, group_members, conversations, agent_runs, agent_turns, agent_sessions, agent_context_snapshots, agent_versions, agents RESTART IDENTITY CASCADE").Error
}

// NewData .
func NewData(c *conf.Data) (*Data, func(), error) {
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

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLog,
	})
	if err != nil {
		return nil, nil, err
	}

	// Configure connection pool with sensible defaults tuned for IM workloads.
	// These can be overridden via config.data.database.* to handle higher
	// concurrency during login/register bursts.
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

	// Auto-migrate schema. Instances that start concurrently (multi-gateway
	// deployments) race on catalog changes, so serialize migrations with a
	// session-level advisory lock held for the duration of AutoMigrate.
	const migrateLockKey = 0x6e6f6e6f6b61 // "nonoka"
	lockConn, err := sqlDB.Conn(context.Background())
	if err != nil {
		return nil, nil, err
	}
	defer lockConn.Close()
	if _, err := lockConn.ExecContext(context.Background(), "SELECT pg_advisory_lock($1)", migrateLockKey); err != nil {
		return nil, nil, err
	}
	defer lockConn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", migrateLockKey) //nolint:errcheck

	if err := db.AutoMigrate(&User{}, &GroupMember{}, &Conversation{}, &Group{}, &AgentRun{}, &AgentTurn{}, &AgentSession{}, &AgentContextSnapshot{}, &Agent{}, &AgentVersion{}); err != nil {
		return nil, nil, err
	}
	if err := db.Exec("DROP INDEX IF EXISTS idx_agent_runs_event_id").Error; err != nil {
		return nil, nil, err
	}
	// Older turns predate session IDs. Backfill before new runs load history.
	if err := db.Exec(`INSERT INTO agent_sessions (agent_id, actor_user_id, topic, created_at, updated_at)
		SELECT DISTINCT agent_id, actor_user_id, topic, NOW(), NOW()
		FROM agent_turns WHERE session_id = 0
		ON CONFLICT (agent_id, actor_user_id, topic) DO NOTHING`).Error; err != nil {
		return nil, nil, err
	}
	if err := db.Exec(`UPDATE agent_turns AS t SET session_id = s.id
		FROM agent_sessions AS s WHERE t.session_id = 0
		AND t.agent_id = s.agent_id AND t.actor_user_id = s.actor_user_id AND t.topic = s.topic`).Error; err != nil {
		return nil, nil, err
	}

	// Initialize Redis client
	var redisClient redis.UniversalClient
	if c.Redis != nil && c.Redis.Addr != "" {
		redisClient = redis.NewClient(&redis.Options{
			Addr: c.Redis.Addr,
		})
	}

	d := &Data{
		db:    db,
		Redis: redisClient,
	}

	cleanup := func() {
		if sqlDB != nil {
			sqlDB.Close()
		}
		if redisClient != nil {
			redisClient.Close()
		}
	}

	return d, cleanup, nil
}
