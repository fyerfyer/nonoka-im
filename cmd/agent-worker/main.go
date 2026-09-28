package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"nonoka-im/internal/agent/app"
	"nonoka-im/internal/agent/eino"
	agevent "nonoka-im/internal/agent/event"
	"nonoka-im/internal/authz"
	"nonoka-im/internal/conf"
	"nonoka-im/internal/data"
	"nonoka-im/internal/msgworker"

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
	groups := data.NewGroupRepo(dataLayer, logger)
	az := authz.NewTopicAuthorizer(groups)
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
	if modelName == "" {
		modelName = "deepseek-chat"
	}
	agentID := envInt64("AGENT_ID", 1)
	botID := envInt64("AGENT_BOT_ID", 0)
	botToken := strings.TrimSpace(os.Getenv("AGENT_BOT_TOKEN"))
	if agentID <= 0 || botID <= 0 || botToken == "" {
		panic("AGENT_ID, AGENT_BOT_ID, and AGENT_BOT_TOKEN must be configured")
	}
	sender := &app.HTTPSender{BaseURL: env("IM_HTTP_URL", "http://127.0.0.1:8000"), Token: botToken, Client: &http.Client{Timeout: 30 * time.Second}}
	rt, err := eino.NewRuntimeWithTools(ctx, apiKey, base, modelName, &app.MessageTools{Storage: messageStorage, Authorizer: az, Sender: sender})
	if err != nil {
		panic(err)
	}
	h := &app.Harness{AgentID: agentID, BotID: botID, Store: store, Runtime: rt, Authorizer: az, HistoryLimit: 8, MaxContextChars: int(envInt64("AGENT_MAX_CONTEXT_CHARS", 12000)), Sender: sender}
	brokers := []string{env("KAFKA_BROKERS", "127.0.0.1:9092")}
	consumer := msgworker.NewKafkaConsumer(msgworker.KafkaConsumerConfig{Brokers: brokers, Topic: env("AGENT_EVENTS_TOPIC", agevent.Topic), GroupID: env("AGENT_EVENTS_GROUP", "nonoka-agent-worker"), WorkerCount: int(envInt64("AGENT_WORKERS", 2)), MaxRetries: int(envInt64("AGENT_MAX_RETRIES", 2)), HandlerTimeout: 90 * time.Second, RetryBackoff: 500 * time.Millisecond}, func(ctx context.Context, _, value []byte, _ map[string]string) error {
		var ev agevent.MessageEvent
		if err := json.Unmarshal(value, &ev); err != nil {
			return err
		}
		if ev.SchemaVersion != agevent.SchemaVersion || ev.EventID == "" || ev.MsgID <= 0 {
			return fmt.Errorf("invalid agent event")
		}
		if !h.ShouldHandle(ev) {
			return nil
		}
		if err := az.CanAccessTopic(ctx, ev.SenderID, ev.Topic); err != nil {
			return fmt.Errorf("event actor topic authorization failed")
		}
		if err := az.CanAccessTopic(ctx, botID, ev.Topic); err != nil {
			return fmt.Errorf("agent bot topic authorization failed")
		}
		stored, err := messageStorage.GetMessageForUser(ctx, ev.SenderID, ev.Topic, ev.MsgID)
		if err != nil {
			return err
		}
		ev.Content = string(stored.Content)
		return h.Handle(ctx, ev)
	}, logger)
	defer consumer.Stop()
	if err := consumer.Start(ctx); err != nil && ctx.Err() == nil {
		logger.Log(log.LevelError, "msg", fmt.Sprintf("agent consumer: %v", err))
	}
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
