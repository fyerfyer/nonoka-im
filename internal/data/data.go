package data

import (
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
var ProviderSet = wire.NewSet(NewData, NewAuthRepo, NewGroupMemberRepo)

// Data .
type Data struct {
	db  *gorm.DB
	// Redis client for distributed session and caching
	Redis redis.UniversalClient
}

// CleanTestData truncates all user tables for integration test isolation.
// This should ONLY be called in test environments.
func (d *Data) CleanTestData() error {
	return d.db.Exec("TRUNCATE TABLE users, group_members RESTART IDENTITY CASCADE").Error
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

	// Auto-migrate schema
	if err := db.AutoMigrate(&User{}, &GroupMember{}); err != nil {
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
