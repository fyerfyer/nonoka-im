// Package bootstrap contains the process-level setup for the agent worker:
// telemetry, probe endpoints and the optional preset agent seed.
package bootstrap

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"nonoka-im/internal/agent/config"
	"nonoka-im/internal/agent/credential"
	"nonoka-im/internal/agent/metrics"
	"nonoka-im/internal/agent/tools/catalog"
	agentdata "nonoka-im/internal/data/agent"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv/v1.24.0"
)

// InitTracer installs an OTLP tracer provider when an endpoint is configured.
func InitTracer(ctx context.Context, cfg config.ObservabilityConfig) (func(), error) {
	if cfg.OTLPTracesEndpoint == "" {
		return func() {}, nil
	}
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(cfg.OTLPTracesEndpoint))
	if err != nil {
		return nil, fmt.Errorf("initialize OTLP trace exporter: %w", err)
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName("nonoka-agent-worker"))),
	)
	otel.SetTracerProvider(provider)
	return func() { _ = provider.Shutdown(context.Background()) }, nil
}

// StartHealthAndMetrics starts the optional liveness and Prometheus endpoints.
// A blank address disables the corresponding server.
func StartHealthAndMetrics(cfg config.ObservabilityConfig, m *metrics.Metrics, logger log.Logger) func() {
	shutdown := make([]func(), 0, 2)
	if cfg.HealthAddr != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})
		shutdown = append(shutdown, serve(cfg.HealthAddr, mux, "health", logger))
	}
	if cfg.MetricsAddr != "" && m != nil {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{}))
		shutdown = append(shutdown, serve(cfg.MetricsAddr, mux, "metrics", logger))
	}
	return func() {
		for _, fn := range shutdown {
			fn()
		}
	}
}

func serve(addr string, handler http.Handler, name string, logger log.Logger) func() {
	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.Log(log.LevelInfo, "msg", "agent worker "+name+" server listening on "+addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Log(log.LevelWarn, "msg", fmt.Sprintf("%s server error: %v", name, err))
		}
	}()
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
}

// SeedAgent creates the preset agent once and binds its built-in tools. A
// zero BootstrapOwnerID disables seeding.
func SeedAgent(ctx context.Context, store *agentdata.Store, az interface {
	SetAgentOwner(context.Context, int64, int64) error
	SetAgentToolBinding(context.Context, int64, string, bool) error
}, cfg config.Config, logger log.Logger) error {
	if cfg.Agent.BootstrapOwnerID <= 0 {
		return nil
	}
	instruction := cfg.Agent.BootstrapInstruction
	if instruction == "" {
		instruction = "You are the Nonoka IM assistant. Use concise, helpful answers."
	}
	bootstrapKeyEnc := ""
	if cfg.Model.APIKey != "" {
		if len(cfg.SecretKey) < 32 {
			return fmt.Errorf("AGENT_SECRET_KEY is required to seal the bootstrap model key")
		}
		sealed, err := credential.Seal([]byte(cfg.SecretKey), []byte(cfg.Model.APIKey))
		if err == nil {
			bootstrapKeyEnc = base64.StdEncoding.EncodeToString(sealed)
		}
	}
	toolsJSON, err := json.Marshal(catalog.DefaultBootstrapToolNames())
	if err != nil {
		return fmt.Errorf("encode bootstrap tool list: %w", err)
	}
	_, err = store.BootstrapAgent(ctx, cfg.Agent.ID, cfg.Agent.BotID, cfg.Agent.BootstrapOwnerID,
		cfg.Agent.BootstrapName, instruction, cfg.Model.Name, cfg.Model.BaseURL, bootstrapKeyEnc, 4096, string(toolsJSON),
		func(ctx context.Context, id int64) error {
			return az.SetAgentOwner(ctx, id, cfg.Agent.BootstrapOwnerID)
		})
	if err != nil {
		return err
	}
	// Always reconcile tool bindings, not only when the agent row was just
	// created. An OpenFGA store reset otherwise leaves an existing agent with
	// all tools permanently unauthorized.
	for _, tool := range catalog.DefaultBootstrapToolNames() {
		if err := az.SetAgentToolBinding(ctx, cfg.Agent.ID, tool, true); err != nil {
			logger.Log(log.LevelWarn, "msg", fmt.Sprintf("bind bootstrap tool %s: %v", tool, err))
		}
	}
	return nil
}
