// Package bootstrap contains process-level setup for the agent worker:
// telemetry, environment overrides and the optional preset agent seed.
package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"

	"nonoka-im/internal/agent/tools/catalog"

	"github.com/go-kratos/kratos/v2/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv/v1.24.0"
)

// InitTracer installs an OTLP tracer provider when an endpoint is configured.
func InitTracer(ctx context.Context, endpoint string) (func(), error) {
	if endpoint == "" {
		return func() {}, nil
	}
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
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

// SeedStore is the narrow persistence surface used to create the preset agent.
type SeedStore interface {
	BootstrapAgent(ctx context.Context, id, botID, ownerID int64, name, instruction string, modelServiceID, modelID int64, modelKey string, maxTokens int, toolsJSON string, grant func(context.Context, int64) error) (bool, error)
}

// SeedAuthorizer is the narrow OpenFGA surface used while seeding.
type SeedAuthorizer interface {
	SetAgentOwner(ctx context.Context, agentID, userID int64) error
	SetAgentToolBinding(ctx context.Context, agentID int64, tool string, enabled bool) error
}

// SeedInput is the resolved preset-agent configuration.
type SeedInput struct {
	AgentID        int64
	BotID          int64
	OwnerID        int64
	Name           string
	Instruction    string
	ModelServiceID int64
	ModelID        int64
	ModelKey       string
}

// SeedAgent creates the preset agent once and binds its built-in tools. A
// zero OwnerID disables seeding.
func SeedAgent(ctx context.Context, store SeedStore, az SeedAuthorizer, in SeedInput, logger log.Logger) error {
	if in.OwnerID <= 0 {
		return nil
	}
	if in.Instruction == "" {
		in.Instruction = "You are the Nonoka IM assistant. Use concise, helpful answers."
	}
	if in.ModelServiceID <= 0 || in.ModelID <= 0 {
		return fmt.Errorf("bootstrap agent requires a model service/model")
	}
	toolsJSON, err := json.Marshal(catalog.DefaultBootstrapToolNames())
	if err != nil {
		return fmt.Errorf("encode bootstrap tool list: %w", err)
	}
	_, err = store.BootstrapAgent(ctx, in.AgentID, in.BotID, in.OwnerID,
		in.Name, in.Instruction, in.ModelServiceID, in.ModelID, in.ModelKey, 4096, string(toolsJSON),
		func(ctx context.Context, id int64) error {
			return az.SetAgentOwner(ctx, id, in.OwnerID)
		})
	if err != nil {
		return err
	}
	for _, tool := range catalog.DefaultBootstrapToolNames() {
		if err := az.SetAgentToolBinding(ctx, in.AgentID, tool, true); err != nil {
			logger.Log(log.LevelWarn, "msg", fmt.Sprintf("bind bootstrap tool %s: %v", tool, err))
		}
	}
	return nil
}
