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
	"syscall"
	"time"

	"nonoka-im/internal/agent/app"
	"nonoka-im/internal/agent/credential"
	"nonoka-im/internal/agent/eino"
	agevent "nonoka-im/internal/agent/event"
	agentmcp "nonoka-im/internal/agent/mcp"
	agentruntime "nonoka-im/internal/agent/runtime"
	agentskills "nonoka-im/internal/agent/skills"
	"nonoka-im/internal/authz"
	"nonoka-im/internal/biz"
	"nonoka-im/internal/conf"
	"nonoka-im/internal/data"
	"nonoka-im/internal/msgworker"
	"nonoka-im/internal/topic"

	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/go-kratos/kratos/v2/log"
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
		if err := store.BootstrapAgent(ctx, agentID, botID, ownerID, env("AGENT_BOOTSTRAP_NAME", "Nonoka Assistant"), instruction, modelName, base, bootstrapKeyEnc, 4096, func(ctx context.Context, id int64) error { return az.SetAgentOwner(ctx, id, ownerID) }); err != nil {
			panic(err)
		}
		for _, tool := range []string{"search_messages", "get_conversation_context", "send_message", "read_file", "get_topic_info"} {
			if err := az.SetAgentToolBinding(ctx, agentID, tool, true); err != nil {
				logger.Log(log.LevelWarn, "msg", fmt.Sprintf("bind bootstrap tool %s: %v", tool, err))
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
	mcpProvider := &agentmcp.Provider{Store: data.NewMCPStore(dataLayer), Guard: az}
	rt.SetMCPToolProvider(mcpProvider.Tools)
	extStore := data.NewMCPStore(dataLayer)
	rt.SetSkillBackendFactory(func(bindings []agentruntime.SkillBinding) (skill.Backend, error) {
		return agentskills.NewBackend(extStore, bindings), nil
	})
	h := &app.Harness{Store: store, Runtime: rt, Authorizer: az, Sender: sender}
	process := func(ctx context.Context, ev agevent.MessageEvent, pinned *data.AgentRun) error {
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
			if version.MCPBindings != "" {
				var bindings []data.MCPBinding
				if err := json.Unmarshal([]byte(version.MCPBindings), &bindings); err != nil {
					return fmt.Errorf("agent %d version %d: decode mcp bindings: %w", agent.ID, version.Version, err)
				}
				for _, binding := range bindings {
					mcpTools = append(mcpTools, agentruntime.MCPTool{Server: binding.Server, Tool: binding.Tool})
				}
			}
			var skillBindings []agentruntime.SkillBinding
			if version.Skills != "" {
				if err := json.Unmarshal([]byte(version.Skills), &skillBindings); err != nil {
					return fmt.Errorf("agent %d version %d: decode skill bindings: %w", agent.ID, version.Version, err)
				}
			}
			resolved := app.ResolvedAgent{ID: agent.ID, BotID: agent.BotUserID, Version: version.Version, Instruction: version.Instruction, Model: version.Model, BaseURL: version.BaseURL, APIKey: apiKey, MaxTokens: version.MaxTokens, Tools: allowed, MCPTools: mcpTools, Skills: skillBindings, RestrictTools: true}
			if err := h.HandleResolved(ctx, ev, resolved); err != nil {
				return err
			}
		}
		return nil
	}
	go func() {
		ticker := time.NewTicker(2 * time.Second)
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
					r := runs[i]
					ev := agevent.MessageEvent{SchemaVersion: agevent.SchemaVersion, EventID: r.EventID, MsgID: r.MsgID, TopicSeq: r.TopicSeq, Topic: r.Topic, SenderID: r.ActorUserID}
					runCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
					if err := process(runCtx, ev, &r); err != nil {
						logger.Log(log.LevelError, "msg", fmt.Sprintf("queued run %d: %v", r.ID, err))
					}
					cancel()
				}
			}
		}
	}()
	brokers := []string{env("KAFKA_BROKERS", "127.0.0.1:9092")}
	consumer := msgworker.NewKafkaConsumer(msgworker.KafkaConsumerConfig{Brokers: brokers, Topic: env("AGENT_EVENTS_TOPIC", agevent.Topic), GroupID: env("AGENT_EVENTS_GROUP", "nonoka-agent-worker"), WorkerCount: int(envInt64("AGENT_WORKERS", 2)), MaxRetries: int(envInt64("AGENT_MAX_RETRIES", 2)), HandlerTimeout: 90 * time.Second, RetryBackoff: 500 * time.Millisecond}, func(ctx context.Context, _, value []byte, _ map[string]string) error {
		var ev agevent.MessageEvent
		if err := json.Unmarshal(value, &ev); err != nil {
			return err
		}
		if ev.SchemaVersion != agevent.SchemaVersion || ev.EventID == "" || ev.MsgID <= 0 {
			return fmt.Errorf("invalid agent event")
		}
		return process(ctx, ev, nil)
	}, logger)
	defer consumer.Stop()
	if err := consumer.Start(ctx); err != nil && ctx.Err() == nil {
		logger.Log(log.LevelError, "msg", fmt.Sprintf("agent consumer: %v", err))
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
