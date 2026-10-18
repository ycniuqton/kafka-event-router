// Package tracing sets up OpenTelemetry for every binary: a config-selected span exporter (none, stdout,
// OTLP/HTTP), W3C trace-context propagation, an HTTP server middleware and trace ids in log lines.
package tracing

import (
	"context"
	"fmt"
	"io"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/eventrouter/router/internal/platform/config"
)

// ResourceKey names the resource attributes every span carries.
type ResourceKey string

const (
	KeyServiceName ResourceKey = "service.name"
	KeyEnvironment ResourceKey = "deployment.environment"
	KeyWorkspace   ResourceKey = "eventrouter.workspace"
)

// ServicePrefix prefixes service.name (event-router-api, event-router-worker, …).
type ServicePrefix string

const DefaultServicePrefix ServicePrefix = "event-router-"

// ExporterFactory builds a span exporter; a nil exporter means tracing is off.
type ExporterFactory func(ctx context.Context, cfg config.TracingConfig) (sdktrace.SpanExporter, error)

// Registry selects the span exporter by ROUTER_TRACING_EXPORTER.
type Registry struct {
	factories map[config.TracingExporterKind]ExporterFactory
	stdout    io.Writer
}

// NewRegistry returns the none, stdout (to stderr, so JSON logs on stdout stay clean) and otlp exporters.
func NewRegistry() *Registry {
	r := &Registry{factories: map[config.TracingExporterKind]ExporterFactory{}, stdout: os.Stderr}
	r.Register(config.TracingNone, func(context.Context, config.TracingConfig) (sdktrace.SpanExporter, error) { return nil, nil })
	r.Register(config.TracingStdout, func(context.Context, config.TracingConfig) (sdktrace.SpanExporter, error) {
		return stdouttrace.New(stdouttrace.WithWriter(r.stdout))
	})
	r.Register(config.TracingOTLP, func(ctx context.Context, cfg config.TracingConfig) (sdktrace.SpanExporter, error) {
		opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(cfg.OTLPEndpoint)}
		if cfg.OTLPInsecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		return otlptracehttp.New(ctx, opts...)
	})
	return r
}

// Register adds an exporter factory.
func (r *Registry) Register(kind config.TracingExporterKind, f ExporterFactory) {
	r.factories[kind] = f
}

// Setup installs the global tracer provider and propagator for one process.
func (r *Registry) Setup(ctx context.Context, cfg config.Config) (*Provider, error) {
	f, ok := r.factories[cfg.Tracing.Exporter]
	if !ok {
		return nil, fmt.Errorf("tracing: no exporter registered for %q", cfg.Tracing.Exporter)
	}
	exporter, err := f(ctx, cfg.Tracing)
	if err != nil {
		return nil, fmt.Errorf("tracing: %w", err)
	}
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	if exporter == nil {
		otel.SetTracerProvider(noop.NewTracerProvider())
		return &Provider{}, nil
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter), sdktrace.WithResource(r.resource(cfg)),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.Tracing.SampleRatio))))
	otel.SetTracerProvider(tp)
	return &Provider{sdk: tp}, nil
}

func (r *Registry) resource(cfg config.Config) *resource.Resource {
	return resource.NewSchemaless(
		attribute.String(string(KeyServiceName), string(DefaultServicePrefix)+string(cfg.Service)),
		attribute.String(string(KeyEnvironment), string(cfg.Environment)),
		attribute.String(string(KeyWorkspace), cfg.Workspace))
}

// Provider owns the SDK tracer provider (nil when tracing is off) and flushes it on shutdown.
type Provider struct {
	sdk *sdktrace.TracerProvider
}

// Tracer returns the global tracer for instrumentation scope name.
func (p *Provider) Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

// Shutdown flushes pending spans.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p.sdk == nil {
		return nil
	}
	return p.sdk.Shutdown(ctx)
}
