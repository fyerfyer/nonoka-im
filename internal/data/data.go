package data

import (
	"time"

	"nonoka-im/internal/conf"

	"github.com/google/wire"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(NewData, NewAuthRepo)

// Data .
type Data struct {
	db *gorm.DB
}

// CleanTestData truncates all user tables for integration test isolation.
// This should ONLY be called in test environments.
func (d *Data) CleanTestData() error {
	return d.db.Exec("TRUNCATE TABLE users RESTART IDENTITY CASCADE").Error
}

// NewData .
func NewData(c *conf.Data) (*Data, func(), error) {
	dsn := c.Database.Source
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
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
	if err := db.AutoMigrate(&User{}); err != nil {
		return nil, nil, err
	}

	d := &Data{
		db: db,
	}

	cleanup := func() {
		if sqlDB != nil {
			sqlDB.Close()
		}
	}

	return d, cleanup, nil
}