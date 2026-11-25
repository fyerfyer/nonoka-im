package tools

import (
	"context"
	"time"

	agentmetrics "nonoka-im/internal/agent/metrics"

	"github.com/cloudwego/eino/compose"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ObserveMiddleware adds one span and one metric sample per tool call. It
// deliberately records names, IDs, status and duration only; arguments and
// results stay out of telemetry.
func ObserveMiddleware(metrics *agentmetrics.Metrics) compose.ToolMiddleware {
	tracer := otel.Tracer("nonoka-im/agent/tools")
	wrap := func(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
		return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
			start := time.Now()
			name := "tool"
			callID := ""
			if input != nil {
				if input.Name != "" {
					name = input.Name
				}
				callID = input.CallID
			}
			ctx, span := tracer.Start(ctx, "execute_tool "+name, trace.WithAttributes(
				attribute.String("gen_ai.operation.name", "execute_tool"),
				attribute.String("gen_ai.tool.name", name),
				attribute.String("gen_ai.tool.type", "extension"),
			))
			if callID != "" {
				span.SetAttributes(attribute.String("gen_ai.tool.call.id", callID))
			}
			out, err := next(ctx, input)
			status := "ok"
			if err != nil {
				status = "error"
				span.SetStatus(codes.Error, "tool execution failed")
				span.SetAttributes(attribute.String("error.type", "tool_error"))
			}
			span.End()
			if metrics != nil {
				metrics.ObserveTool(name, status, time.Since(start).Seconds())
			}
			return out, err
		}
	}
	return compose.ToolMiddleware{Invokable: wrap}
}
