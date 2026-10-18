package livetail

import "github.com/prometheus/client_golang/prometheus"

// TapMetricName names the tap's self-metrics.
type TapMetricName string

const MetricTailDropped TapMetricName = "router_livetail_dropped_total"

// TapMetricLabel names the labels of the tap's self-metrics.
type TapMetricLabel string

const (
	LabelRouteID TapMetricLabel = "route_id"
	LabelReason  TapMetricLabel = "reason"
)

// TapMetrics counts records the tap did not copy (rate limit or full queue), per route.
type TapMetrics struct {
	dropped *prometheus.CounterVec
}

// NewTapMetrics registers the counter on reg.
func NewTapMetrics(reg prometheus.Registerer) (*TapMetrics, error) {
	c := prometheus.NewCounterVec(prometheus.CounterOpts{Name: string(MetricTailDropped),
		Help: "Live tail records not copied to the tail stream, by reason."}, []string{string(LabelRouteID), string(LabelReason)})
	if err := reg.Register(c); err != nil {
		return nil, err
	}
	return &TapMetrics{dropped: c}, nil
}

// Dropped implements DropCounter.
func (m *TapMetrics) Dropped(routeID string, reason DropReason) {
	m.dropped.WithLabelValues(routeID, string(reason)).Inc()
}
