package library

import (
	"github.com/prometheus/client_golang/prometheus"
)

// MetricName names the library's Prometheus series and labels.
type MetricName string

const (
	MetricPublishTotal MetricName = "router_library_publish_total"
	MetricLabelObject  MetricName = "object"
)

// PublishMetrics counts published versions (first versions included) per object kind.
type PublishMetrics interface {
	Published(kind ObjectKind)
}

// NoPublishMetrics discards counts.
type NoPublishMetrics struct{}

// Published implements PublishMetrics.
func (NoPublishMetrics) Published(ObjectKind) {}

// PromPublishMetrics implements PublishMetrics with a counter.
type PromPublishMetrics struct {
	published *prometheus.CounterVec
}

// NewPromPublishMetrics builds and registers the counter (nil registerer: not registered).
func NewPromPublishMetrics(reg prometheus.Registerer) *PromPublishMetrics {
	m := &PromPublishMetrics{published: prometheus.NewCounterVec(prometheus.CounterOpts{Name: string(MetricPublishTotal),
		Help: "Library versions published by object kind."}, []string{string(MetricLabelObject)})}
	if reg != nil {
		reg.MustRegister(m.published)
	}
	return m
}

// Published implements PublishMetrics.
func (m *PromPublishMetrics) Published(kind ObjectKind) {
	m.published.WithLabelValues(string(kind)).Inc()
}
