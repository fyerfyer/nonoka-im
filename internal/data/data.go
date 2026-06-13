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

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, err
	}
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)

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
