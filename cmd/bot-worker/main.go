package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"nonoka-im/internal/authz"
	botadapters "nonoka-im/internal/bot/adapters"
	botapp "nonoka-im/internal/bot/app"
	"nonoka-im/internal/bot/delivery"
	botworker "nonoka-im/internal/bot/worker"
	"nonoka-im/internal/conf"
	"nonoka-im/internal/data"
	botdata "nonoka-im/internal/data/bot"
	imdata "nonoka-im/internal/data/im"
	imapp "nonoka-im/internal/im/app"
	mongostore "nonoka-im/internal/im/storage/mongo"
	kafkax "nonoka-im/internal/kafka"
	"nonoka-im/internal/server"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var flagconf string

func init() {
	flag.StringVar(&flagconf, "conf", "../../configs/bot-worker.yaml", "config path")
}

func main() {
	flag.Parse()
	logger := log.With(log.NewStdLogger(os.Stdout),
		"ts", log.DefaultTimestamp,
		"caller", log.DefaultCaller,
		"service.name", "bot-worker",
		"trace.id", tracing.TraceID(),
		"span.id", tracing.SpanID(),
	)
	loadDotEnv(".env")
	c := config.New(config.WithSource(file.NewSource(flagconf)))
	defer c.Close()
	if err := c.Load(); err != nil {
		panic(err)
	}
	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		panic(err)
	}
	if bc.Data == nil {
		panic("data config is required")
	}
	applyDataEnv(&bc)
	if bc.Data.Mongodb == nil {
		panic("mongodb config is required")
	}

	dataLayer, cleanup, err := data.NewData(bc.Data)
	if err != nil {
		panic(fmt.Errorf("init data layer: %w", err))
	}
	defer cleanup()

	mongoClient, err := mongo.Connect(options.Client().ApplyURI(bc.Data.Mongodb.Uri))
	if err != nil {
		panic(fmt.Errorf("connect mongo: %w", err))
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = mongoClient.Disconnect(shutdownCtx)
	}()
	mongoDB := mongoClient.Database(bc.Data.Mongodb.Database)

	az, err := authz.NewOpenFGAFromEnv()
	if err != nil {
		panic(err)
	}
	storage := mongostore.NewStore(mongoDB, logger)
	query := &imapp.QueryService{Reader: storage, Authorizer: az, Log: log.NewHelper(logger)}
	sealer, err := botadapters.NewSealer()
	if err != nil {
		panic(err)
	}
	store := botdata.NewStore(dataLayer.DB())
	reader := &botadapters.Reader{Query: query}
	users := botadapters.Users{Repo: imdata.NewAuthRepo(dataLayer.DB(), dataLayer.Redis, logger)}
	webhookSender := delivery.NewSender(botadapters.NewWebhookGuard(), logger)
	deliverySvc := botapp.NewDeliveryService(store, reader, users, webhookSender, sealer, logger)
	deliverySvc.Config = botapp.DeliveryConfig{
		MaxAttempts: envInt("BOT_MAX_ATTEMPTS", 8),
		Lock:        envDuration("BOT_DELIVERY_LOCK", 2*time.Minute),
		Jitter:      !strings.EqualFold(os.Getenv("BOT_DELIVERY_JITTER"), "off"),
	}

	consumerCfg := kafkaConfig(bc.Data.Kafka)
	consumer := kafkax.NewConsumer(consumerCfg, nil, logger)
	runner := &botworker.Worker{
		Consumer: consumer, Delivery: deliverySvc, Store: store,
		Log: log.NewHelper(logger), Workers: envInt("BOT_DELIVERY_WORKERS", 8),
		Poll:      envDuration("BOT_DELIVERY_POLL", 200*time.Millisecond),
		StopGrace: envDuration("BOT_SHUTDOWN_GRACE", 30*time.Second),
		Retention: envDuration("BOT_DELIVERY_RETENTION", 30*24*time.Hour),
	}
	consumer.SetHandler(runner.HandleEvent)

	probeAddr := envString("BOT_PROBE_ADDR", ":8092")
	metricsAddr := envString("BOT_METRICS_ADDR", ":8093")
	ready := func(ctx context.Context) error {
		if !consumer.Ready() {
			return fmt.Errorf("kafka consumer not ready")
		}
		if sqlDB, err := dataLayer.DB().DB(); err == nil {
			if err := sqlDB.PingContext(ctx); err != nil {
				return fmt.Errorf("postgres not ready: %w", err)
			}
		}
		if err := mongoClient.Ping(ctx, nil); err != nil {
			return fmt.Errorf("mongodb not ready: %w", err)
		}
		return nil
	}
	health := server.NewProbeServer("health", probeAddr, nil, ready, logger)
	metricsSrv := server.NewProbeServer("metrics", metricsAddr, promhttp.Handler(), nil, logger)

	app := kratos.New(
		kratos.Name("bot-worker"),
		kratos.Server(health, metricsSrv, runner),
	)
	if err := app.Run(); err != nil {
		panic(err)
	}
}

func applyDataEnv(bc *conf.Bootstrap) {
	if bc.Data.Database == nil {
		bc.Data.Database = &conf.Data_Database{}
	}
	if bc.Data.Redis == nil {
		bc.Data.Redis = &conf.Data_Redis{}
	}
	if bc.Data.Mongodb == nil {
		bc.Data.Mongodb = &conf.Data_MongoDB{}
	}
	if bc.Data.Kafka == nil {
		bc.Data.Kafka = &conf.Data_Kafka{}
	}
	if value := strings.TrimSpace(os.Getenv("POSTGRES_DSN")); value != "" {
		bc.Data.Database.Source = value
	}
	if value := strings.TrimSpace(os.Getenv("MONGODB_URI")); value != "" {
		bc.Data.Mongodb.Uri = value
	}
	if value := strings.TrimSpace(os.Getenv("MONGODB_DATABASE")); value != "" {
		bc.Data.Mongodb.Database = value
	}
	if value := strings.TrimSpace(os.Getenv("REDIS_ADDR")); value != "" {
		bc.Data.Redis.Addr = value
	}
	if value := strings.TrimSpace(os.Getenv("KAFKA_BROKERS")); value != "" {
		var brokers []string
		for _, item := range strings.Split(value, ",") {
			if item = strings.TrimSpace(item); item != "" {
				brokers = append(brokers, item)
			}
		}
		if len(brokers) > 0 {
			bc.Data.Kafka.Brokers = brokers
		}
	}
	if value := strings.TrimSpace(os.Getenv("BOT_EVENTS_TOPIC")); value != "" {
		bc.Data.Kafka.Topic = value
	}
	if value := strings.TrimSpace(os.Getenv("BOT_EVENTS_GROUP")); value != "" {
		bc.Data.Kafka.ConsumerGroup = value
	}
	if value := strings.TrimSpace(os.Getenv("BOT_EVENTS_DLQ_TOPIC")); value != "" {
		bc.Data.Kafka.DlqTopic = value
	}
}

func kafkaConfig(kc *conf.Data_Kafka) kafkax.ConsumerConfig {
	cfg := kafkax.ConsumerConfig{
		Brokers: []string{"127.0.0.1:9092"}, Topic: "agent-events",
		GroupID: "nonoka-bot-worker", WorkerCount: 4, MaxRetries: 2,
		HandlerTimeout: 10 * time.Second, RetryBackoff: 500 * time.Millisecond,
		DLQTopic: "agent-events-dlq", CommitBatchSize: 100, CommitFlushInterval: time.Second,
	}
	if kc == nil {
		return cfg
	}
	if len(kc.Brokers) > 0 {
		cfg.Brokers = kc.Brokers
	}
	if kc.Topic != "" {
		cfg.Topic = kc.Topic
	}
	if kc.ConsumerGroup != "" {
		cfg.GroupID = kc.ConsumerGroup
	}
	if kc.DlqTopic != "" {
		cfg.DLQTopic = kc.DlqTopic
	}
	if kc.WorkerCount > 0 {
		cfg.WorkerCount = int(kc.WorkerCount)
	}
	if kc.MaxRetries > 0 {
		cfg.MaxRetries = int(kc.MaxRetries)
	}
	if kc.MaxWait != nil {
		cfg.MaxWait = kc.MaxWait.AsDuration()
	}
	if kc.CommitBatchSize > 0 {
		cfg.CommitBatchSize = int(kc.CommitBatchSize)
	}
	if kc.CommitFlushInterval != nil {
		cfg.CommitFlushInterval = kc.CommitFlushInterval.AsDuration()
	}
	return cfg
}

func envString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func loadDotEnv(path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key != "" && os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
}
