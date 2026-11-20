package kafka

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestTraceParentRoundTrip(t *testing.T) {
	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled, TraceState: trace.TraceState{},
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)
	raw := TraceParent(ctx)
	if raw == "" {
		t.Fatal("expected a traceparent value")
	}
	parsed, ok := ParseTraceParent(raw)
	if !ok || parsed.TraceID() != traceID || parsed.SpanID() != spanID {
		t.Fatalf("parse mismatch: ok=%v parsed=%v", ok, parsed)
	}
	link, ok := LinkFromTraceParent(raw)
	if !ok || link.SpanContext.TraceID() != traceID {
		t.Fatalf("link mismatch: ok=%v link=%v", ok, link)
	}
	if _, ok := ParseTraceParent("not-a-traceparent"); ok {
		t.Fatal("invalid traceparent must not parse")
	}
}
