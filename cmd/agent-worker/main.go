package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"nonoka-im/internal/agent/adapters"
	"nonoka-im/internal/agent/app"
	"nonoka-im/internal/agent/app/admin"
	"nonoka-im/internal/agent/bootstrap"
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
	"nonoka-im/internal/audit"
	"nonoka-im/internal/authz"
	"nonoka-im/internal/conf"
	"nonoka-im/internal/data"
	agentdata "nonoka-im/internal/data/agent"
	imdata "nonoka-im/internal/data/im"
	platformdata "nonoka-im/internal/data/platform"
	imapp "nonoka-im/internal/im/app"
	mongostore "nonoka-im/internal/im/storage/mongo"
	kafkax "nonoka-im/internal/kafka"
	"nonoka-im/internal/llm"
	"nonoka-im/internal/messaging/httpclient"
	messagingim "nonoka-im/internal/messaging/im"
	messagingmongo "nonoka-im/internal/messaging/mongo"
	"nonoka-im/internal/realtime"
	"nonoka-im/internal/server"

	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var flagconf string

func init() {
	flag.StringVar(&flagconf, "conf", "../../configs", "config path, eg: -conf config.yaml")
}

func main() {
	flag.Parse()
	loadDotEnv(".env")
	logger := log.NewStdLogger(os.Stdout)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := config.New(config.WithSource(file.NewSource(flagconf)))
	defer c.Close()
	if err := c.Load(); err != nil {
		panic(err)
	}
	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		panic(err)
	}
	secrets := bootstrap.ApplyEnv(&bc)
	if err := bootstrap.Normalize(&bc, secrets); err != nil {
		panic(err)
	}
	aw := bc.AgentWorker

	shutdownTelemetry, err := bootstrap.InitTracer(ctx, bc.Observability.OtlpTracesEndpoint)
	if err != nil {
		panic(err)
	}
	defer shutdownTelemetry()

	agentMetrics := agentmetrics.New()
	dataLayer, cleanup, err := data.NewData(bc.Data)
	if err != nil {
		panic(err)
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
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	if err := mongoClient.Ping(pingCtx, nil); err != nil {
		pingCancel()
		panic(fmt.Errorf("ping mongo: %w", err))
	}
	pingCancel()
	mongoDB := mongoClient.Database(bc.Data.Mongodb.Database)

	az, err := authz.NewOpenFGAFromEnv()
	if err != nil {
		panic(err)
	}
	store := agentdata.NewStore(dataLayer.DB())
	registry := platformdata.NewStore(dataLayer.DB())

	sealKey, err := credential.SealKeyFromEnv()
	if err != nil {
		sealKey = []byte(secrets.SecretKey)
	}
	if len(sealKey) < 32 {
		panic("AGENT_SECRET_KEY must contain at least 32 bytes")
	}
	bifrostURL := strings.TrimRight(strings.TrimSpace(os.Getenv("BIFROST_URL")), "/")
	var gateway llm.Gateway
	if bifrostURL != "" {
		gateway = llm.NewClient(bifrostURL, llm.Auth{
			AdminToken: os.Getenv("BIFROST_ADMIN_TOKEN"),
			SetupToken: os.Getenv("BIFROST_SETUP_TOKEN"),
			Username:   os.Getenv("BIFROST_ADMIN_USERNAME"),
			Password:   os.Getenv("BIFROST_ADMIN_PASSWORD"),
		})
	}
	modelSvc := llm.NewService(registry, llm.EnvSealer{Key: sealKey}, az, gateway, llm.Config{
		BaseURL:             bifrostURL,
		AgentMonthlyBudget:  envFloat("BIFROST_AGENT_MONTHLY_BUDGET_USD", 20),
		GlobalMonthlyBudget: envFloat("BIFROST_GLOBAL_MONTHLY_BUDGET_USD", 200),
		AgentRequestPerMin:  envInt("BIFROST_AGENT_REQUEST_PER_MINUTE", 60),
		AgentTokensPerMin:   envInt("BIFROST_AGENT_TOKENS_PER_MINUTE", 200000),
	})
	var seedModelID int64
	var seedModelKey string
	if bifrostURL != "" && !strings.EqualFold(os.Getenv("BIFROST_AUTO_PROVISION"), "false") {
		preferred := strings.TrimSpace(aw.BootstrapModel)
		model, serr := modelSvc.Bootstrap(ctx, preferred)
		if serr != nil {
			logger.Log(log.LevelWarn, "msg", fmt.Sprintf("bootstrap model catalog: %v", serr))
		} else {
			seedModelID, seedModelKey = int64(model.ID), model.ModelKey
		}
	}
	quotaService := app.NewQuotaService(store, app.QuotaConfig{
		Timezone: envString("AGENT_QUOTA_TIMEZONE", "Asia/Shanghai"),
		User: domain.QuotaLimits{
			MaxConcurrentRuns: envInt("AGENT_USER_MAX_CONCURRENT_RUNS", 2),
			DailyRuns:         envInt("AGENT_USER_DAILY_RUNS", 200),
			DailyTokens:       int64(envInt("AGENT_USER_DAILY_TOKENS", 2_000_000)),
			DailyCostUSD:      envFloat("AGENT_USER_DAILY_COST_USD", 5),
			MaxToolTurns:      envInt("AGENT_USER_MAX_TOOL_TURNS", 8),
		},
		Agent: domain.QuotaLimits{
			MaxConcurrentRuns: envInt("AGENT_MAX_CONCURRENT_RUNS", 4),
			DailyRuns:         envInt("AGENT_DAILY_RUNS", 500),
			DailyTokens:       int64(envInt("AGENT_DAILY_TOKENS", 5_000_000)),
			DailyCostUSD:      envFloat("AGENT_DAILY_COST_USD", 10),
			MaxToolTurns:      envInt("AGENT_MAX_TOOL_TURNS", 8),
		},
		MaxSessionQueued: envInt("AGENT_MAX_SESSION_QUEUED", 3),
		MaxGlobalQueued:  envInt("AGENT_MAX_GLOBAL_QUEUED", 200),
	})

	replySender := &adapters.Sender{
		Client: &httpclient.Client{BaseURL: aw.ImHttpUrl, HTTP: &http.Client{Timeout: 30 * time.Second}},
		TokenIssuer: func(agentID, botID int64, topic string) (string, error) {
			return credential.Sign(secrets.JWTSecret, agentID, botID, topic)
		},
	}
	directory := &adapters.Directory{
		Groups: imdata.NewGroupRepo(dataLayer.DB(), logger),
		Users:  imdata.NewAuthRepo(dataLayer.DB(), dataLayer.Redis, logger),
	}

	messageStorage := mongostore.NewStore(mongoDB, logger)
	query := &imapp.QueryService{Reader: messageStorage, Authorizer: az, Log: log.NewHelper(logger)}
	fileStore := mongostore.NewFileStore(mongoDB)
	toolsBackend := &app.MessageTools{
		Reader:     adapters.Messages{Reader: messagingim.NewReader(query)},
		Authorizer: az,
		Sender:     replySender,
		Files:      adapters.Files{Reader: messagingmongo.NewFiles(fileStore)},
		Directory:  directory,
	}
	mcpProvider := &agentmcp.Provider{
		Store: registry, Sealer: llm.EnvSealer{Key: sealKey}, Guard: agentmcp.NewGuardFromEnv(),
		ToolGuard: az, IdleTTL: 10 * time.Minute, MaxClients: 64,
	}
	defer mcpProvider.Close()

	builtinRegistry := builtin.NewRegistry(toolsBackend, az, 8192)
	deps := eino.Deps{
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

	gatewayGRPC := strings.TrimSpace(os.Getenv("GATEWAY_GRPC_ADDR"))
	if gatewayGRPC == "" && bc.Dispatch != nil {
		for _, node := range bc.Dispatch.Gateways {
			if node.GetGatewayGrpcAddr() != "" {
				gatewayGRPC = node.GetGatewayGrpcAddr()
				break
			}
		}
	}
	var runTransport realtime.RunEventTransport
	if gatewayGRPC != "" {
		pusher, err := realtime.NewGatewayPusher([]string{gatewayGRPC}, logger)
		if err != nil {
			panic(err)
		}
		runTransport = pusher
	}
	broadcaster := realtime.NewRunBroadcaster(
		realtime.NewRedisRunEventStore(dataLayer.Redis),
		runTransport,
		func(ctx context.Context, event realtime.RunEvent) error {
			return store.AppendRunStep(ctx, &domain.AgentRunStep{
				RunID: event.RunID, Seq: event.Seq, Type: string(event.Type), Tool: event.Tool,
				ToolCallID: event.ToolCallID, Summary: event.Summary, Status: event.Status,
				CreatedAt: time.Now(),
			})
		},
		logger,
	)

	harness := &app.Harness{
		Store: store, Runtime: rt, Authorizer: az, Sender: replySender, MaxAttempts: int(aw.MaxAttempts),
		Files: toolsBackend.Files, Quota: quotaService, Stream: broadcaster, Audience: directory, Log: log.NewHelper(logger),
	}
	quotaNotifier := &app.QuotaNotifier{Quota: quotaService, Sender: replySender}
	exec := &executor.Executor{
		Store: store, Registry: registry, Models: modelSvc, Authorizer: az, Harness: harness,
		Reader: toolsBackend.Reader,
		Quota:  quotaService, Notifier: quotaNotifier,
		MaxAttempts: int(aw.MaxAttempts), RunTimeout: aw.RunTimeout.AsDuration(),
		Lease: aw.Lease.AsDuration(), Stream: broadcaster, Log: log.NewHelper(logger),
	}
	auditSvc := audit.NewService(registry)
	mcpAdmin := admin.NewMCPService(registry, az, mcpProvider, llm.EnvSealer{Key: sealKey}, auditSvc, logger)
	dispatcher := &worker.Dispatcher{
		Store: store, Authorizer: az, Reader: toolsBackend.Reader,
		Quota: quotaService, Notifier: quotaNotifier, Log: log.NewHelper(logger),
	}
	sweeperRunner := &sweeper.Sweeper{Store: store, Metrics: agentMetrics, Log: log.NewHelper(logger), Interval: aw.SweepInterval.AsDuration(), MaxAttempts: int(aw.MaxAttempts)}

	consumer := kafkax.NewConsumer(kafkax.ConsumerConfig{
		Brokers: bc.Data.Kafka.Brokers, Topic: bc.Data.Kafka.Topic, GroupID: bc.Data.Kafka.ConsumerGroup,
		WorkerCount: int(bc.Data.Kafka.WorkerCount), MaxRetries: int(bc.Data.Kafka.MaxRetries),
		HandlerTimeout: aw.DispatchTimeout.AsDuration(), RetryBackoff: 500 * time.Millisecond,
		DLQTopic: bc.Data.Kafka.DlqTopic, CommitBatchSize: int(bc.Data.Kafka.CommitBatchSize),
		CommitFlushInterval: durationOrZero(bc.Data.Kafka.CommitFlushInterval),
	}, nil, logger)
	runner := &worker.Worker{
		Store: store, Dispatcher: dispatcher, Executor: exec, Sweeper: sweeperRunner, Consumer: consumer,
		Metrics: agentMetrics, Log: log.NewHelper(logger),
		RunWorkers: int(aw.RunWorkers), QueuedPoll: aw.QueuedPoll.AsDuration(),
		Background: []func(context.Context){
			func(ctx context.Context) { mcpProvider.StartJanitor(ctx, 5*time.Minute) },
			func(ctx context.Context) { mcpAdmin.HealthCheckLoop(ctx, 5*time.Minute) },
			func(ctx context.Context) { ensureAgentGatewayKeys(ctx, store, modelSvc, logger) },
			func(ctx context.Context) {
				ticker := time.NewTicker(24 * time.Hour)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						days := envInt("AUDIT_RETENTION_DAYS", 365)
						if days <= 0 {
							continue
						}
						cutoff := time.Now().AddDate(0, 0, -days)
						if n, err := registry.PurgeAuditEvents(ctx, cutoff, 5000); err != nil {
							logger.Log(log.LevelWarn, "msg", fmt.Sprintf("audit retention purge: %v", err))
						} else if n > 0 {
							logger.Log(log.LevelInfo, "msg", fmt.Sprintf("audit retention purge removed %d events", n))
						}
					}
				}
			},
		},
	}
	consumer.SetHandler(runner.HandleEvent)

	healthAddr := bc.Observability.HealthAddr
	metricsAddr := bc.Observability.MetricsAddr
	ready := func(ctx context.Context) error {
		if !runner.Ready() {
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
	health := server.NewProbeServer("agent-worker-health", healthAddr, nil, ready, logger)
	var metrics *server.ProbeServer
	if metricsAddr != "" && metricsAddr != healthAddr {
		metrics = server.NewProbeServer("agent-worker-metrics", metricsAddr, promhttp.HandlerFor(agentMetrics.Registry, promhttp.HandlerOpts{}), nil, logger)
	}
	servers := []transport.Server{runner, health}
	if metrics != nil {
		servers = append(servers, metrics)
	}
	runner.Background = append(runner.Background, func(ctx context.Context) {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if n, err := modelSvc.CleanupKeys(ctx); err != nil {
					logger.Log(log.LevelWarn, "msg", fmt.Sprintf("gateway key cleanup: %v", err))
				} else if n > 0 {
					logger.Log(log.LevelInfo, "msg", fmt.Sprintf("gateway key cleanup removed %d keys", n))
				}
			}
		}
	})

	application := kratos.New(
		kratos.Name("agent-worker"),
		kratos.Logger(logger),
		kratos.Server(servers...),
		kratos.BeforeStart(func(ctx context.Context) error {
			if aw.BootstrapOwnerId > 0 && seedModelID <= 0 {
				logger.Log(log.LevelWarn, "msg", "bootstrap agent skipped: no curated model available (configure BIFROST_URL and BIFROST_DEFAULT_MODEL)")
				return nil
			}
			return bootstrap.SeedAgent(ctx, store, az, bootstrap.SeedInput{
				AgentID: aw.BootstrapAgentId, BotID: aw.BootstrapBotId, OwnerID: aw.BootstrapOwnerId,
				Name: aw.BootstrapName, Instruction: aw.BootstrapInstruction,
				ModelID: seedModelID, ModelKey: seedModelKey,
			}, logger)
		}),
	)
	if err := application.Run(); err != nil {
		logger.Log(log.LevelError, "msg", fmt.Sprintf("agent worker: %v", err))
		os.Exit(1)
	}
}

func durationOrZero(d interface{ AsDuration() time.Duration }) time.Duration {
	if d == nil {
		return 0
	}
	return d.AsDuration()
}

func envFloat(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v < 0 {
		return fallback
	}
	return v
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		return fallback
	}
	return v
}

func envString(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// ensureAgentGatewayKeys backfills per-agent virtual keys after migration or a
// Bifrost outage. It is idempotent and retries on a timer.
func ensureAgentGatewayKeys(ctx context.Context, store *agentdata.Store, models *llm.Service, logger log.Logger) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	run := func() {
		agents, err := store.ListPublishedAgents(ctx)
		if err != nil {
			logger.Log(log.LevelWarn, "msg", fmt.Sprintf("list published agents for keys: %v", err))
			return
		}
		for i := range agents {
			versions, err := store.ListVersions(ctx, agents[i].ID)
			if err != nil {
				continue
			}
			seen := map[int64]bool{}
			modelsForAgent := make([]domain.Model, 0, len(versions))
			for _, version := range versions {
				if version.ModelID <= 0 || seen[version.ModelID] {
					continue
				}
				seen[version.ModelID] = true
				model, err := models.Lookup(ctx, version.ModelID)
				if err != nil {
					continue
				}
				modelsForAgent = append(modelsForAgent, *model)
			}
			if len(modelsForAgent) == 0 {
				continue
			}
			if err := models.EnsureAgentKey(ctx, agents[i].ID, modelsForAgent); err != nil {
				logger.Log(log.LevelWarn, "msg", fmt.Sprintf("ensure agent key agent=%d: %v", agents[i].ID, err))
			}
		}
	}
	run()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
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
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), "\"'")
		if key != "" && os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
}
