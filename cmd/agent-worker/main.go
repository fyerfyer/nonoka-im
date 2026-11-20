package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"nonoka-im/internal/agent/adapters"
	"nonoka-im/internal/agent/app"
	"nonoka-im/internal/agent/bootstrap"
	"nonoka-im/internal/agent/config"
	"nonoka-im/internal/agent/credential"
	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/eino"
	"nonoka-im/internal/agent/executor"
	agentmcp "nonoka-im/internal/agent/mcp"
	agentmetrics "nonoka-im/internal/agent/metrics"
	agentport "nonoka-im/internal/agent/port"
	agentskills "nonoka-im/internal/agent/skills"
	agentstore "nonoka-im/internal/agent/store"
	"nonoka-im/internal/agent/sweeper"
	"nonoka-im/internal/agent/tools"
	"nonoka-im/internal/agent/tools/builtin"
	"nonoka-im/internal/agent/worker"
	"nonoka-im/internal/authz"
	"nonoka-im/internal/conf"
	"nonoka-im/internal/data"
	agentdata "nonoka-im/internal/data/agent"
	imdata "nonoka-im/internal/data/im"
	platformdata "nonoka-im/internal/data/platform"
	kafkax "nonoka-im/internal/kafka"
	"nonoka-im/internal/messaging"
	"nonoka-im/internal/msgworker"

	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/go-kratos/kratos/v2/log"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func main() {
	loadDotEnv(".env")
	logger := log.NewStdLogger(os.Stdout)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.Load(configPath())
	if err != nil {
		panic(err)
	}
	shutdownTelemetry, err := bootstrap.InitTracer(ctx, cfg.Observability)
	if err != nil {
		panic(err)
	}
	defer shutdownTelemetry()

	agentMetrics := agentmetrics.New()
	shutdownProbes := bootstrap.StartHealthAndMetrics(cfg.Observability, agentMetrics, logger)
	defer shutdownProbes()

	dataLayer, cleanup, err := data.NewData(&conf.Data{Database: &conf.Data_Database{Source: cfg.Data.PostgresDSN}})
	if err != nil {
		panic(err)
	}
	defer cleanup()

	mongoClient, err := mongo.Connect(options.Client().ApplyURI(cfg.Data.MongoDBURI))
	if err != nil {
		panic(fmt.Errorf("connect mongo: %w", err))
	}
	defer func() { _ = mongoClient.Disconnect(context.Background()) }()
	if err := mongoClient.Ping(ctx, nil); err != nil {
		panic(fmt.Errorf("ping mongo: %w", err))
	}
	mongoDB := mongoClient.Database(cfg.Data.MongoDBDatabase)
	messageStorage := msgworker.NewMessageStorage(mongoDB, logger)

	az, err := authz.NewOpenFGAFromEnv()
	if err != nil {
		panic(err)
	}
	store := agentdata.NewStore(dataLayer.DB())
	registry := platformdata.NewStore(dataLayer.DB())

	sealKey := []byte(cfg.SecretKey)
	if len(sealKey) < 32 {
		if fromEnv, err := credential.SealKeyFromEnv(); err == nil {
			sealKey = fromEnv
		}
	}
	if len(sealKey) < 32 {
		panic("AGENT_SECRET_KEY must contain at least 32 bytes")
	}
	if err := bootstrap.SeedAgent(ctx, store, az, cfg, logger); err != nil {
		panic(err)
	}

	if cfg.Model.APIKey == "" {
		// The demo stack starts the worker without a shared model key. Agents
		// that carry their own key still run; this placeholder only fails for
		// agents relying on the missing environment fallback.
		logger.Log(log.LevelWarn, "msg", "OPENAI_API_KEY/DEEPSEEK_API_KEY not set; only agents with their own API key will run")
		cfg.Model.APIKey = "missing-model-key"
	}

	replySender := &adapters.Sender{
		Client: &messaging.Client{BaseURL: cfg.IM.HTTPURL, HTTP: &http.Client{Timeout: 30 * time.Second}},
		TokenIssuer: func(agentID, botID int64, topic string) (string, error) {
			return credential.Sign(cfg.JWTSecret, agentID, botID, topic)
		},
	}
	directory := &adapters.Directory{
		Groups: imdata.NewGroupRepo(dataLayer.DB(), logger),
		Users:  imdata.NewAuthRepo(dataLayer.DB(), dataLayer.Redis, logger),
	}
	toolsBackend := &app.MessageTools{
		Reader:     adapters.Messages{Reader: messaging.NewReader(messageStorage)},
		Authorizer: az,
		Sender:     replySender,
		Files:      adapters.Files{Reader: messaging.NewFiles(mongoDB)},
		Directory:  directory,
	}
	mcpProvider := &agentmcp.Provider{
		Store: registry, Guard: az, IdleTTL: 10 * time.Minute, MaxClients: 64,
	}
	defer mcpProvider.Close()
	go mcpProvider.StartJanitor(ctx, 5*time.Minute)

	builtinRegistry := builtin.NewRegistry(toolsBackend, az, 8192)
	deps := eino.Deps{
		APIKey: cfg.Model.APIKey, BaseURL: cfg.Model.BaseURL, Model: cfg.Model.Name,
		MCPToolProvider: mcpProvider.Tools,
		ToolCatalog:     builtinRegistry,
		SkillBackendFactory: func(bindings []agentport.SkillBinding) (skill.Backend, error) {
			return agentskills.NewBackend(registry, bindings), nil
		},
		Checkpoints: &agentstore.CheckPoints{Store: store},
		Ledger:      store,
		MarkSideEffect: func(ctx context.Context, rc tools.RunContext) error {
			ok, err := store.MarkRunSideEffect(ctx, rc.RunID, rc.AttemptID)
			if err != nil {
				return err
			}
			if !ok {
				return domain.ErrRunState
			}
			return nil
		},
	}
	for _, t := range builtinRegistry.Tools() {
		deps.Tools = append(deps.Tools, t)
	}
	rt, err := eino.New(ctx, deps)
	if err != nil {
		panic(err)
	}

	harness := &app.Harness{
		Store:       store,
		Runtime:     rt,
		Authorizer:  az,
		Sender:      replySender,
		MaxAttempts: cfg.Worker.MaxAttempts,
	}
	exec := &executor.Executor{
		Store: store, Registry: registry, Authorizer: az, Harness: harness,
		Reader: toolsBackend.Reader, SealKey: sealKey,
		MaxAttempts: cfg.Worker.MaxAttempts, RunTimeout: cfg.Worker.RunTimeout.Value(),
		Lease: cfg.Worker.Lease.Value(), Log: log.NewHelper(logger),
	}
	dispatcher := &worker.Dispatcher{Store: store, Authorizer: az, Reader: toolsBackend.Reader, Log: log.NewHelper(logger)}
	sw := &sweeper.Sweeper{Store: store, Metrics: agentMetrics, Log: log.NewHelper(logger), Interval: cfg.Worker.SweepInterval.Value(), MaxAttempts: cfg.Worker.MaxAttempts}
	consumer := kafkax.NewConsumer(kafkax.ConsumerConfig{
		Brokers: cfg.Kafka.Brokers, Topic: cfg.Kafka.EventsTopic, GroupID: cfg.Kafka.EventsGroup,
		WorkerCount: cfg.Kafka.ConsumerWorkers, MaxRetries: cfg.Kafka.MaxRetries,
		HandlerTimeout: cfg.Kafka.DispatchTimeout.Value(), RetryBackoff: 500 * time.Millisecond,
		DLQTopic: cfg.Kafka.DLQTopic,
	}, nil, logger)
	workerRunner := &worker.Worker{
		Store: store, Dispatcher: dispatcher, Executor: exec, Sweeper: sw, Consumer: consumer,
		Metrics: agentMetrics, Log: log.NewHelper(logger),
		RunWorkers: cfg.Worker.RunWorkers, QueuedPoll: cfg.Worker.QueuedPoll.Value(),
	}
	consumer.SetHandler(workerRunner.HandleEvent)

	if err := workerRunner.Run(ctx); err != nil && ctx.Err() == nil {
		logger.Log(log.LevelError, "msg", fmt.Sprintf("agent consumer: %v", err))
		os.Exit(1)
	}
}

func configPath() string {
	if path := os.Getenv("AGENT_CONFIG"); path != "" {
		return path
	}
	for _, path := range []string{"configs/agent-worker.yaml", "/app/configs/agent-worker.yaml"} {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func loadDotEnv(path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), "\"'")
		if key != "" && os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
}
