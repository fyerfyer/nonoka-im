package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"nonoka-im/internal/agent/app"
	"nonoka-im/internal/agent/credential"
	"nonoka-im/internal/agent/eino"
	agevent "nonoka-im/internal/agent/event"
	agentmcp "nonoka-im/internal/agent/mcp"
	agentruntime "nonoka-im/internal/agent/runtime"
	agentskills "nonoka-im/internal/agent/skills"
	agentstore "nonoka-im/internal/agent/store"
	"nonoka-im/internal/authz"
	"nonoka-im/internal/biz"
	"nonoka-im/internal/conf"
	"nonoka-im/internal/data"
	"nonoka-im/internal/msgworker"
	"nonoka-im/internal/topic"

	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv/v1.24.0"
)

func main() {
	loadDotEnv(".env")
	logger := log.NewStdLogger(os.Stdout)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Optional observability endpoints, disabled unless their address env is
	// set: AGENT_HEALTH_ADDR serves a liveness probe, AGENT_METRICS_ADDR serves
	// Prometheus metrics. Both are plain HTTP servers independent of Kafka.
	agentMetrics := newAgentWorkerMetrics(prometheus.NewRegistry())
	if addr := os.Getenv("AGENT_HEALTH_ADDR"); addr != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})
		srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
		go func() {
			logger.Log(log.LevelInfo, "msg", "agent worker health server listening on "+addr)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Log(log.LevelWarn, "msg", fmt.Sprintf("health server error: %v", err))
			}
		}()
		defer func() {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			_ = srv.Shutdown(shutdownCtx)
		}()
	}
	if addr := os.Getenv("AGENT_METRICS_ADDR"); addr != "" {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.HandlerFor(agentMetrics.registry, promhttp.HandlerOpts{}))
		srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
		go func() {
			logger.Log(log.LevelInfo, "msg", "agent worker metrics server listening on "+addr)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Log(log.LevelWarn, "msg", fmt.Sprintf("metrics server error: %v", err))
			}
		}()
		defer func() {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			_ = srv.Shutdown(shutdownCtx)
		}()
	}
	if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"); endpoint != "" {
		exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
		if err != nil {
			panic(fmt.Errorf("initialize OTLP trace exporter: %w", err))
		}
		provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter), sdktrace.WithResource(resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName("nonoka-agent-worker"))))
		otel.SetTracerProvider(provider)
		defer func() { _ = provider.Shutdown(context.Background()) }()
	}
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = "host=127.0.0.1 user=postgres password=root dbname=nonoka_im port=5432 sslmode=disable"
	}
	d := &conf.Data{Database: &conf.Data_Database{Source: dsn}}
	dataLayer, cleanup, err := data.NewData(d)
	if err != nil {
		panic(err)
	}
	defer cleanup()
	mongoURI := env("MONGODB_URI", "mongodb://127.0.0.1:27017")
	mongoDBName := env("MONGODB_DATABASE", "nonoka_im")
	mongoClient, err := mongo.Connect(options.Client().ApplyURI(mongoURI))
	if err != nil {
		panic(fmt.Errorf("connect mongo: %w", err))
	}
	defer func() { _ = mongoClient.Disconnect(context.Background()) }()
	if err := mongoClient.Ping(ctx, nil); err != nil {
		panic(fmt.Errorf("ping mongo: %w", err))
	}
	messageStorage := msgworker.NewMessageStorage(mongoClient.Database(mongoDBName), logger)
	az, err := authz.NewOpenFGAFromEnv()
	if err != nil {
		panic(err)
	}
	store := data.NewAgentStore(dataLayer)
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("DEEPSEEK_API_KEY")
	}
	if apiKey == "" {
		apiKey = os.Getenv("DEEPSEEK_KEY")
	}
	base := os.Getenv("OPENAI_BASE_URL")
	if base == "" {
		base = "https://api.deepseek.com/v1"
	}
	modelName := os.Getenv("OPENAI_MODEL")
	sealKey, err := credential.SealKeyFromEnv()
	if err != nil {
		panic(err)
	}
	agentID := envInt64("AGENT_ID", 1)
	botID := envInt64("AGENT_BOT_ID", 0)
	if ownerID := envInt64("AGENT_BOOTSTRAP_OWNER_ID", 0); ownerID > 0 {
		instruction := env("AGENT_BOOTSTRAP_INSTRUCTION", "You are the Nonoka IM assistant. Use concise, helpful answers.")
		bootstrapKeyEnc := ""
		if apiKey != "" {
			if sealed, err := credential.Seal(sealKey, []byte(apiKey)); err == nil {
				bootstrapKeyEnc = base64.StdEncoding.EncodeToString(sealed)
			}
		}
		created, err := store.BootstrapAgent(ctx, agentID, botID, ownerID, env("AGENT_BOOTSTRAP_NAME", "Nonoka Assistant"), instruction, modelName, base, bootstrapKeyEnc, 4096, func(ctx context.Context, id int64) error { return az.SetAgentOwner(ctx, id, ownerID) })
		if err != nil {
			panic(err)
		}
		if created {
			for _, tool := range []string{"search_messages", "get_conversation_context", "send_message", "read_file", "get_topic_info", "get_my_info", "list_my_groups", "get_group_members"} {
				if err := az.SetAgentToolBinding(ctx, agentID, tool, true); err != nil {
					logger.Log(log.LevelWarn, "msg", fmt.Sprintf("bind bootstrap tool %s: %v", tool, err))
				}
			}
		}
	}
	secret := []byte(os.Getenv("AGENT_WORKER_JWT_SECRET"))
	if len(secret) < 32 {
		panic("AGENT_WORKER_JWT_SECRET must contain at least 32 bytes")
	}
	if apiKey == "" {
		// The demo stack always starts this worker without a shared model key.
		// Agents that carry their own key still run; this placeholder only
		// fails for agents relying on the missing environment fallback.
		logger.Log(log.LevelWarn, "msg", "OPENAI_API_KEY/DEEPSEEK_API_KEY not set; only agents with their own API key will run")
		apiKey = "missing-model-key"
	}
	sender := &app.HTTPSender{BaseURL: env("IM_HTTP_URL", "http://127.0.0.1:8000"), Client: &http.Client{Timeout: 30 * time.Second}, TokenIssuer: func(agentID, botID int64, topic string) (string, error) {
		return credential.Sign(secret, agentID, botID, topic)
	}}
	directory := &workerDirectory{groups: data.NewGroupRepo(dataLayer, logger), users: data.NewAuthRepo(dataLayer, logger)}
	rt, err := eino.NewRuntimeWithTools(ctx, apiKey, base, modelName, &app.MessageTools{Storage: messageStorage, Authorizer: az, Sender: sender, Files: mongoClient.Database(mongoDBName), Directory: directory}, az)
	if err != nil {
		panic(err)
	}
	rt.SetCheckPointStore(&agentstore.CheckPoints{Store: store})
	mcpStore := data.NewMCPStore(dataLayer)
	mcpProvider := &agentmcp.Provider{Store: mcpStore, Guard: az}
	rt.SetMCPToolProvider(mcpProvider.Tools)
	extStore := mcpStore
	rt.SetSkillBackendFactory(func(bindings []agentruntime.SkillBinding) (skill.Backend, error) {
		return agentskills.NewBackend(extStore, bindings), nil
	})
	h := &app.Harness{Store: store, Runtime: rt, Authorizer: az, Sender: sender}

	// dispatch resolves the triggering event to candidate agents and records
	// queued runs. It only performs short database/authorization reads so the
	// Kafka consumer can commit offsets quickly; all model work happens in the
	// run worker pool via execute.
	dispatch := func(ctx context.Context, ev agevent.MessageEvent) error {
		if ev.SenderID <= 0 || ev.Topic == "" {
			return fmt.Errorf("invalid agent event: sender and topic required")
		}
		if err := az.CanAccessTopic(ctx, ev.SenderID, ev.Topic); err != nil {
			if errors.Is(err, authz.ErrDenied) {
				// The sender lost topic access; nothing to trigger.
				return nil
			}
			return fmt.Errorf("event actor topic authorization failed: %w", err)
		}
		var botIDs []int64
		switch topic.ParseType(ev.Topic) {
		case topic.TypeP2P:
			x, y, err := topic.ExtractUserIDsFromP2PTopic(ev.Topic)
			if err != nil {
				return err
			}
			if x == ev.SenderID {
				botIDs = []int64{y}
			} else if y == ev.SenderID {
				botIDs = []int64{x}
			}
		case topic.TypeGroup:
			botIDs = ev.MentionedAgentIDs
		}
		agents, err := store.ActiveAgentsByBot(ctx, botIDs)
		if err != nil {
			return err
		}
		if len(agents) == 0 {
			return nil
		}
		stored, err := messageStorage.GetMessageForUser(ctx, ev.SenderID, ev.Topic, ev.MsgID)
		if err != nil {
			return err
		}
		ev.Content = string(stored.Content)
		for _, agent := range agents {
			if ev.SenderID == agent.BotUserID {
				continue
			}
			// Per-agent authorization (CanUseAgent, bot topic access) is checked
			// in execute, which marks denied runs cancelled with access_revoked.
			if _, err := store.QueueRun(ctx, &data.AgentRun{EventID: ev.EventID, AgentID: agent.ID, AgentVersion: agent.CurrentVersion, ActorUserID: ev.SenderID, Topic: ev.Topic, MsgID: ev.MsgID, TopicSeq: ev.TopicSeq, Status: "queued", CreatedAt: time.Now()}); err != nil {
				return err
			}
		}
		return nil
	}

	// execute runs a single queued run: re-checks authorization with the pinned
	// run version, loads the message and delegates to the harness.
	execute := func(ctx context.Context, ev agevent.MessageEvent, pinned *data.AgentRun) error {
		if err := az.CanAccessTopic(ctx, ev.SenderID, ev.Topic); err != nil {
			if pinned != nil && errors.Is(err, authz.ErrDenied) {
				return store.UpdateRun(ctx, pinned.ID, map[string]any{"status": "cancelled", "error_code": "access_revoked", "finished_at": time.Now()})
			}
			return fmt.Errorf("event actor topic authorization failed: %w", err)
		}
		var agents []data.Agent
		if pinned != nil {
			agent, err := store.GetAgent(ctx, pinned.AgentID)
			if err != nil {
				return err
			}
			if agent.Status != "active" {
				return store.UpdateRun(ctx, pinned.ID, map[string]any{"status": "cancelled", "finished_at": time.Now()})
			}
			agents = []data.Agent{*agent}
		} else {
			var botIDs []int64
			switch topic.ParseType(ev.Topic) {
			case topic.TypeP2P:
				x, y, err := topic.ExtractUserIDsFromP2PTopic(ev.Topic)
				if err != nil {
					return err
				}
				if x == ev.SenderID {
					botIDs = []int64{y}
				} else if y == ev.SenderID {
					botIDs = []int64{x}
				}
			case topic.TypeGroup:
				botIDs = ev.MentionedAgentIDs
			}
			var err error
			agents, err = store.ActiveAgentsByBot(ctx, botIDs)
			if err != nil {
				return err
			}
		}
		if len(agents) == 0 {
			return nil
		}
		stored, err := messageStorage.GetMessageForUser(ctx, ev.SenderID, ev.Topic, ev.MsgID)
		if err != nil {
			return err
		}
		ev.Content = string(stored.Content)
		for _, agent := range agents {
			if ev.SenderID == agent.BotUserID {
				continue
			}
			if err := az.CanUseAgent(ctx, ev.SenderID, agent.ID); err != nil {
				if pinned != nil && errors.Is(err, authz.ErrDenied) {
					return store.UpdateRun(ctx, pinned.ID, map[string]any{"status": "cancelled", "error_code": "access_revoked", "finished_at": time.Now()})
				}
				if !errors.Is(err, authz.ErrDenied) {
					return err
				}
				continue
			}
			if err := az.CanAccessTopic(ctx, agent.BotUserID, ev.Topic); err != nil {
				if pinned != nil && errors.Is(err, authz.ErrDenied) {
					return store.UpdateRun(ctx, pinned.ID, map[string]any{"status": "cancelled", "error_code": "access_revoked", "finished_at": time.Now()})
				}
				if !errors.Is(err, authz.ErrDenied) {
					return err
				}
				continue
			}
			versionNumber := agent.CurrentVersion
			if pinned != nil {
				versionNumber = pinned.AgentVersion
			}
			version, err := store.GetVersion(ctx, agent.ID, versionNumber)
			if err != nil {
				return err
			}
			var allowed []string
			if err := json.Unmarshal([]byte(version.Tools), &allowed); err != nil {
				return err
			}
			apiKey := ""
			if version.APIKeyEnc != "" {
				raw, err := base64.StdEncoding.DecodeString(version.APIKeyEnc)
				if err != nil {
					return fmt.Errorf("agent %d version %d: decode sealed key: %w", agent.ID, version.Version, err)
				}
				plain, err := credential.Open(sealKey, raw)
				if err != nil {
					return fmt.Errorf("agent %d version %d: unseal model key: %w", agent.ID, version.Version, err)
				}
				apiKey = string(plain)
			}
			var mcpTools []agentruntime.MCPTool
			var mcpBindings []data.MCPBinding
			if version.MCPBindings != "" {
				if err := json.Unmarshal([]byte(version.MCPBindings), &mcpBindings); err != nil {
					return fmt.Errorf("agent %d version %d: decode mcp bindings: %w", agent.ID, version.Version, err)
				}
				for _, binding := range mcpBindings {
					mcpTools = append(mcpTools, agentruntime.MCPTool{Server: binding.Server, Tool: binding.Tool})
				}
			}
			var skillBindings []agentruntime.SkillBinding
			if version.Skills != "" {
				if err := json.Unmarshal([]byte(version.Skills), &skillBindings); err != nil {
					return fmt.Errorf("agent %d version %d: decode skill bindings: %w", agent.ID, version.Version, err)
				}
			}
			// Write-class MCP tools interrupt for human confirmation before
			// executing; read-only tools (and local tools) run directly.
			var approvalTools []string
			for _, binding := range mcpBindings {
				server, err := mcpStore.GetServer(ctx, binding.Server)
				if err != nil {
					return fmt.Errorf("agent %d version %d: load mcp server %q: %w", agent.ID, version.Version, binding.Server, err)
				}
				if server.RiskClass == "write" {
					approvalTools = append(approvalTools, binding.Exposed)
				}
			}
			resolved := app.ResolvedAgent{ID: agent.ID, BotID: agent.BotUserID, Version: version.Version, Instruction: version.Instruction, Model: version.Model, BaseURL: version.BaseURL, APIKey: apiKey, MaxTokens: version.MaxTokens, Tools: allowed, MCPTools: mcpTools, Skills: skillBindings, RestrictTools: true, ApprovalTools: approvalTools}
			// A run carrying a checkpoint is resuming an interrupted approval
			// flow: claim it atomically and continue from the checkpoint
			// instead of starting a fresh model run.
			if pinned != nil && pinned.CheckpointID != "" {
				_, claimed, err := store.ClaimRun(ctx, pinned.EventID, &data.AgentRun{EventID: pinned.EventID, AgentID: pinned.AgentID})
				if err != nil {
					return err
				}
				if !claimed {
					return nil
				}
				// ClaimRun leaves started_at untouched; set it so the stale-run
				// sweeper measures this execution, not the original enqueue.
				if err := store.UpdateRun(ctx, pinned.ID, map[string]any{"started_at": time.Now()}); err != nil {
					return err
				}
				if blocker, err := store.BlockingRun(ctx, pinned.SessionID, pinned.TopicSeq, pinned.ID); err != nil {
					return err
				} else if blocker != nil {
					return store.UpdateRun(ctx, pinned.ID, map[string]any{"status": "queued", "started_at": nil})
				}
				approval, err := store.ApprovalForRun(ctx, pinned.ID)
				if err != nil {
					return err
				}
				if approval == nil {
					return store.UpdateRun(ctx, pinned.ID, map[string]any{"status": "failed", "error_code": "approval_missing", "finished_at": time.Now()})
				}
				if approval.Status == "pending" {
					// Not decided yet; put the run back to waiting.
					return store.UpdateRun(ctx, pinned.ID, map[string]any{"status": "waiting_approval", "started_at": nil})
				}
				return h.HandleResume(ctx, ev, resolved, pinned, approval)
			}
			if err := h.HandleResolved(ctx, ev, resolved); err != nil {
				return err
			}
		}
		return nil
	}

	runTimeout := envDuration("AGENT_RUN_TIMEOUT", 120*time.Second)
	replayMaxFailures := int(envInt64("AGENT_REPLAY_MAX_FAILURES", 10))
	failCounts := make(map[uint64]int)
	var failMu sync.Mutex
	noteFailure := func(id uint64) int {
		failMu.Lock()
		defer failMu.Unlock()
		failCounts[id]++
		return failCounts[id]
	}
	clearFailures := func(id uint64) {
		failMu.Lock()
		delete(failCounts, id)
		failMu.Unlock()
	}
	executeRun := func(r data.AgentRun) {
		start := time.Now()
		defer func() {
			agentMetrics.runDuration.Observe(time.Since(start).Seconds())
			if p := recover(); p != nil {
				agentMetrics.runsTotal.WithLabelValues("panic").Inc()
				logger.Log(log.LevelError, "msg", fmt.Sprintf("execute run %d panic: %v", r.ID, p))
			}
		}()
		ev := agevent.MessageEvent{SchemaVersion: agevent.SchemaVersion, EventID: r.EventID, MsgID: r.MsgID, TopicSeq: r.TopicSeq, Topic: r.Topic, SenderID: r.ActorUserID}
		runCtx, cancel := context.WithTimeout(ctx, runTimeout)
		defer cancel()
		if err := execute(runCtx, ev, &r); err != nil {
			agentMetrics.runsTotal.WithLabelValues("failure").Inc()
			logger.Log(log.LevelError, "msg", fmt.Sprintf("queued run %d: %v", r.ID, err))
			if noteFailure(r.ID) >= replayMaxFailures {
				fctx, fcancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
				if uerr := store.UpdateRun(fctx, r.ID, map[string]any{"status": "failed", "error_code": "replay_exhausted", "finished_at": time.Now()}); uerr != nil {
					logger.Log(log.LevelError, "msg", fmt.Sprintf("mark run %d failed: %v", r.ID, uerr))
				}
				fcancel()
				clearFailures(r.ID)
			}
			return
		}
		agentMetrics.runsTotal.WithLabelValues("success").Inc()
		clearFailures(r.ID)
	}

	runWorkers := int(envInt64("AGENT_RUN_WORKERS", 8))
	if runWorkers < 1 {
		runWorkers = 1
	}
	work := make(chan data.AgentRun, 64)
	for w := 0; w < runWorkers; w++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case r, ok := <-work:
					if !ok {
						return
					}
					executeRun(r)
				}
			}
		}()
	}

	// Queued-run replay: pull dispatch-ordered runs and hand them to the worker
	// pool. Per-session ordering is enforced by QueuedRuns/BlockingRun plus the
	// session advisory lock, so batch items can execute concurrently.
	go func() {
		defer func() {
			if p := recover(); p != nil {
				logger.Log(log.LevelError, "msg", fmt.Sprintf("queued-runs loop panic: %v", p))
			}
		}()
		ticker := time.NewTicker(envDuration("AGENT_QUEUED_POLL", time.Second))
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runs, err := store.QueuedRuns(ctx, 32)
				if err != nil {
					logger.Log(log.LevelError, "msg", fmt.Sprintf("load queued runs: %v", err))
					continue
				}
				for i := range runs {
					select {
					case work <- runs[i]:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	// Stale-run sweeper: a worker that dies mid-run leaves a "running" row that
	// would otherwise block every later run in the session forever.
	go func() {
		defer func() {
			if p := recover(); p != nil {
				logger.Log(log.LevelError, "msg", fmt.Sprintf("stale-run sweeper panic: %v", p))
			}
		}()
		ticker := time.NewTicker(envDuration("AGENT_SWEEP_INTERVAL", 30*time.Second))
		defer ticker.Stop()
		staleAfter := envDuration("AGENT_STALE_RUN_TIMEOUT", 10*time.Minute)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sctx, scancel := context.WithTimeout(ctx, 15*time.Second)
				sweepStart := time.Now()
				requeued, failed, err := store.SweepStaleRuns(sctx, staleAfter, int(envInt64("AGENT_MAX_ATTEMPTS", 5)))
				agentMetrics.sweepsTotal.Inc()
				agentMetrics.sweepDuration.Observe(time.Since(sweepStart).Seconds())
				if err != nil {
					logger.Log(log.LevelError, "msg", fmt.Sprintf("sweep stale runs: %v", err))
				} else {
					agentMetrics.staleRunsRequeuedTotal.Add(float64(requeued))
					agentMetrics.staleRunsFailedTotal.Add(float64(failed))
					if requeued > 0 || failed > 0 {
						logger.Log(log.LevelInfo, "msg", fmt.Sprintf("swept stale runs: requeued=%d failed=%d", requeued, failed))
					}
				}
				// Expired approvals resolve as rejections: requeue the waiting
				// run so the resume path feeds the model the rejection result.
				runIDs, err := store.ExpirePendingApprovals(sctx, time.Now())
				if err != nil {
					logger.Log(log.LevelError, "msg", fmt.Sprintf("expire pending approvals: %v", err))
				}
				for _, runID := range runIDs {
					if err := store.ClaimWaitingRun(sctx, runID); err != nil && !errors.Is(err, data.ErrRunState) {
						logger.Log(log.LevelError, "msg", fmt.Sprintf("requeue run %d after approval expiry: %v", runID, err))
					}
				}
				if pruned, err := store.PruneExpiredCheckpoints(sctx, time.Now(), 500); err != nil {
					logger.Log(log.LevelError, "msg", fmt.Sprintf("prune checkpoints: %v", err))
				} else if pruned > 0 {
					logger.Log(log.LevelInfo, "msg", fmt.Sprintf("pruned %d expired checkpoints", pruned))
				}
				scancel()
			}
		}
	}()

	brokers := strings.Split(env("KAFKA_BROKERS", "127.0.0.1:9092"), ",")
	consumer := msgworker.NewKafkaConsumer(msgworker.KafkaConsumerConfig{Brokers: brokers, Topic: env("AGENT_EVENTS_TOPIC", agevent.Topic), GroupID: env("AGENT_EVENTS_GROUP", "nonoka-agent-worker"), WorkerCount: int(envInt64("AGENT_WORKERS", 4)), MaxRetries: int(envInt64("AGENT_MAX_RETRIES", 2)), HandlerTimeout: envDuration("AGENT_DISPATCH_TIMEOUT", 10*time.Second), RetryBackoff: 500 * time.Millisecond, DLQTopic: env("AGENT_EVENTS_DLQ_TOPIC", "agent-events-dlq")}, func(ctx context.Context, _, value []byte, _ map[string]string) error {
		var ev agevent.MessageEvent
		if err := json.Unmarshal(value, &ev); err != nil {
			return err
		}
		if ev.SchemaVersion != agevent.SchemaVersion || ev.EventID == "" || ev.MsgID <= 0 {
			return fmt.Errorf("invalid agent event")
		}
		return dispatch(ctx, ev)
	}, logger)
	defer consumer.Stop()
	if err := consumer.Start(ctx); err != nil && ctx.Err() == nil {
		logger.Log(log.LevelError, "msg", fmt.Sprintf("agent consumer: %v", err))
		os.Exit(1)
	}
}

// workerDirectory adapts the PostgreSQL repositories to the app.Directory
// interface used by the get_topic_info tool.
type workerDirectory struct {
	groups data.GroupRepo
	users  biz.AuthRepo
}

func (d *workerDirectory) Username(ctx context.Context, id int64) (string, error) {
	user, err := d.users.GetUserByID(ctx, id)
	if err != nil {
		return fmt.Sprintf("user-%d", id), nil
	}
	return user.Username, nil
}

func (d *workerDirectory) GroupName(ctx context.Context, groupID string) (string, error) {
	group, err := d.groups.GetGroup(ctx, groupID)
	if err != nil {
		return "", err
	}
	return group.Name, nil
}

func (d *workerDirectory) GroupMemberIDs(ctx context.Context, groupID string) ([]int64, error) {
	members, err := d.groups.ListMembers(ctx, groupID)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.UserId)
	}
	return ids, nil
}

// UserProfile returns the actor-facing profile fields of one account.
func (d *workerDirectory) UserProfile(ctx context.Context, id int64) (app.UserProfile, error) {
	user, err := d.users.GetUserByID(ctx, id)
	if err != nil {
		return app.UserProfile{}, err
	}
	return app.UserProfile{ID: user.ID, Username: user.Username, CreatedAt: user.CreatedAt.Unix()}, nil
}

// MyGroups lists the user's groups with member counts so list_my_groups can
// render them without the model needing extra lookups.
func (d *workerDirectory) MyGroups(ctx context.Context, userID int64) ([]app.GroupRef, error) {
	groups, err := d.groups.ListMyGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	refs := make([]app.GroupRef, 0, len(groups))
	for _, g := range groups {
		members, err := d.groups.ListMembers(ctx, g.GroupID)
		if err != nil {
			return nil, err
		}
		refs = append(refs, app.GroupRef{ID: g.GroupID, Name: g.Name, MemberCount: len(members)})
	}
	return refs, nil
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func envInt64(k string, d int64) int64 {
	v, err := strconv.ParseInt(os.Getenv(k), 10, 64)
	if err != nil {
		return d
	}
	return v
}

func envDuration(k string, d time.Duration) time.Duration {
	v, err := time.ParseDuration(os.Getenv(k))
	if err != nil || v <= 0 {
		return d
	}
	return v
}

// agentWorkerMetrics holds the Prometheus collectors for the agent worker.
// Metric names follow the nonoka_<subsystem>_<name> style used by
// internal/metrics for the gateway and msgworker.
type agentWorkerMetrics struct {
	registry *prometheus.Registry

	runsTotal              *prometheus.CounterVec
	runDuration            prometheus.Histogram
	sweepsTotal            prometheus.Counter
	sweepDuration          prometheus.Histogram
	staleRunsRequeuedTotal prometheus.Counter
	staleRunsFailedTotal   prometheus.Counter
}

func newAgentWorkerMetrics(registry *prometheus.Registry) *agentWorkerMetrics {
	m := &agentWorkerMetrics{
		registry: registry,
		runsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "nonoka",
			Subsystem: "agent_worker",
			Name:      "runs_total",
			Help:      "Total number of queued agent runs executed.",
		}, []string{"result"}),
		runDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "nonoka",
			Subsystem: "agent_worker",
			Name:      "run_duration_seconds",
			Help:      "Duration of executing a queued agent run.",
			Buckets:   prometheus.ExponentialBuckets(0.1, 2, 12),
		}),
		sweepsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "nonoka",
			Subsystem: "agent_worker",
			Name:      "sweeps_total",
			Help:      "Total number of stale-run sweeper passes.",
		}),
		sweepDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "nonoka",
			Subsystem: "agent_worker",
			Name:      "sweep_duration_seconds",
			Help:      "Duration of a stale-run sweep pass.",
			Buckets:   prometheus.ExponentialBuckets(0.001, 2, 12),
		}),
		staleRunsRequeuedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "nonoka",
			Subsystem: "agent_worker",
			Name:      "stale_runs_requeued_total",
			Help:      "Total number of stale runs requeued by the sweeper.",
		}),
		staleRunsFailedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "nonoka",
			Subsystem: "agent_worker",
			Name:      "stale_runs_failed_total",
			Help:      "Total number of stale runs marked failed by the sweeper.",
		}),
	}
	registry.MustRegister(
		m.runsTotal,
		m.runDuration,
		m.sweepsTotal,
		m.sweepDuration,
		m.staleRunsRequeuedTotal,
		m.staleRunsFailedTotal,
	)
	return m
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
