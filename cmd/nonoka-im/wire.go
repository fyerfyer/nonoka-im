//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"nonoka-im/internal/agent/app"
	"nonoka-im/internal/agent/app/admin"
	"nonoka-im/internal/agent/credential"
	"nonoka-im/internal/agent/domain"
	agentmcp "nonoka-im/internal/agent/mcp"
	"nonoka-im/internal/alert"
	"nonoka-im/internal/audit"
	"nonoka-im/internal/authz"
	"nonoka-im/internal/biz"
	botadapters "nonoka-im/internal/bot/adapters"
	botapp "nonoka-im/internal/bot/app"
	botdelivery "nonoka-im/internal/bot/delivery"
	botratelimit "nonoka-im/internal/bot/ratelimit"
	botstream "nonoka-im/internal/bot/stream"
	"nonoka-im/internal/conf"
	"nonoka-im/internal/data"
	agentdata "nonoka-im/internal/data/agent"
	botdata "nonoka-im/internal/data/bot"
	imdata "nonoka-im/internal/data/im"
	platformdata "nonoka-im/internal/data/platform"
	"nonoka-im/internal/gateway"
	imapp "nonoka-im/internal/im/app"
	"nonoka-im/internal/im/search"
	mongostore "nonoka-im/internal/im/storage/mongo"
	"nonoka-im/internal/llm"
	"nonoka-im/internal/messaging"
	messagingim "nonoka-im/internal/messaging/im"
	"nonoka-im/internal/metrics"
	"nonoka-im/internal/realtime"
	"nonoka-im/internal/server"
	"nonoka-im/internal/service"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"gorm.io/gorm"
)

// provideKafkaConfig returns Kafka configuration from the config file, falling
// back to environment variables and sensible defaults. By default the producer
// operates synchronously (Async=false) so that Gateway only ACKs clients after
// Kafka has acknowledged the message.
func provideKafkaConfig(confData *conf.Data) gateway.KafkaConfig {
	var cfg gateway.KafkaConfig
	if confData != nil && confData.Kafka != nil {
		cfg = gateway.KafkaConfigFromProto(confData.Kafka)
	}

	// Environment variables override config file values.
	if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
		cfg.Brokers = nil
		for _, b := range strings.Split(brokers, ",") {
			if b = strings.TrimSpace(b); b != "" {
				cfg.Brokers = append(cfg.Brokers, b)
			}
		}
	}
	if topic := os.Getenv("KAFKA_TOPIC"); topic != "" {
		cfg.Topic = topic
	}

	if len(cfg.Brokers) == 0 {
		cfg.Brokers = []string{"127.0.0.1:9092"}
	}
	if cfg.Topic == "" {
		cfg.Topic = "im-messages"
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 100
	}
	return cfg
}

// provideNodeIDString returns the unique node ID as a plain string.
// This is used by gateway.SessionManager which expects a string.
func provideNodeIDString() string {
	nodeID := os.Getenv("GATEWAY_NODE_ID")
	if nodeID == "" {
		nodeID = id
	}
	if nodeID == "" {
		nodeID, _ = os.Hostname()
	}
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

// GatewayGrpcAddr is the gRPC endpoint advertised to other services (msgworker).
type GatewayGrpcAddr string

// provideGatewayGrpcAddr derives the gRPC address from environment or config.
func provideGatewayGrpcAddr(dispatchConf *conf.Dispatch, serverConf *conf.Server, nodeID NodeID) GatewayGrpcAddr {
	id := string(nodeID)
	// Check static config first
	if dispatchConf != nil {
		for _, gw := range dispatchConf.Gateways {
			if gw.NodeId == id && gw.GetGatewayGrpcAddr() != "" {
				return GatewayGrpcAddr(gw.GetGatewayGrpcAddr())
			}
		}
		if len(dispatchConf.Gateways) > 0 && dispatchConf.Gateways[0].GetGatewayGrpcAddr() != "" {
			return GatewayGrpcAddr(dispatchConf.Gateways[0].GetGatewayGrpcAddr())
		}
	}
	// Fallback to env, then server config, then default
	grpcAddr := os.Getenv("GATEWAY_GRPC_ADDR")
	if grpcAddr == "" && serverConf != nil && serverConf.Grpc != nil && serverConf.Grpc.Addr != "" {
		grpcAddr = serverConf.Grpc.Addr
	}
	if grpcAddr == "" {
		grpcAddr = "127.0.0.1:9000"
	}
	return GatewayGrpcAddr(grpcAddr)
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
			cfg.ReadTimeout = gatewayConf.ReadTimeout.AsDuration()
		}
		if gatewayConf.WriteTimeout != nil {
			cfg.WriteTimeout = gatewayConf.WriteTimeout.AsDuration()
		}
	}
	return cfg
}

// provideRedisClient extracts the Redis client from Data.
func provideRedisClient(d *data.Data) redis.UniversalClient {
	return d.Redis
}

// provideGormDB exposes the shared PostgreSQL handle to data subpackages.
func provideGormDB(d *data.Data) *gorm.DB {
	return d.DB()
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
func provideMessageStorage(db *mongo.Database, logger log.Logger, m *metrics.Metrics) (*mongostore.Store, error) {
	storage := mongostore.NewStore(db, logger, m)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := storage.EnsureIndexes(ctx); err != nil {
		return nil, fmt.Errorf("ensure mongodb indexes: %w", err)
	}
	return storage, nil
}

func provideFileStore(db *mongo.Database) *mongostore.FileStore {
	return mongostore.NewFileStore(db)
}

func provideIMMessageService(producer *gateway.DomainProducer, authorizer authz.Authorizer, attachments *imapp.FileService, logger log.Logger, m *metrics.Metrics) *imapp.MessageService {
	return &imapp.MessageService{
		Producer: producer, Authorizer: authorizer, Attachments: attachments,
		Metrics: m, Log: log.NewHelper(logger),
	}
}

func provideSearchIndex(searchConf *conf.Search, db *mongo.Database) (imapp.SearchIndex, error) {
	return search.NewIndex(searchConf, db)
}

func provideIMQueryService(storage *mongostore.Store, authorizer authz.Authorizer, searchIndex imapp.SearchIndex, logger log.Logger) *imapp.QueryService {
	return &imapp.QueryService{Reader: storage, Search: searchIndex, Authorizer: authorizer, Log: log.NewHelper(logger)}
}

func provideIMConversationService(repo imdata.ConversationRepo) *imapp.ConversationService {
	return &imapp.ConversationService{Repo: repo}
}

func provideIMGroupService(repo imdata.GroupRepo, authorizer *authz.OpenFGA) *imapp.GroupService {
	return &imapp.GroupService{Repo: repo, Authorizer: authorizer}
}

func provideIMFileService(store *mongostore.FileStore, authorizer *authz.OpenFGA) *imapp.FileService {
	return &imapp.FileService{Store: store, Authorizer: authorizer, MaxUpload: service.DefaultMaxUploadBytes}
}

func provideIMOutboxService(storage *mongostore.Store, logger log.Logger) *imapp.OutboxService {
	svc := &imapp.OutboxService{Repo: storage, Log: log.NewHelper(logger)}
	if url := strings.TrimSpace(os.Getenv("AGENT_OUTBOX_ALERT_WEBHOOK")); url != "" {
		svc.Notifier = alert.NewWebhook(url)
	}
	if raw := strings.TrimSpace(os.Getenv("AGENT_OUTBOX_ALERT_MIN_INTERVAL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil {
			svc.AlertMinInterval = d
		}
	}
	if raw := strings.TrimSpace(os.Getenv("AGENT_OUTBOX_ALERT_MAX_AGE")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil {
			svc.AlertMaxAge = d
		}
	}
	return svc
}

func provideOutboxAdmin(outbox *imapp.OutboxService) messaging.OutboxAdmin {
	return messagingim.NewOutboxAdmin(outbox)
}

func provideAgentAdminService(store *agentdata.Store, platform *platformdata.Store, models *llm.Service, az *authz.OpenFGA, groups imdata.GroupRepo, users biz.AuthRepo, auditor *audit.Service, logger log.Logger) *admin.AgentService {
	return admin.NewAgentService(store, platform, models, models, az, groups, userDirectory{repo: users}, auditor, logger)
}

func provideAuditService(platform *platformdata.Store) *audit.Service {
	return audit.NewService(platform)
}

func provideQuotaService(store *agentdata.Store, az *authz.OpenFGA, auditor *audit.Service) *app.QuotaService {
	svc := app.NewQuotaService(store, app.QuotaConfig{
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
	svc.Authz = az
	svc.Audit = auditor
	return svc
}

func provideBotStore(db *gorm.DB) *botdata.Store { return botdata.NewStore(db) }

func provideBotService(store *botdata.Store, az *authz.OpenFGA, auditor *audit.Service, logger log.Logger) (*botapp.Service, error) {
	sealer, err := botadapters.NewSealer()
	if err != nil {
		return nil, err
	}
	return botapp.NewService(store, az, sealer, botadapters.NewWebhookGuard(), auditor, logger), nil
}

func provideBotAuth(store *botdata.Store, logger log.Logger) *botapp.AuthService {
	return botapp.NewAuthService(store, logger)
}

func provideBotReader(query *imapp.QueryService) *botadapters.Reader {
	return &botadapters.Reader{Query: query}
}

func provideBotSender(messages *imapp.MessageService) *botadapters.Sender {
	return &botadapters.Sender{Messages: messages}
}

func provideBotUsers(users biz.AuthRepo) botadapters.Users {
	return botadapters.Users{Repo: users}
}

func provideBotGroupMembers(groups imdata.GroupRepo) botadapters.GroupMembers {
	return botadapters.GroupMembers{Repo: groups}
}

func provideBotRecipients(members botadapters.GroupMembers) botadapters.Recipients {
	return botadapters.Recipients{Groups: members}
}

func provideBotDelivery(store *botdata.Store, reader *botadapters.Reader, users botadapters.Users, sender *botadapters.Sender, logger log.Logger) (*botapp.DeliveryService, error) {
	sealer, err := botadapters.NewSealer()
	if err != nil {
		return nil, err
	}
	webhookSender := botdelivery.NewSender(botadapters.NewWebhookGuard(), logger)
	return botapp.NewDeliveryService(store, reader, users, webhookSender, sealer, logger), nil
}

func provideBotStreamPublisher(dispatch *conf.Dispatch, local GatewayGrpcAddr, logger log.Logger) (*botadapters.StreamPublisher, func(), error) {
	var addrs []string
	if dispatch != nil {
		for _, node := range dispatch.Gateways {
			if node.GetGatewayGrpcAddr() != "" {
				addrs = append(addrs, node.GetGatewayGrpcAddr())
			}
		}
	}
	if len(addrs) == 0 && local != "" {
		addrs = append(addrs, string(local))
	}
	if len(addrs) == 0 {
		return &botadapters.StreamPublisher{}, func() {}, nil
	}
	pusher, err := realtime.NewGatewayPusher(addrs, logger)
	if err != nil {
		return nil, nil, err
	}
	return &botadapters.StreamPublisher{Transport: pusher}, func() { _ = pusher.Close() }, nil
}

func provideBotStreamService(store *botdata.Store, az *authz.OpenFGA, streams *botstream.RedisStore, sender *botadapters.Sender, recipients botadapters.Recipients, publisher *botadapters.StreamPublisher, logger log.Logger) *botapp.StreamService {
	svc := botapp.NewStreamService(store, az, streams, sender, recipients, publisher, logger)
	return svc
}

func provideBotStreamStore(redisClient redis.UniversalClient) *botstream.RedisStore {
	return botstream.NewRedisStore(redisClient)
}

func provideBotHTTP(bots *botapp.Service, auth *botapp.AuthService, delivery *botapp.DeliveryService, streams *botapp.StreamService, reader *botadapters.Reader, sender *botadapters.Sender, limiter *botratelimit.Limiter, logger log.Logger) *service.BotHTTP {
	return service.NewBotHTTP(bots, auth, delivery, streams, reader, sender, limiter, logger)
}

func provideBotRateLimiter(redisClient redis.UniversalClient) *botratelimit.Limiter {
	return botratelimit.New(redisClient)
}

func envString(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// provideModelService wires the flat model catalog and per-agent virtual keys
// to Bifrost. An empty BIFROST_URL disables gateway management; runtime
// resolution then fails closed.
func provideModelService(platform *platformdata.Store, az *authz.OpenFGA, auditor *audit.Service) *llm.Service {
	sealKey, _ := credential.SealKeyFromEnv()
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("BIFROST_URL")), "/")
	var gateway llm.Gateway
	if baseURL != "" {
		gateway = llm.NewClient(baseURL, llm.Auth{
			AdminToken: os.Getenv("BIFROST_ADMIN_TOKEN"),
			SetupToken: os.Getenv("BIFROST_SETUP_TOKEN"),
			Username:   os.Getenv("BIFROST_ADMIN_USERNAME"),
			Password:   os.Getenv("BIFROST_ADMIN_PASSWORD"),
		})
	}
	svc := llm.NewService(platform, llm.EnvSealer{Key: sealKey}, az, gateway, llm.Config{
		BaseURL:             baseURL,
		AgentMonthlyBudget:  envFloat("BIFROST_AGENT_MONTHLY_BUDGET_USD", 20),
		GlobalMonthlyBudget: envFloat("BIFROST_GLOBAL_MONTHLY_BUDGET_USD", 200),
		AgentRequestPerMin:  envInt("BIFROST_AGENT_REQUEST_PER_MINUTE", 60),
		AgentTokensPerMin:   envInt("BIFROST_AGENT_TOKENS_PER_MINUTE", 200000),
	})
	svc.Audit = auditor
	return svc
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

func provideRunEventBroadcaster(redisClient redis.UniversalClient, dispatch *conf.Dispatch, logger log.Logger) (*realtime.RunBroadcaster, func(), error) {
	var addrs []string
	if dispatch != nil {
		for _, node := range dispatch.Gateways {
			if node.GetGatewayGrpcAddr() != "" {
				addrs = append(addrs, node.GetGatewayGrpcAddr())
			}
		}
	}
	if len(addrs) == 0 {
		if addr := strings.TrimSpace(os.Getenv("GATEWAY_GRPC_ADDR")); addr != "" {
			addrs = []string{addr}
		}
	}
	var transport realtime.RunEventTransport
	cleanup := func() {}
	if len(addrs) > 0 {
		pusher, err := realtime.NewGatewayPusher(addrs, logger)
		if err != nil {
			return nil, nil, err
		}
		transport = pusher
		cleanup = func() { _ = pusher.Close() }
	}
	broadcaster := realtime.NewRunBroadcaster(realtime.NewRedisRunEventStore(redisClient), transport, nil, logger)
	return broadcaster, cleanup, nil
}

func provideAgentRunService(store *agentdata.Store, az *authz.OpenFGA, redisClient redis.UniversalClient, publisher *realtime.RunBroadcaster) *admin.RunService {
	return admin.NewRunService(store, az, realtime.NewRedisRunEventStore(redisClient), publisher)
}

func provideAgentApprovalService(store *agentdata.Store, az *authz.OpenFGA, auditor *audit.Service, m *metrics.Metrics) *admin.ApprovalService {
	svc := admin.NewApprovalService(store, az, auditor)
	svc.Metrics = m
	return svc
}

func provideAgentMCPService(platform *platformdata.Store, az *authz.OpenFGA, auditor *audit.Service, logger log.Logger) *admin.MCPService {
	sealKey, _ := credential.SealKeyFromEnv()
	sealer := llm.EnvSealer{Key: sealKey}
	discovery := &agentmcp.Provider{
		Store: platform, Sealer: sealer, Guard: agentmcp.NewGuardFromEnv(),
	}
	return admin.NewMCPService(platform, az, discovery, sealer, auditor, logger)
}

func provideAgentSkillService(platform *platformdata.Store, az *authz.OpenFGA, auditor *audit.Service) *admin.SkillService {
	return admin.NewSkillService(platform, az, auditor)
}

type userDirectory struct{ repo biz.AuthRepo }

func (u userDirectory) UserExists(ctx context.Context, userID int64) (bool, error) {
	_, err := u.repo.GetUserByID(ctx, userID)
	return err == nil, nil
}

// provideMetrics creates a Prometheus metrics collector.
func provideMetrics() *metrics.Metrics {
	return metrics.NewMetrics()
}

// provideMetricsSlice wraps a single metrics collector into a slice so that
// Wire can satisfy constructors that accept variadic metrics arguments.
func provideMetricsSlice(m *metrics.Metrics) []*metrics.Metrics {
	return []*metrics.Metrics{m}
}

// provideWebSocketServer creates a WebSocket server with configurable timeouts and origin policy.
func provideWebSocketServer(handler *gateway.Handler, logger log.Logger, hb gateway.HeartbeatConfig, gatewayConf *conf.GatewayConfig) *gateway.WebSocketServer {
	allowedOrigins := []string(nil)
	if gatewayConf != nil {
		allowedOrigins = gatewayConf.AllowedOrigins
	}
	return gateway.NewWebSocketServer(handler, logger, hb.ReadTimeout, hb.WriteTimeout, allowedOrigins)
}

// provideHandler centralizes runtime-only gateway options so wire_gen.go stays generated.
func provideHandler(manager *gateway.Manager, sessions *gateway.SessionManager, messages *imapp.MessageService, query *imapp.QueryService, jwtSecret []byte, hb gateway.HeartbeatConfig, gatewayConf *conf.GatewayConfig, groups imdata.GroupRepo, logger log.Logger, m *metrics.Metrics) *gateway.Handler {
	h := gateway.NewHandler(manager, sessions, messages, query, jwtSecret, hb, logger, m)
	if gatewayConf != nil && gatewayConf.RecallWindow != nil {
		h.SetRecallWindow(gatewayConf.RecallWindow.AsDuration())
	}
	if gatewayConf != nil && gatewayConf.MsgRateLimit != nil {
		h.SetMessageRateLimit(gatewayConf.MsgRateLimit.MsgPerSec, int(gatewayConf.MsgRateLimit.Burst))
	}
	h.SetGroupMemberResolver(func(ctx context.Context, groupID string) ([]int64, error) {
		members, err := groups.ListMembers(ctx, groupID)
		if err != nil {
			return nil, err
		}
		ids := make([]int64, 0, len(members))
		for _, member := range members {
			ids = append(ids, member.UserID)
		}
		return ids, nil
	})
	return h
}

// provideAuthRateLimiter builds the auth-endpoint rate limiter on the
// shared Redis client. A missing Redis client yields a disabled limiter
// (Allow always true) so auth never hard-fails on infra misconfiguration.
func provideAuthRateLimiter(r redis.UniversalClient, auth *conf.Auth, logger log.Logger) *server.AuthRateLimiter {
	limits := server.AuthRateLimits{}
	if auth != nil && auth.GetRateLimit() != nil {
		limits.LoginIP = int64(auth.GetRateLimit().GetLoginIpPerMinute())
		limits.LoginUser = int64(auth.GetRateLimit().GetLoginUserPerMinute())
		limits.RegisterIP = int64(auth.GetRateLimit().GetRegisterIpPerMinute())
	}
	return server.NewAuthRateLimiterWithLimits(r, limits, logger)
}

type sessionPresenceChecker struct {
	sessions *gateway.SessionManager
	nodeTTL  time.Duration
}

func (p *sessionPresenceChecker) IsUserOnline(ctx context.Context, userID int64) (bool, error) {
	return p.sessions.IsUserOnline(ctx, userID, p.nodeTTL)
}

func provideUserService(uc *biz.AuthUsecase, sessions *gateway.SessionManager, cfg GatewayRegistryConfig) *service.UserService {
	svc := service.NewUserService(uc)
	svc.SetPresenceChecker(&sessionPresenceChecker{sessions: sessions, nodeTTL: cfg.TTL})
	return svc
}

// provideGatewayRegistry creates a GatewayRegistry for node heartbeat registration.
func provideGatewayRegistry(redis redis.UniversalClient, nodeID NodeID, url GatewayURL, grpcAddr GatewayGrpcAddr, cfg GatewayRegistryConfig, manager *gateway.Manager, logger log.Logger) *gateway.GatewayRegistry {
	return gateway.NewGatewayRegistry(redis, string(nodeID), string(url), string(grpcAddr), cfg.Interval, cfg.TTL, manager, logger)
}

// wireApp init kratos application.
func wireApp(*conf.Server, *conf.Data, *conf.Auth, *conf.Dispatch, *conf.GatewayConfig, *conf.Search, log.Logger) (*kratos.App, func(), error) {
	panic(wire.Build(
		server.ProviderSet,
		data.ProviderSet,
		imdata.ProviderSet,
		biz.ProviderSet,
		service.ProviderSet,
		gateway.ProviderSet,
		provideNodeIDString,
		provideNodeID,
		provideGatewayURL,
		provideGatewayGrpcAddr,
		provideGatewayRegistryConfig,
		provideGatewayRegistry,
		provideJWTSecret,
		provideHeartbeatConfig,
		provideWebSocketServer,
		provideHandler,
		provideAuthRateLimiter,
		provideUserService,
		provideRedisClient,
		provideGormDB,
		provideKafkaConfig,
		provideMongoDB,
		provideMessageStorage,
		gateway.NewDomainProducer,
		provideFileStore,
		provideIMMessageService,
		provideSearchIndex,
		provideIMQueryService,
		provideIMConversationService,
		provideIMGroupService,
		provideIMFileService,
		provideIMOutboxService,
		provideOutboxAdmin,
		provideAgentAdminService,
		provideAuditService,
		provideQuotaService,
		provideRunEventBroadcaster,
		provideAgentRunService,
		provideAgentApprovalService,
		provideAgentMCPService,
		provideAgentSkillService,
		provideModelService,
		provideBotStore,
		provideBotService,
		provideBotAuth,
		provideBotReader,
		provideBotSender,
		provideBotUsers,
		provideBotGroupMembers,
		provideBotRecipients,
		provideBotDelivery,
		provideBotStreamPublisher,
		provideBotStreamService,
		provideBotStreamStore,
		provideBotRateLimiter,
		provideBotHTTP,
		agentdata.NewStore,
		platformdata.NewStore,
		provideMetrics,
		provideMetricsSlice,
		authz.NewOpenFGAFromEnv,
		wire.Bind(new(authz.Authorizer), new(*authz.OpenFGA)),
		wire.Bind(new(authz.PlatformAuthorizer), new(*authz.OpenFGA)),
		newApp,
	))
}
