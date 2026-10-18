package audit

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventrouter/router/internal/platform/config"
)

// SinkFactory builds one sink kind from configuration.
type SinkFactory func(cfg config.AuditStreamConfig, log *slog.Logger) (Sink, error)

// SinkRegistry selects the configured sinks without switching on concrete types.
type SinkRegistry struct {
	factories map[SinkKind]SinkFactory
}

// NewSinkRegistry returns the registry with platform-core's log sink; features Register more kinds.
func NewSinkRegistry() *SinkRegistry {
	r := &SinkRegistry{factories: map[SinkKind]SinkFactory{}}
	r.Register(config.AuditStreamLog, func(_ config.AuditStreamConfig, log *slog.Logger) (Sink, error) {
		return NewLogSink(log), nil
	})
	return r
}

// Register adds a factory for a sink kind.
func (r *SinkRegistry) Register(kind SinkKind, f SinkFactory) {
	r.factories[kind] = f
}

// Build instantiates every sink named in configuration.
func (r *SinkRegistry) Build(cfg config.AuditStreamConfig, log *slog.Logger) ([]Sink, error) {
	var out []Sink
	for _, kind := range cfg.Sinks {
		f, ok := r.factories[kind]
		if !ok {
			return nil, fmt.Errorf("audit: no stream sink registered for %q", kind)
		}
		sink, err := f(cfg, log)
		if err != nil {
			return nil, fmt.Errorf("audit: stream sink %s: %w", kind, err)
		}
		out = append(out, sink)
	}
	return out, nil
}

// LogSinkMessage is the message of every streamed audit log line (log shippers filter on it).
type LogSinkMessage string

const (
	LogSinkEvent  LogSinkMessage = "audit.event"
	LogSinkRecord LogSinkMessage = "audit"
)

// LogSink writes each committed entry as one structured log line for log-based SIEM ingestion.
type LogSink struct {
	log *slog.Logger
}

// NewLogSink builds the sink.
func NewLogSink(log *slog.Logger) *LogSink {
	return &LogSink{log: log}
}

// Kind implements Sink.
func (s *LogSink) Kind() SinkKind { return config.AuditStreamLog }

// Deliver implements Sink.
func (s *LogSink) Deliver(ctx context.Context, rec StreamRecord) error {
	s.log.InfoContext(ctx, string(LogSinkEvent), string(LogSinkRecord), rec)
	return nil
}

// StreamMetric names the stream's Prometheus metrics.
type StreamMetric string

const (
	MetricStreamDelivered StreamMetric = "router_audit_stream_delivered_total"
	MetricStreamDropped   StreamMetric = "router_audit_stream_dropped_total"
	MetricStreamFailed    StreamMetric = "router_audit_stream_failed_total"
	MetricLabelSink       StreamMetric = "sink"
)

// PromStreamObserver counts stream deliveries per sink in Prometheus.
type PromStreamObserver struct {
	delivered, dropped, failed *prometheus.CounterVec
}

// NewPromStreamObserver registers the counters on reg.
func NewPromStreamObserver(reg prometheus.Registerer) *PromStreamObserver {
	o := &PromStreamObserver{
		delivered: prometheus.NewCounterVec(prometheus.CounterOpts{Name: string(MetricStreamDelivered)}, []string{string(MetricLabelSink)}),
		dropped:   prometheus.NewCounterVec(prometheus.CounterOpts{Name: string(MetricStreamDropped)}, []string{string(MetricLabelSink)}),
		failed:    prometheus.NewCounterVec(prometheus.CounterOpts{Name: string(MetricStreamFailed)}, []string{string(MetricLabelSink)}),
	}
	for _, c := range []prometheus.Collector{o.delivered, o.dropped, o.failed} {
		_ = reg.Register(c)
	}
	return o
}

// Delivered implements StreamObserver.
func (o *PromStreamObserver) Delivered(kind SinkKind) {
	o.delivered.WithLabelValues(string(kind)).Inc()
}

// Dropped implements StreamObserver.
func (o *PromStreamObserver) Dropped(kind SinkKind) { o.dropped.WithLabelValues(string(kind)).Inc() }

// Failed implements StreamObserver.
func (o *PromStreamObserver) Failed(kind SinkKind) { o.failed.WithLabelValues(string(kind)).Inc() }
