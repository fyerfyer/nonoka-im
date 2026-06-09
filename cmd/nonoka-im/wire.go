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

// provideNodeIDString returns the unique node ID as a plain string.
// This is used by gateway.SessionManager which expects a string.
func provideNodeIDString() string {
	nodeID, _ := os.Hostname()
	if nodeID == "" {
		nodeID = "gateway-0"
	}
	return nodeID
}

// NodeID is a unique identifier for a gateway node, used to disambiguate
// wire providers that would otherwise have the same type.
type NodeID string

// provideNodeID returns the unique node ID as a typed NodeID.
func provideNodeID() NodeID {
	return NodeID(provideNodeIDString())
}

// GatewayURL is the WebSocket endpoint advertised to clients.
type GatewayURL string

// provideGatewayURL derives the WebSocket URL from environment or config.
func provideGatewayURL(dispatchConf *conf.Dispatch, nodeID NodeID) GatewayURL {
	id := string(nodeID)
	// Check static config first
	if dispatchConf != nil {
		for _, gw := range dispatchConf.Gateways {
			if gw.NodeId == id && gw.GatewayUrl != "" {
				return GatewayURL(gw.GatewayUrl)
			}
		}
		if len(dispatchConf.Gateways) > 0 && dispatchConf.Gateways[0].GatewayUrl != "" {
			return GatewayURL(dispatchConf.Gateways[0].GatewayUrl)
		}
	}
	// Fallback to env or default
	wsAddr := os.Getenv("GATEWAY_WS_ADDR")
	if wsAddr == "" {
		wsAddr = "ws://localhost:8000/ws"
	}
	return GatewayURL(wsAddr)
}

// GatewayRegistryConfig holds heartbeat configuration for the gateway registry.
type GatewayRegistryConfig struct {
	Interval time.Duration
	TTL      time.Duration
}

// provideGatewayRegistryConfig returns heartbeat interval and TTL from config or defaults.
func provideGatewayRegistryConfig(dispatchConf *conf.Dispatch) GatewayRegistryConfig {
	cfg := GatewayRegistryConfig{
		Interval: 10 * time.Second,
		TTL:      30 * time.Second,
	}
	if dispatchConf != nil {
		if dispatchConf.HeartbeatInterval != nil {
			cfg.Interval = dispatchConf.HeartbeatInterval.AsDuration()
		}
		if dispatchConf.NodeTtl != nil {
			cfg.TTL = dispatchConf.NodeTtl.AsDuration()
		}
	}
	return cfg
}

// provideJWTSecret extracts the JWT secret from auth config.
func provideJWTSecret(c *conf.Auth) []byte {
	return []byte(c.JwtSecret)
}

// provideHeartbeatConfig returns heartbeat configuration from config or defaults (#32).
func provideHeartbeatConfig(gatewayConf *conf.GatewayConfig) gateway.HeartbeatConfig {
	cfg := gateway.HeartbeatConfig{
		Interval: 30 * time.Second,
		Timeout:  90 * time.Second,
	}
	if gatewayConf != nil {
		if gatewayConf.HeartbeatInterval != nil {
			cfg.Interval = gatewayConf.HeartbeatInterval.AsDuration()
		}
		if gatewayConf.HeartbeatTimeout != nil {
			cfg.Timeout = gatewayConf.HeartbeatTimeout.AsDuration()
		}
		if gatewayConf.ReadTimeout != nil {
			// ReadTimeout is used by WebSocketServer, passed through Handler.
			cfg.ReadTimeout = gatewayConf.ReadTimeout.AsDuration()
		}
	}
	return cfg
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

// provideWebSocketServer creates a WebSocket server with configurable read timeout.
func provideWebSocketServer(handler *gateway.Handler, logger log.Logger, hb gateway.HeartbeatConfig) *gateway.WebSocketServer {
	return gateway.NewWebSocketServer(handler, logger, hb.ReadTimeout)
}

// provideGatewayRegistry creates a GatewayRegistry for node heartbeat registration.
func provideGatewayRegistry(redis redis.UniversalClient, nodeID NodeID, url GatewayURL, cfg GatewayRegistryConfig, manager *gateway.Manager, logger log.Logger) *gateway.GatewayRegistry {
	return gateway.NewGatewayRegistry(redis, string(nodeID), string(url), cfg.Interval, cfg.TTL, manager, logger)
}

// wireApp init kratos application.
func wireApp(*conf.Server, *conf.Data, *conf.Auth, *conf.Dispatch, *conf.GatewayConfig, log.Logger) (*kratos.App, func(), error) {
	panic(wire.Build(
		server.ProviderSet,
		data.ProviderSet,
		biz.ProviderSet,
		service.ProviderSet,
		gateway.ProviderSet,
		provideNodeIDString,
		provideNodeID,
		provideGatewayURL,
		provideGatewayRegistryConfig,
		provideGatewayRegistry,
		provideJWTSecret,
		provideHeartbeatConfig,
		provideWebSocketServer,
		provideRedisClient,
		provideKafkaConfig,
		provideMongoDB,
		provideMessageStorage,
		newApp,
	))
}
