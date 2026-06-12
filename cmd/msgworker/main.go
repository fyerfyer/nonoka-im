package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nonoka-im/internal/conf"
	"nonoka-im/internal/msgworker"

	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var flagconf string

func init() {
	flag.StringVar(&flagconf, "conf", "../../configs", "config path, eg: -conf config.yaml")
}

func main() {
	flag.Parse()

	logger := log.With(log.NewStdLogger(os.Stdout),
		"ts", log.DefaultTimestamp,
		"caller", log.DefaultCaller,
		"service.id", mustHostname(),
		"service.name", "msgworker",
		"trace.id", tracing.TraceID(),
		"span.id", tracing.SpanID(),
	)

	c := config.New(
		config.WithSource(
			file.NewSource(flagconf),
		),
	)
	defer c.Close()

	if err := c.Load(); err != nil {
		panic(err)
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize Redis
	var redisClient redis.UniversalClient
	if bc.Data != nil && bc.Data.Redis != nil && bc.Data.Redis.Addr != "" {
		redisClient = redis.NewClient(&redis.Options{
			Addr: bc.Data.Redis.Addr,
		})
	} else {
		logger.Log(log.LevelFatal, "msg", "redis config is required")
		os.Exit(1)
	}

	// Initialize MongoDB
	if bc.Data == nil || bc.Data.Mongodb == nil || bc.Data.Mongodb.Uri == "" {
		logger.Log(log.LevelFatal, "msg", "mongodb config is required")
		os.Exit(1)
	}
	mongoClient, err := mongo.Connect(options.Client().ApplyURI(bc.Data.Mongodb.Uri))
	if err != nil {
		logger.Log(log.LevelFatal, "msg", fmt.Sprintf("connect mongodb: %v", err))
		os.Exit(1)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = mongoClient.Disconnect(ctx)
	}()

	mongoDB := mongoClient.Database(bc.Data.Mongodb.Database)

	// Initialize Kafka config
	kafkaCfg := msgworker.KafkaConsumerConfig{
		Brokers: bc.Data.Kafka.Brokers,
		Topic:   bc.Data.Kafka.Topic,
		GroupID: bc.Data.Kafka.ConsumerGroup,
	}
	if kafkaCfg.Topic == "" {
		kafkaCfg.Topic = "im-messages"
	}
	if kafkaCfg.GroupID == "" {
		kafkaCfg.GroupID = "msgworker-group"
	}

	// Gateway address priority: 1) GATEWAY_GRPC_ADDR env 2) server.grpc.addr from config 3) default
	gatewayAddr := os.Getenv("GATEWAY_GRPC_ADDR")
	if gatewayAddr == "" && bc.Server != nil && bc.Server.Grpc != nil && bc.Server.Grpc.Addr != "" {
		gatewayAddr = bc.Server.Grpc.Addr
	}
	if gatewayAddr == "" {
		gatewayAddr = "127.0.0.1:19000"
	}

	// Build dependencies
	seqGen := msgworker.NewSeqGenerator(redisClient, logger)
	snowflake := msgworker.NewSnowflake(1)
	storage := msgworker.NewMessageStorage(mongoDB, logger)
	if err := storage.EnsureIndexes(ctx); err != nil {
		logger.Log(log.LevelFatal, "msg", fmt.Sprintf("ensure indexes: %v", err))
		os.Exit(1)
	}

	pusher, err := msgworker.NewGatewayPusher([]string{gatewayAddr}, logger)
	if err != nil {
		logger.Log(log.LevelWarn, "msg", fmt.Sprintf("gateway pusher init failed: %v", err))
		// Non-fatal: worker can still persist messages
	}

	// Create consumer with handler wired to worker
	consumer := msgworker.NewKafkaConsumer(kafkaCfg, nil, logger)
	worker := msgworker.NewMsgWorker(
		consumer,
		seqGen,
		snowflake,
		storage,
		pusher,
		logger,
	)

	// Initialize group member service (Redis-backed)
	groupMemberSvc := msgworker.NewRedisGroupMemberService(redisClient, logger)
	worker.SetGroupMemberService(groupMemberSvc)

	// Wire handler after worker is created
	consumer.SetHandler(worker.HandleMessage)

	// Handle shutdown gracefully
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logger.Log(log.LevelInfo, "msg", "shutdown signal received")
		cancel()
	}()

	logger.Log(log.LevelInfo, "msg", "msgworker starting...")
	if err := worker.Start(ctx); err != nil && err != context.Canceled {
		logger.Log(log.LevelFatal, "msg", fmt.Sprintf("msgworker error: %v", err))
		os.Exit(1)
	}
}

func mustHostname() string {
	h, _ := os.Hostname()
	if h == "" {
		return "msgworker-0"
	}
	return h
}
