package telemetry

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestTraceContextRoundTrip(t *testing.T) {
	original := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    [16]byte{1, 2, 3},
		SpanID:     [8]byte{4, 5, 6},
		TraceFlags: trace.FlagsSampled,
	})
	carrier := propagation.MapCarrier{}
	propagator := propagation.TraceContext{}
	propagator.Inject(trace.ContextWithSpanContext(context.Background(), original), carrier)

	extracted := propagator.Extract(context.Background(), carrier)
	got := trace.SpanContextFromContext(extracted)
	if got.TraceID() != original.TraceID() || got.SpanID() != original.SpanID() || got.TraceFlags() != original.TraceFlags() {
		t.Fatalf("extracted context %#v, want %#v", got, original)
	}
	if carrier.Get("traceparent") == "" {
		t.Fatal("traceparent was not injected")
	}
}
