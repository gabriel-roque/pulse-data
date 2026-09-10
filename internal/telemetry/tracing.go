package telemetry

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/pulse-data/pulse"

// SetupTracing installs the W3C propagator and, when configured, an OTLP batch
// exporter. Use http(s):// for OTLP/HTTP and grpc(s):// for OTLP/gRPC. Bare
// endpoints keep the OTLP/HTTP default for backwards-compatible configuration.
func SetupTracing(ctx context.Context, service string) func(context.Context) error {
	otel.SetTextMapPropagator(propagation.TraceContext{})
	endpoint := strings.TrimSpace(os.Getenv("PULSE_OTEL_ENDPOINT"))
	if endpoint == "" {
		return func(context.Context) error { return nil }
	}

	var exporter sdktrace.SpanExporter
	var err error
	switch {
	case strings.HasPrefix(endpoint, "grpc://"), strings.HasPrefix(endpoint, "grpcs://"):
		opts, optionsErr := grpcExporterOptions(endpoint)
		if optionsErr == nil {
			exporter, err = otlptracegrpc.New(ctx, opts...)
		} else {
			err = optionsErr
		}
	default:
		opts, optionsErr := httpExporterOptions(endpoint)
		if optionsErr == nil {
			exporter, err = otlptracehttp.New(ctx, opts...)
		} else {
			err = optionsErr
		}
	}
	if err != nil {
		log.Printf("pulse telemetry disabled: OTLP exporter: %v", err)
		return func(context.Context) error { return nil }
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", service))),
	)
	otel.SetTracerProvider(provider)
	return provider.Shutdown
}

func httpExporterOptions(endpoint string) ([]otlptracehttp.Option, error) {
	if !strings.Contains(endpoint, "://") {
		return []otlptracehttp.Option{otlptracehttp.WithEndpoint(endpoint), otlptracehttp.WithInsecure()}, nil
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("OTLP endpoint %q has no host", endpoint)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, &unsupportedSchemeError{scheme: parsed.Scheme}
	}
	opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(parsed.Host)}
	if parsed.Scheme == "http" {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	if parsed.Path != "" && parsed.Path != "/" {
		opts = append(opts, otlptracehttp.WithURLPath(parsed.Path))
	}
	return opts, nil
}

func grpcExporterOptions(endpoint string) ([]otlptracegrpc.Option, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("OTLP endpoint %q has no host", endpoint)
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return nil, fmt.Errorf("OTLP/gRPC endpoint %q must not contain a path", endpoint)
	}
	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(parsed.Host)}
	if parsed.Scheme == "grpc" {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	return opts, nil
}

type unsupportedSchemeError struct{ scheme string }

func (e *unsupportedSchemeError) Error() string {
	return "unsupported OTLP endpoint scheme " + e.scheme
}

func Tracer() trace.Tracer { return otel.Tracer(tracerName) }

func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return Tracer().Start(ctx, name, opts...)
}

func EndSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}
