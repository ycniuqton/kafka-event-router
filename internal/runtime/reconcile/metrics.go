package reconcile

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/runner"
)

// Worker-level metric names.
const (
	MetricControlPlaneUp runner.MetricName = "router_worker_control_plane_up"
	MetricCapacityUsed   runner.MetricName = "router_worker_capacity_used"
)

// GaugeValue is the 0/1 value of boolean gauges.
type GaugeValue float64

const (
	GaugeDown GaugeValue = 0
	GaugeUp   GaugeValue = 1
)

// Metrics are the reconciler's worker metrics.
type Metrics struct {
	up       prometheus.Gauge
	capacity prometheus.Gauge
	reloads  *prometheus.CounterVec
}

// NewMetrics registers the worker gauges (nil registerer = unregistered).
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		up:       prometheus.NewGauge(prometheus.GaugeOpts{Name: string(MetricControlPlaneUp)}),
		capacity: prometheus.NewGauge(prometheus.GaugeOpts{Name: string(MetricCapacityUsed)}),
		reloads:  prometheus.NewCounterVec(prometheus.CounterOpts{Name: string(runner.MetricReloads)}, []string{string(runner.LabelKind)}),
	}
	if reg != nil {
		reg.MustRegister(m.up, m.capacity, m.reloads)
	}
	return m
}

// ControlPlaneUp sets router_worker_control_plane_up.
func (m *Metrics) ControlPlaneUp(up bool) {
	v := GaugeDown
	if up {
		v = GaugeUp
	}
	m.up.Set(float64(v))
}

// Capacity sets router_worker_capacity_used.
func (m *Metrics) Capacity(used float64) { m.capacity.Set(used) }

// Reload counts a version change by kind.
func (m *Metrics) Reload(kind domain.ReloadKind) { m.reloads.WithLabelValues(string(kind)).Inc() }
