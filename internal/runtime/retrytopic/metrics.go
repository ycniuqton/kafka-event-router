package retrytopic

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventrouter/router/internal/dlq/record"
)

// MetricName names the retry metrics.
type MetricName string

const (
	MetricScheduled MetricName = "router_events_retry_scheduled_total"
	MetricRecovered MetricName = "router_events_retry_recovered_total"
	MetricExhausted MetricName = "router_events_retry_exhausted_total"
	MetricPending   MetricName = "router_retry_pending"
)

// MetricLabel names the retry metric labels.
type MetricLabel string

const (
	LabelRouteID   MetricLabel = "route_id"
	LabelVersion   MetricLabel = "version"
	LabelWorker    MetricLabel = "worker"
	LabelErrorType MetricLabel = "error_type"
	LabelAttempt   MetricLabel = "attempt"
)

// Labels identify one runner's series.
type Labels struct {
	RouteID string
	Version string
	Worker  string
}

// Metrics are the retry counters and the pending gauge (nil-safe: a nil *Metrics records nothing).
type Metrics struct {
	scheduled *prometheus.CounterVec
	recovered *prometheus.CounterVec
	exhausted *prometheus.CounterVec
	pending   *prometheus.GaugeVec
}

// NewMetrics registers the retry metrics on reg.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	base := []string{string(LabelRouteID), string(LabelVersion), string(LabelWorker), string(LabelErrorType)}
	m := &Metrics{
		scheduled: prometheus.NewCounterVec(prometheus.CounterOpts{Name: string(MetricScheduled)}, append(append([]string{}, base...), string(LabelAttempt))),
		recovered: prometheus.NewCounterVec(prometheus.CounterOpts{Name: string(MetricRecovered)}, base),
		exhausted: prometheus.NewCounterVec(prometheus.CounterOpts{Name: string(MetricExhausted)}, base),
		pending:   prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: string(MetricPending)}, []string{string(LabelRouteID)}),
	}
	reg.MustRegister(m.scheduled, m.recovered, m.exhausted, m.pending)
	return m
}

// Scheduled counts one record written to the tier of attempt n.
func (m *Metrics) Scheduled(l Labels, et record.ErrorType, n int) {
	if m != nil {
		m.scheduled.WithLabelValues(l.RouteID, l.Version, l.Worker, string(et), strconv.Itoa(n)).Inc()
	}
}

// Recovered counts one retried record that finally succeeded (et: the error that caused the retry).
func (m *Metrics) Recovered(l Labels, et record.ErrorType) {
	if m != nil {
		m.recovered.WithLabelValues(l.RouteID, l.Version, l.Worker, string(et)).Inc()
	}
}

// Exhausted counts one record that used every attempt and followed onError.
func (m *Metrics) Exhausted(l Labels, et record.ErrorType) {
	if m != nil {
		m.exhausted.WithLabelValues(l.RouteID, l.Version, l.Worker, string(et)).Inc()
	}
}

// Pending sets the tier records of a route not yet finished.
func (m *Metrics) Pending(routeID string, n int64) {
	if m != nil {
		m.pending.WithLabelValues(routeID).Set(float64(n))
	}
}
