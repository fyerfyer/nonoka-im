// Package config defines the agent-worker configuration. Precedence is
// defaults < YAML file < environment variables, so container deployments can
// keep injecting secrets through the environment while the checked-in YAML
// documents the shape and local defaults.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a YAML-friendly time.Duration ("2m", "500ms").
type Duration time.Duration

func (d Duration) Value() time.Duration { return time.Duration(d) }

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var raw string
	if err := node.Decode(&raw); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return err
	}
	*d = Duration(parsed)
	return nil
}

type DataConfig struct {
	PostgresDSN     string `yaml:"postgres_dsn"`
	MongoDBURI      string `yaml:"mongodb_uri"`
	MongoDBDatabase string `yaml:"mongodb_database"`
}

type KafkaConfig struct {
	Brokers           []string `yaml:"brokers"`
	EventsTopic       string   `yaml:"events_topic"`
	EventsGroup       string   `yaml:"events_group"`
	DLQTopic          string   `yaml:"dlq_topic"`
	ConsumerWorkers   int      `yaml:"consumer_workers"`
	MaxRetries        int      `yaml:"max_retries"`
	DispatchTimeout   Duration `yaml:"dispatch_timeout"`
	CommitBatchSize   int      `yaml:"commit_batch_size"`
	CommitFlushPeriod Duration `yaml:"commit_flush_period"`
}

type WorkerConfig struct {
	RunWorkers    int      `yaml:"run_workers"`
	RunTimeout    Duration `yaml:"run_timeout"`
	Lease         Duration `yaml:"lease"`
	Heartbeat     Duration `yaml:"heartbeat"`
	QueuedPoll    Duration `yaml:"queued_poll"`
	SweepInterval Duration `yaml:"sweep_interval"`
	MaxAttempts   int      `yaml:"max_attempts"`
}

type IMConfig struct {
	HTTPURL string `yaml:"http_url"`
}

type AgentConfig struct {
	ID                   int64  `yaml:"id"`
	BotID                int64  `yaml:"bot_id"`
	BootstrapOwnerID     int64  `yaml:"bootstrap_owner_id"`
	BootstrapName        string `yaml:"bootstrap_name"`
	BootstrapInstruction string `yaml:"bootstrap_instruction"`
}

type ModelConfig struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
	Name    string `yaml:"name"`
}

type ObservabilityConfig struct {
	HealthAddr         string `yaml:"health_addr"`
	MetricsAddr        string `yaml:"metrics_addr"`
	OTLPTracesEndpoint string `yaml:"otlp_traces_endpoint"`
}

// Config is the full agent-worker configuration.
type Config struct {
	Data          DataConfig          `yaml:"data"`
	Kafka         KafkaConfig         `yaml:"kafka"`
	Worker        WorkerConfig        `yaml:"worker"`
	IM            IMConfig            `yaml:"im"`
	Agent         AgentConfig         `yaml:"agent"`
	Model         ModelConfig         `yaml:"model"`
	Observability ObservabilityConfig `yaml:"observability"`

	// Secrets stay environment-only so they never land in YAML.
	JWTSecret []byte `yaml:"-"`
	SecretKey string `yaml:"-"`
}

// Default returns the local-development defaults. The YAML file overrides
// them, then environment variables override both.
func Default() Config {
	return Config{
		Data: DataConfig{
			PostgresDSN:     "host=127.0.0.1 user=postgres password=root dbname=nonoka_im port=5432 sslmode=disable",
			MongoDBURI:      "mongodb://127.0.0.1:27017",
			MongoDBDatabase: "nonoka_im",
		},
		Kafka: KafkaConfig{
			Brokers:           []string{"127.0.0.1:9092"},
			EventsTopic:       "agent-events",
			EventsGroup:       "nonoka-agent-worker",
			DLQTopic:          "agent-events-dlq",
			ConsumerWorkers:   4,
			MaxRetries:        2,
			DispatchTimeout:   Duration(10 * time.Second),
			CommitBatchSize:   100,
			CommitFlushPeriod: Duration(time.Second),
		},
		Worker: WorkerConfig{
			RunWorkers:    8,
			RunTimeout:    Duration(2 * time.Minute),
			Lease:         Duration(3 * time.Minute),
			Heartbeat:     Duration(45 * time.Second),
			QueuedPoll:    Duration(time.Second),
			SweepInterval: Duration(30 * time.Second),
			MaxAttempts:   5,
		},
		IM: IMConfig{HTTPURL: "http://127.0.0.1:8000"},
		Agent: AgentConfig{
			ID:                   1,
			BootstrapName:        "Nonoka Assistant",
			BootstrapInstruction: "You are the Nonoka IM assistant. Use concise, helpful answers.",
		},
		Model: ModelConfig{BaseURL: "https://api.deepseek.com/v1"},
		Observability: ObservabilityConfig{
			HealthAddr:  ":8090",
			MetricsAddr: ":8091",
		},
	}
}

// Load reads the optional YAML file at path and applies environment
// overrides. An empty or missing path is allowed (env-only deployments).
func Load(path string) (Config, error) {
	cfg := Default()
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			// An explicitly configured path must fail loudly. Silently falling
			// back to defaults/env could connect the worker to the wrong
			// database or model endpoint.
			return Config{}, fmt.Errorf("read agent-worker config %s: %w", path, err)
		}
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return Config{}, fmt.Errorf("parse agent-worker config %s: %w", path, err)
		}
	}
	applyEnv(&cfg)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyEnv(c *Config) {
	env := func(key string, dst *string) {
		if v := os.Getenv(key); v != "" {
			*dst = v
		}
	}
	env("POSTGRES_DSN", &c.Data.PostgresDSN)
	env("MONGODB_URI", &c.Data.MongoDBURI)
	env("MONGODB_DATABASE", &c.Data.MongoDBDatabase)

	if v := os.Getenv("KAFKA_BROKERS"); v != "" {
		c.Kafka.Brokers = splitList(v)
	}
	env("AGENT_EVENTS_TOPIC", &c.Kafka.EventsTopic)
	env("AGENT_EVENTS_GROUP", &c.Kafka.EventsGroup)
	env("AGENT_EVENTS_DLQ_TOPIC", &c.Kafka.DLQTopic)
	intEnv("AGENT_WORKERS", &c.Kafka.ConsumerWorkers)
	intEnv("AGENT_MAX_RETRIES", &c.Kafka.MaxRetries)
	durationEnv("AGENT_DISPATCH_TIMEOUT", &c.Kafka.DispatchTimeout)
	intEnv("AGENT_COMMIT_BATCH_SIZE", &c.Kafka.CommitBatchSize)
	durationEnv("AGENT_COMMIT_FLUSH_PERIOD", &c.Kafka.CommitFlushPeriod)

	intEnv("AGENT_RUN_WORKERS", &c.Worker.RunWorkers)
	durationEnv("AGENT_RUN_TIMEOUT", &c.Worker.RunTimeout)
	durationEnv("AGENT_RUN_LEASE", &c.Worker.Lease)
	durationEnv("AGENT_HEARTBEAT", &c.Worker.Heartbeat)
	durationEnv("AGENT_QUEUED_POLL", &c.Worker.QueuedPoll)
	durationEnv("AGENT_SWEEP_INTERVAL", &c.Worker.SweepInterval)
	intEnv("AGENT_MAX_ATTEMPTS", &c.Worker.MaxAttempts)

	env("IM_HTTP_URL", &c.IM.HTTPURL)
	int64Env("AGENT_ID", &c.Agent.ID)
	int64Env("AGENT_BOT_ID", &c.Agent.BotID)
	int64Env("AGENT_BOOTSTRAP_OWNER_ID", &c.Agent.BootstrapOwnerID)
	env("AGENT_BOOTSTRAP_NAME", &c.Agent.BootstrapName)
	env("AGENT_BOOTSTRAP_INSTRUCTION", &c.Agent.BootstrapInstruction)

	if v := firstEnv("OPENAI_API_KEY", "DEEPSEEK_API_KEY", "DEEPSEEK_KEY"); v != "" {
		c.Model.APIKey = v
	}
	env("OPENAI_BASE_URL", &c.Model.BaseURL)
	env("OPENAI_MODEL", &c.Model.Name)

	env("AGENT_HEALTH_ADDR", &c.Observability.HealthAddr)
	env("AGENT_METRICS_ADDR", &c.Observability.MetricsAddr)
	env("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", &c.Observability.OTLPTracesEndpoint)

	if v := os.Getenv("AGENT_WORKER_JWT_SECRET"); v != "" {
		c.JWTSecret = []byte(v)
	}
	c.SecretKey = os.Getenv("AGENT_SECRET_KEY")
}

// Validate normalizes derived values and rejects unsafe combinations.
func (c *Config) Validate() error {
	if len(c.Kafka.Brokers) == 0 {
		return fmt.Errorf("kafka brokers are required")
	}
	if c.Kafka.EventsTopic == "" {
		return fmt.Errorf("agent events topic is required")
	}
	if c.Worker.RunWorkers < 1 {
		c.Worker.RunWorkers = 1
	}
	if c.Kafka.ConsumerWorkers < 1 {
		c.Kafka.ConsumerWorkers = 1
	}
	if c.Worker.MaxAttempts < 1 {
		c.Worker.MaxAttempts = 5
	}
	if c.Worker.RunTimeout.Value() <= 0 {
		c.Worker.RunTimeout = Duration(2 * time.Minute)
	}
	if c.Worker.Lease.Value() <= c.Worker.RunTimeout.Value() {
		c.Worker.Lease = Duration(c.Worker.RunTimeout.Value() + time.Minute)
	}
	if c.Worker.Heartbeat.Value() <= 0 {
		c.Worker.Heartbeat = Duration(c.Worker.Lease.Value() / 3)
	}
	if c.Worker.Heartbeat.Value() >= c.Worker.Lease.Value() {
		return fmt.Errorf("AGENT_HEARTBEAT (%s) must be shorter than AGENT_RUN_LEASE (%s)", c.Worker.Heartbeat.Value(), c.Worker.Lease.Value())
	}
	if len(c.JWTSecret) < 32 {
		return fmt.Errorf("AGENT_WORKER_JWT_SECRET must contain at least 32 bytes")
	}
	if c.Data.PostgresDSN == "" {
		return fmt.Errorf("postgres DSN is required")
	}
	return nil
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
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func intEnv(key string, dst *int) {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		*dst = v
	}
}

func int64Env(key string, dst *int64) {
	if v, err := strconv.ParseInt(os.Getenv(key), 10, 64); err == nil {
		*dst = v
	}
}

func durationEnv(key string, dst *Duration) {
	if v, err := time.ParseDuration(os.Getenv(key)); err == nil && v > 0 {
		*dst = Duration(v)
	}
}
