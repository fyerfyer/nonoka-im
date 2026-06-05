//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"nonoka-im/internal/biz"
	"nonoka-im/internal/conf"
	"nonoka-im/internal/data"
	"nonoka-im/internal/gateway"
	"nonoka-im/internal/msgworker"
	"nonoka-im/internal/server"
	"nonoka-im/internal/service"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// provideKafkaConfig returns Kafka configuration from environment or defaults.
func provideKafkaConfig() gateway.KafkaConfig {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		brokers = "127.0.0.1:9092"
	}
	topic := os.Getenv("KAFKA_TOPIC")
	if topic == "" {
		topic = "im-messages"
	}
	return gateway.KafkaConfig{
		Brokers:   []string{brokers},
		Topic:     topic,
		BatchSize: 100,
		Async:     true,
	}
}

// provideNodeID returns the unique node ID for this gateway instance.
func provideNodeID() string {
	nodeID, _ := os.Hostname()
	if nodeID == "" {
		nodeID = "gateway-0"
	}
	return nodeID
}

// provideJWTSecret extracts the JWT secret from auth config.
func provideJWTSecret(c *conf.Auth) []byte {
	return []byte(c.JwtSecret)
}

// provideHeartbeatConfig returns default heartbeat configuration.
func provideHeartbeatConfig() gateway.HeartbeatConfig {
	return gateway.HeartbeatConfig{
		Interval: 30 * time.Second,
		Timeout:  90 * time.Second,
	}
}

// provideRedisClient extracts the Redis client from Data.
func provideRedisClient(d *data.Data) redis.UniversalClient {
	return d.Redis
}

// provideMongoDB creates a MongoDB client and returns the database.
func provideMongoDB(c *conf.Data) (*mongo.Database, func(), error) {
	if c == nil || c.Mongodb == nil || c.Mongodb.Uri == "" {
		return nil, nil, fmt.Errorf("mongodb config is required")
	}
	client, err := mongo.Connect(options.Client().ApplyURI(c.Mongodb.Uri))
	if err != nil {
		return nil, nil, fmt.Errorf("connect mongodb: %w", err)
	}
	db := client.Database(c.Mongodb.Database)
	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Disconnect(ctx)
	}
	return db, cleanup, nil
}

// provideMessageStorage creates a MessageStorage from MongoDB database and ensures indexes.
func provideMessageStorage(db *mongo.Database, logger log.Logger) (*msgworker.MessageStorage, error) {
	storage := msgworker.NewMessageStorage(db, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := storage.EnsureIndexes(ctx); err != nil {
		return nil, fmt.Errorf("ensure mongodb indexes: %w", err)
	}
	return storage, nil
}

// wireApp init kratos application.
func wireApp(*conf.Server, *conf.Data, *conf.Auth, log.Logger) (*kratos.App, func(), error) {
	panic(wire.Build(
		server.ProviderSet,
		data.ProviderSet,
		biz.ProviderSet,
		service.ProviderSet,
		gateway.ProviderSet,
		provideNodeID,
		provideJWTSecret,
		provideHeartbeatConfig,
		provideRedisClient,
		provideKafkaConfig,
		provideMongoDB,
		provideMessageStorage,
		newApp,
	))
}
