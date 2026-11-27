package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"nonoka-im/internal/alert"
	"nonoka-im/internal/conf"
	"nonoka-im/internal/data"
	imdata "nonoka-im/internal/data/im"
	"nonoka-im/internal/im/app"
	imconsumer "nonoka-im/internal/im/consumer"
	imgroup "nonoka-im/internal/im/group"
	impush "nonoka-im/internal/im/push"
	"nonoka-im/internal/im/search"
	"nonoka-im/internal/im/seq"
	mongostore "nonoka-im/internal/im/storage/mongo"
	kafkax "nonoka-im/internal/kafka"
	messagingkafka "nonoka-im/internal/messaging/kafka"
	"nonoka-im/internal/metrics"
	"nonoka-im/internal/realtime"
	"nonoka-im/internal/server"

	"github.com/go-kratos/kratos/v2"
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
		logger.Log(log.LevelFatal, "msg", "data config is required")
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	mongoClient, err := connectMongo(ctx, bc.Data)
	if err != nil {
		logger.Log(log.LevelFatal, "msg", err.Error())
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = mongoClient.Disconnect(shutdownCtx)
	}()
	mongoDB := mongoClient.Database(bc.Data.Mongodb.Database)

	m := metrics.NewMetrics()
	storage := mongostore.NewStore(mongoDB, logger, m)
	if bc.Data.Mongodb.WriteConcern != "" {
		storage.SetWriteConcern(bc.Data.Mongodb.WriteConcern)
	}
	var searchClient *search.Client
	if search.IsMeilisearch(bc.Search) {
		searchClient = search.FromConfig(bc.Search)
		storage.SetSearchEnabled(true)
		if err := searchClient.EnsureIndex(ctx); err != nil {
			logger.Log(log.LevelWarn, "msg", fmt.Sprintf("search index setup deferred: %v", err))
		}
	}
	if err := storage.EnsureIndexes(ctx); err != nil {
		logger.Log(log.LevelFatal, "msg", fmt.Sprintf("ensure indexes: %v", err))
		os.Exit(1)
	}

	gatewayAddr := os.Getenv("GATEWAY_GRPC_ADDR")
	if gatewayAddr == "" && bc.Server != nil && bc.Server.Grpc != nil {
		gatewayAddr = bc.Server.Grpc.Addr
	}
	gatewayAddrs := make([]string, 0, 2)
	for _, addr := range strings.Split(gatewayAddr, ",") {
		if addr = strings.TrimSpace(addr); addr != "" {
			gatewayAddrs = append(gatewayAddrs, normalizePushAddr(addr))
		}
	}
	if len(gatewayAddrs) == 0 {
		gatewayAddrs = []string{"127.0.0.1:19000"}
	}
	rawPusher, err := realtime.NewGatewayPusher(gatewayAddrs, logger, m)
	if err != nil {
		logger.Log(log.LevelFatal, "msg", fmt.Sprintf("gateway pusher init failed: %v", err))
		os.Exit(1)
	}
	nodeTTL := 30 * time.Second
	if bc.Dispatch != nil && bc.Dispatch.NodeTtl != nil {
		nodeTTL = bc.Dispatch.NodeTtl.AsDuration()
	}
	router := realtime.NewGatewayRouter(redisClient, nodeTTL, logger)
	if bc.Msgworker != nil && bc.Msgworker.SessionCacheTtl != nil {
		router.SetSessionCacheTTL(bc.Msgworker.SessionCacheTtl.AsDuration())
	}
	if err := router.Start(ctx); err != nil {
		logger.Log(log.LevelFatal, "msg", fmt.Sprintf("router start: %v", err))
		os.Exit(1)
	}
	rawPusher.SetRouter(router)
	pusher := impush.NewPusher(rawPusher)

	seqGen := seq.New(redisClient, logger)
	seqGen.SetFloor(func(ctx context.Context, topic string) (uint64, error) {
		return storage.GetTopicMaxSeq(ctx, topic)
	})
	seqGen.SetAllocator(storage)
	storage.SetRedis(redisClient)

	nodeID := int64(1)
	var lease *seq.NodeLease
	if raw := strings.TrimSpace(os.Getenv("MSG_ID_NODE_ID")); raw != "" {
		if parsed, parseErr := strconv.ParseInt(raw, 10, 64); parseErr == nil && parsed >= 0 && parsed < 1024 {
			nodeID = parsed
		} else {
			logger.Log(log.LevelFatal, "msg", fmt.Sprintf("invalid MSG_ID_NODE_ID %q", raw))
			os.Exit(1)
		}
	} else {
		var leaseErr error
		lease, leaseErr = seq.AcquireNodeID(ctx, redisClient, fmt.Sprintf("%s:%d", mustHostname(), os.Getpid()), logger)
		if leaseErr != nil {
			logger.Log(log.LevelFatal, "msg", fmt.Sprintf("allocate snowflake node id: %v", leaseErr))
			os.Exit(1)
		}
		nodeID = lease.ID
		go lease.KeepAlive(ctx, 5*time.Minute)
		defer lease.Release(context.Background())
	}
	ids := seq.NewIDs(nodeID)
	conversations := imdata.NewConversationRepo(dataLayer.DB(), logger)
	groupMembers := imgroup.NewService(imdata.NewGroupMemberRepo(dataLayer.DB(), logger), redisClient, logger)
	botIndex := imgroup.NewBotIndexWithQuery(dataLayer.ListBotUserIDs, dataLayer.IsBotUser, logger)

	messages := &app.MessageService{
		Writer: storage, IDs: ids, Seqs: seqGen, Conversations: conversations,
		GroupMembers: groupMembers, Bots: botIndex, AgentEvents: true,
		Metrics: m, Log: log.NewHelper(logger),
	}
	retryQueue := impush.NewRetryQueue(redisClient, pusher, logger)
	push := &app.PushService{
		Pusher: pusher, RetryQueue: retryQueue, GroupMembers: groupMembers, Bots: botIndex,
		Metrics: m, Log: log.NewHelper(logger),
	}
	if bc.Msgworker != nil {
		push.ConfigureReceipts(int(bc.Msgworker.ReceiptBatchSize), bc.Msgworker.GetReceiptBatchTimeout().AsDuration())
	}

	eventBrokers := kafkaBrokers(bc.Data)
	agentPublisher := messagingkafka.NewPublisher(eventBrokers, os.Getenv("AGENT_EVENTS_TOPIC"))
	defer agentPublisher.Close()
	outbox := &app.OutboxService{Repo: storage, Publisher: agentPublisher, Metrics: m, Log: log.NewHelper(logger)}
	if url := strings.TrimSpace(os.Getenv("AGENT_OUTBOX_ALERT_WEBHOOK")); url != "" {
		outbox.Notifier = alert.NewWebhook(url)
	}
	if raw := strings.TrimSpace(os.Getenv("AGENT_OUTBOX_ALERT_MIN_INTERVAL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil {
			outbox.AlertMinInterval = d
		}
	}
	if raw := strings.TrimSpace(os.Getenv("AGENT_OUTBOX_ALERT_MAX_AGE")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil {
			outbox.AlertMaxAge = d
		}
	}

	consumerCfg := kafkaConsumerConfig(bc.Data.Kafka)
	consumer := kafkax.NewConsumer(consumerCfg, nil, logger)
	worker := imconsumer.NewWorker(consumer, messages, push, outbox, logger, m)
	var searchRelay *search.Relay
	if searchClient != nil && searchClient.Enabled() {
		searchRelay = &search.Relay{
			Repo: storage, Index: searchClient, Log: log.NewHelper(logger),
			BatchSize: search.RelayBatchSize(bc.Search),
			Stats: func(pending, failed int64, age time.Duration) {
				m.SetSearchIndexStats(pending, failed, age.Seconds())
			},
			OnError: m.IncSearchIndexApplyFailure,
		}
	}
	worker.Background = []func(context.Context){
		func(runCtx context.Context) { botIndex.Start(runCtx, 30*time.Second) },
		func(runCtx context.Context) { retryQueue.Start(runCtx) },
	}
	if searchRelay != nil {
		worker.Background = append(worker.Background, func(runCtx context.Context) {
			searchRelay.Run(runCtx, search.RelayInterval(bc.Search))
		})
	}
	probeAddr := os.Getenv("METRICS_ADDR")
	if probeAddr == "" && bc.Observability != nil {
		probeAddr = bc.Observability.MetricsAddr
	}
	if probeAddr == "" {
		probeAddr = "0.0.0.0:18001"
	}
	probe := server.NewProbeServer("msgworker", probeAddr, m.Handler(), func(ctx context.Context) error {
		if lease != nil {
			select {
			case <-lease.Lost():
				return fmt.Errorf("snowflake node lease lost")
			default:
			}
		}
		if !worker.Ready() {
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
	}, logger)

	application := kratos.New(
		kratos.Name("msgworker"),
		kratos.Logger(logger),
		kratos.Server(worker, probe),
	)
	if lease != nil {
		// A lost lease means another process may reuse this node ID. Stop
		// generating IDs immediately instead of risking silent collisions.
		go func() {
			select {
			case <-lease.Lost():
				logger.Log(log.LevelError, "msg", "snowflake node lease lost; stopping msgworker")
				_ = application.Stop()
			case <-ctx.Done():
			}
		}()
	}
	if err := application.Run(); err != nil {
		logger.Log(log.LevelFatal, "msg", fmt.Sprintf("msgworker error: %v", err))
		os.Exit(1)
	}
}

func connectMongo(ctx context.Context, c *conf.Data) (*mongo.Client, error) {
	if c == nil || c.Mongodb == nil || c.Mongodb.Uri == "" {
		return nil, fmt.Errorf("mongodb config is required")
	}
	client, err := mongo.Connect(options.Client().ApplyURI(c.Mongodb.Uri))
	if err != nil {
		return nil, fmt.Errorf("connect mongodb: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("ping mongodb: %w", err)
	}
	return client, nil
}

func kafkaBrokers(c *conf.Data) []string {
	if raw := os.Getenv("KAFKA_BROKERS"); raw != "" {
		if parsed := parseBrokerList(raw); len(parsed) > 0 {
			return parsed
		}
	}
	if c != nil && c.Kafka != nil && len(c.Kafka.Brokers) > 0 {
		return c.Kafka.Brokers
	}
	return []string{"127.0.0.1:9092"}
}

func parseBrokerList(raw string) []string {
	var out []string
	for _, broker := range strings.Split(raw, ",") {
		if broker = strings.TrimSpace(broker); broker != "" {
			out = append(out, broker)
		}
	}
	return out
}

func kafkaConsumerConfig(kc *conf.Data_Kafka) kafkax.ConsumerConfig {
	if kc == nil {
		return kafkax.ConsumerConfig{}
	}
	cfg := kafkax.ConsumerConfig{
		Brokers: kc.GetBrokers(), Topic: kc.GetTopic(), GroupID: kc.GetConsumerGroup(),
		WorkerCount: int(kc.GetWorkerCount()), MinBytes: int(kc.GetMinBytes()), MaxBytes: int(kc.GetMaxBytes()),
		MaxRetries: int(kc.GetMaxRetries()), DLQTopic: kc.GetDlqTopic(),
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
	if cfg.Topic == "" {
		cfg.Topic = "im-messages"
	}
	if cfg.GroupID == "" {
		cfg.GroupID = "msgworker-group"
	}
	return cfg
}

func mustHostname() string {
	h, _ := os.Hostname()
	if h == "" {
		return "msgworker-0"
	}
	return h
}

// normalizePushAddr turns a server listen address into a dialable one. The
// configured grpc.addr is 0.0.0.0:9002 for the gateway, which cannot be used
// as a dial target by a co-located worker.
func normalizePushAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		return net.JoinHostPort("127.0.0.1", port)
	}
	return addr
}
