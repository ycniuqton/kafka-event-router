package assist

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricName names the assistant's Prometheus series and labels.
type MetricName string

const (
	MetricRequestsTotal MetricName = "router_assist_requests_total"
	MetricLatency       MetricName = "router_assist_latency_seconds"
	MetricLabelTask     MetricName = "task"
	MetricLabelProvider MetricName = "provider"
	MetricLabelResult   MetricName = "result"
)

// RequestResult is the outcome label of one assistant request.
type RequestResult string

const (
	ResultOK    RequestResult = "ok"
	ResultError RequestResult = "error"
)

// Metrics observes assistant requests (Prometheus in cmd/api, no-op in tests).
type Metrics interface {
	Observe(task AssistTask, provider ProviderKind, result RequestResult, took time.Duration)
}

// NoMetrics discards observations.
type NoMetrics struct{}

// Observe implements Metrics.
func (NoMetrics) Observe(AssistTask, ProviderKind, RequestResult, time.Duration) {}

// PromMetrics implements Metrics with a request counter and a latency histogram.
type PromMetrics struct {
	requests *prometheus.CounterVec
	latency  *prometheus.HistogramVec
}

// NewPromMetrics builds and registers the collectors (nil registerer: not registered).
func NewPromMetrics(reg prometheus.Registerer) *PromMetrics {
	m := &PromMetrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{Name: string(MetricRequestsTotal),
			Help: "Assistant requests by task, provider and result."},
			[]string{string(MetricLabelTask), string(MetricLabelProvider), string(MetricLabelResult)}),
		latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: string(MetricLatency),
			Help: "Assistant request latency.", Buckets: prometheus.DefBuckets}, []string{string(MetricLabelTask)}),
	}
	if reg != nil {
		reg.MustRegister(m.requests, m.latency)
	}
	return m
}

// Observe implements Metrics.
func (m *PromMetrics) Observe(task AssistTask, provider ProviderKind, result RequestResult, took time.Duration) {
	m.requests.WithLabelValues(string(task), string(provider), string(result)).Inc()
	m.latency.WithLabelValues(string(task)).Observe(took.Seconds())
}
