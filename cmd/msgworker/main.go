package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nonoka-im/internal/conf"
	"nonoka-im/internal/data"
	"nonoka-im/internal/metrics"
	"nonoka-im/internal/msgworker"

	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
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

	// Initialize data layer (PostgreSQL + Redis)
	if bc.Data == nil {
		logger.Log(log.LevelFatal, "msg", "data config is required")
		os.Exit(1)
	}
	dataLayer, dataCleanup, err := data.NewData(bc.Data)
	if err != nil {
		logger.Log(log.LevelFatal, "msg", fmt.Sprintf("init data layer: %v", err))
		os.Exit(1)
	}
	defer dataCleanup()

	redisClient := dataLayer.Redis
	if redisClient == nil {
		logger.Log(log.LevelFatal, "msg", "redis config is required")
		os.Exit(1)
	}

	// Initialize MongoDB
	if bc.Data.Mongodb == nil || bc.Data.Mongodb.Uri == "" {
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

	// Verify MongoDB connectivity early.
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	if err := mongoClient.Ping(pingCtx, nil); err != nil {
		pingCancel()
		logger.Log(log.LevelFatal, "msg", fmt.Sprintf("ping mongodb: %v", err))
		os.Exit(1)
	}
	pingCancel()

	mongoDB := mongoClient.Database(bc.Data.Mongodb.Database)

	// Initialize Kafka config from protobuf settings.
	kafkaCfg := msgworkerConfigFromProto(bc.Data.Kafka)
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
	m := metrics.NewMetrics()
	seqGen := msgworker.NewSeqGenerator(redisClient, logger)
	snowflake := msgworker.NewSnowflake(1)
	storage := msgworker.NewMessageStorage(mongoDB, logger, m)
	storage.SetRedis(redisClient)
	if err := storage.EnsureIndexes(ctx); err != nil {
		logger.Log(log.LevelFatal, "msg", fmt.Sprintf("ensure indexes: %v", err))
		os.Exit(1)
	}

	pusher, err := msgworker.NewGatewayPusher([]string{gatewayAddr}, logger, m)
	if err != nil {
		logger.Log(log.LevelFatal, "msg", fmt.Sprintf("gateway pusher init failed: %v", err))
		os.Exit(1)
	}

	// Configure dynamic gateway routing so pushes go only to nodes that host
	// the target user's sessions, instead of broadcasting to all gateways.
	nodeTTL := 30 * time.Second
	if bc.Dispatch != nil && bc.Dispatch.NodeTtl != nil {
		nodeTTL = bc.Dispatch.NodeTtl.AsDuration()
	}
	if pusher != nil {
		router := msgworker.NewGatewayRouter(redisClient, nodeTTL, logger)
		pusher.SetRouter(router)
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
		m,
	)

	// Start a small HTTP server for Prometheus metrics and pprof.
	metricsAddr := os.Getenv("METRICS_ADDR")
	if metricsAddr == "" {
		metricsAddr = "0.0.0.0:18001"
	}
	metricsSrv := &http.Server{Addr: metricsAddr, Handler: nil}
	http.Handle("/metrics", m.Handler())
	go func() {
		logger.Log(log.LevelInfo, "msg", fmt.Sprintf("msgworker metrics server listening on %s", metricsAddr))
		if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Log(log.LevelWarn, "msg", fmt.Sprintf("metrics server error: %v", err))
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = metricsSrv.Shutdown(shutdownCtx)
	}()

	// Wire push retry queue so failed online pushes are retried asynchronously.
	if pusher != nil {
		retryQueue := msgworker.NewPushRetryQueue(redisClient, pusher, logger)
		worker.SetRetryQueue(retryQueue)
	}

	// Initialize group member service (PostgreSQL-backed with Redis cache)
	groupMemberRepo := data.NewGroupMemberRepo(dataLayer, logger)
	groupMemberSvc := msgworker.NewPersistentGroupMemberService(groupMemberRepo, redisClient, logger)
	worker.SetGroupMemberService(groupMemberSvc)

	// Wire handler after worker is created
	consumer.SetHandler(worker.HandleMessage)

	// Ensure graceful shutdown: close consumer, retry queue, and pusher.
	defer func() {
		if err := worker.Stop(); err != nil {
			logger.Log(log.LevelWarn, "msg", fmt.Sprintf("worker stop error: %v", err))
		}
	}()

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

// msgworkerConfigFromProto builds a msgworker.KafkaConsumerConfig from the
// protobuf Kafka config. Defaults are applied by NewKafkaConsumer.
func msgworkerConfigFromProto(kc *conf.Data_Kafka) msgworker.KafkaConsumerConfig {
	if kc == nil {
		return msgworker.KafkaConsumerConfig{}
	}
	cfg := msgworker.KafkaConsumerConfig{
		Brokers:     kc.GetBrokers(),
		Topic:       kc.GetTopic(),
		GroupID:     kc.GetConsumerGroup(),
		WorkerCount: int(kc.GetWorkerCount()),
		MinBytes:    int(kc.GetMinBytes()),
		MaxBytes:    int(kc.GetMaxBytes()),
		MaxRetries:  int(kc.GetMaxRetries()),
	}
	if kc.GetMaxWait() != nil {
		cfg.MaxWait = kc.GetMaxWait().AsDuration()
	}
	if kc.GetCommitInterval() != nil {
		cfg.CommitInterval = kc.GetCommitInterval().AsDuration()
	}
	if so := kc.GetStartOffset(); so != 0 {
		cfg.StartOffset = so
	}
	if kc.GetCommitBatchSize() != 0 {
		cfg.CommitBatchSize = int(kc.GetCommitBatchSize())
	}
	if kc.GetCommitFlushInterval() != nil {
		cfg.CommitFlushInterval = kc.GetCommitFlushInterval().AsDuration()
	}
	return cfg
}
