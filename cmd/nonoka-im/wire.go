//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

package main

import (
	"os"
	"time"

	"nonoka-im/internal/biz"
	"nonoka-im/internal/conf"
	"nonoka-im/internal/data"
	"nonoka-im/internal/gateway"
	"nonoka-im/internal/server"
	"nonoka-im/internal/service"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
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
		newApp,
	))
}
