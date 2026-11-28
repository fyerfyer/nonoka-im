package bootstrap

import (
	"os"
	"strconv"
	"strings"
	"time"

	"nonoka-im/internal/conf"

	"google.golang.org/protobuf/types/known/durationpb"
)

// Secrets stays environment-only so credentials never land in YAML.
type Secrets struct {
	JWTSecret []byte
	SecretKey string
	ModelKey  string
}

// ApplyEnv overlays container/deployment environment variables on the YAML
// configuration. Secrets are returned separately.
func ApplyEnv(bc *conf.Bootstrap) Secrets {
	ensureData(bc)
	if bc.AgentWorker == nil {
		bc.AgentWorker = &conf.AgentWorker{}
	}
	if bc.Observability == nil {
		bc.Observability = &conf.Observability{}
	}
	aw := bc.AgentWorker

	setString(&bc.Data.Database.Source, "POSTGRES_DSN")
	setString(&bc.Data.Mongodb.Uri, "MONGODB_URI")
	setString(&bc.Data.Mongodb.Database, "MONGODB_DATABASE")
	if bc.Data.Redis == nil {
		bc.Data.Redis = &conf.Data_Redis{}
	}
	setString(&bc.Data.Redis.Addr, "REDIS_ADDR")
	if brokers := splitList(os.Getenv("KAFKA_BROKERS")); len(brokers) > 0 {
		bc.Data.Kafka.Brokers = brokers
	}
	setString(&bc.Data.Kafka.Topic, "AGENT_EVENTS_TOPIC")
	setString(&bc.Data.Kafka.ConsumerGroup, "AGENT_EVENTS_GROUP")
	setString(&bc.Data.Kafka.DlqTopic, "AGENT_EVENTS_DLQ_TOPIC")
	setInt32(&bc.Data.Kafka.WorkerCount, "AGENT_WORKERS")
	setInt32(&bc.Data.Kafka.MaxRetries, "AGENT_MAX_RETRIES")

	setInt32(&aw.RunWorkers, "AGENT_RUN_WORKERS")
	setDuration(&aw.DispatchTimeout, "AGENT_DISPATCH_TIMEOUT")
	setDuration(&aw.RunTimeout, "AGENT_RUN_TIMEOUT")
	setDuration(&aw.Lease, "AGENT_RUN_LEASE")
	setDuration(&aw.Heartbeat, "AGENT_HEARTBEAT")
	setDuration(&aw.QueuedPoll, "AGENT_QUEUED_POLL")
	setDuration(&aw.SweepInterval, "AGENT_SWEEP_INTERVAL")
	setInt32(&aw.MaxAttempts, "AGENT_MAX_ATTEMPTS")
	setString(&aw.ImHttpUrl, "IM_HTTP_URL")
	setInt64(&aw.BootstrapAgentId, "AGENT_ID")
	setInt64(&aw.BootstrapBotId, "AGENT_BOT_ID")
	setInt64(&aw.BootstrapOwnerId, "AGENT_BOOTSTRAP_OWNER_ID")
	setString(&aw.BootstrapName, "AGENT_BOOTSTRAP_NAME")
	setString(&aw.BootstrapInstruction, "AGENT_BOOTSTRAP_INSTRUCTION")
	setString(&aw.BootstrapModel, "BIFROST_DEFAULT_MODEL")

	setString(&bc.Observability.HealthAddr, "AGENT_HEALTH_ADDR")
	setString(&bc.Observability.MetricsAddr, "AGENT_METRICS_ADDR")
	setString(&bc.Observability.OtlpTracesEndpoint, "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")

	return Secrets{
		JWTSecret: []byte(os.Getenv("AGENT_WORKER_JWT_SECRET")),
		SecretKey: os.Getenv("AGENT_SECRET_KEY"),
		ModelKey:  firstEnv("OPENAI_API_KEY", "DEEPSEEK_API_KEY", "DEEPSEEK_KEY"),
	}
}

// Normalize fills safe defaults and validates required values.
func Normalize(bc *conf.Bootstrap, secrets Secrets) error {
	ensureData(bc)
	if bc.AgentWorker == nil {
		bc.AgentWorker = &conf.AgentWorker{}
	}
	aw := bc.AgentWorker
	if len(bc.Data.Kafka.Brokers) == 0 {
		bc.Data.Kafka.Brokers = []string{"127.0.0.1:9092"}
	}
	if bc.Data.Kafka.Topic == "" {
		bc.Data.Kafka.Topic = "agent-events"
	}
	if bc.Data.Kafka.ConsumerGroup == "" {
		bc.Data.Kafka.ConsumerGroup = "nonoka-agent-worker"
	}
	if aw.RunWorkers < 1 {
		aw.RunWorkers = 8
	}
	if bc.Data.Kafka.WorkerCount < 1 {
		bc.Data.Kafka.WorkerCount = 1
	}
	if aw.MaxAttempts < 1 {
		aw.MaxAttempts = 5
	}
	if aw.GetRunTimeout().AsDuration() <= 0 {
		aw.RunTimeout = durationpb.New(2 * time.Minute)
	}
	if aw.GetLease().AsDuration() <= aw.GetRunTimeout().AsDuration() {
		aw.Lease = durationpb.New(aw.GetRunTimeout().AsDuration() + time.Minute)
	}
	if aw.GetHeartbeat().AsDuration() <= 0 {
		aw.Heartbeat = durationpb.New(aw.GetLease().AsDuration() / 3)
	}
	if aw.GetQueuedPoll().AsDuration() <= 0 {
		aw.QueuedPoll = durationpb.New(time.Second)
	}
	if aw.GetSweepInterval().AsDuration() <= 0 {
		aw.SweepInterval = durationpb.New(30 * time.Second)
	}
	if aw.GetDispatchTimeout().AsDuration() <= 0 {
		aw.DispatchTimeout = durationpb.New(10 * time.Second)
	}
	if aw.ImHttpUrl == "" {
		aw.ImHttpUrl = "http://127.0.0.1:8000"
	}
	if aw.BootstrapName == "" {
		aw.BootstrapName = "Nonoka Assistant"
	}
	if aw.BootstrapInstruction == "" {
		aw.BootstrapInstruction = "You are the Nonoka IM assistant. Use concise, helpful answers."
	}
	if len(secrets.JWTSecret) < 32 {
		return errString("AGENT_WORKER_JWT_SECRET must contain at least 32 bytes")
	}
	if bc.Data.Database == nil || bc.Data.Database.Source == "" {
		return errString("postgres DSN is required")
	}
	if bc.Data.Mongodb == nil || bc.Data.Mongodb.Uri == "" {
		return errString("mongodb URI is required")
	}
	return nil
}

func ensureData(bc *conf.Bootstrap) {
	if bc.Data == nil {
		bc.Data = &conf.Data{}
	}
	if bc.Data.Database == nil {
		bc.Data.Database = &conf.Data_Database{}
	}
	if bc.Data.Mongodb == nil {
		bc.Data.Mongodb = &conf.Data_MongoDB{}
	}
	if bc.Data.Kafka == nil {
		bc.Data.Kafka = &conf.Data_Kafka{}
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func setString(dst *string, key string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

func setInt32(dst *int32, key string) {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		*dst = int32(v)
	}
}

func setInt64(dst *int64, key string) {
	if v, err := strconv.ParseInt(os.Getenv(key), 10, 64); err == nil {
		*dst = v
	}
}

func setDuration(dst **durationpb.Duration, key string) {
	if raw := os.Getenv(key); raw != "" {
		if v, err := time.ParseDuration(raw); err == nil && v > 0 {
			*dst = durationpb.New(v)
		}
	}
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}

func splitList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
