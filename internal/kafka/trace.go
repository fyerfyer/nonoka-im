package kafka

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const TraceParentHeader = "traceparent"

// TraceParent renders the current span as a W3C traceparent header value.
func TraceParent(ctx context.Context) string {
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	return carrier.Get(TraceParentHeader)
}

// ParseTraceParent parses a W3C traceparent value into a span context.
func ParseTraceParent(raw string) (trace.SpanContext, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return trace.SpanContext{}, false
	}
	carrier := propagation.MapCarrier{TraceParentHeader: raw}
	ctx := propagation.TraceContext{}.Extract(context.Background(), carrier)
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return trace.SpanContext{}, false
	}
	return sc, true
}

// LinkFromTraceParent builds an OTel span link for a producer span ID carried
// in a message header. The consumer span remains the parent of the local work;
// the link records the causal producer without inventing a new parent.
func LinkFromTraceParent(raw string) (trace.Link, bool) {
	sc, ok := ParseTraceParent(raw)
	if !ok {
		return trace.Link{}, false
	}
	return trace.Link{SpanContext: sc}, true
}
