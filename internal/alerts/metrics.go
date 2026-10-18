package alerts

import (
	"github.com/prometheus/client_golang/prometheus"
)

// SelfMetric names the alerting self-metrics.
type SelfMetric string

const (
	MetricEvalErrors    SelfMetric = "router_alert_eval_errors_total"
	MetricNotifications SelfMetric = "router_alert_notifications_total"
)

// SelfMetricLabel names labels of the self-metrics.
type SelfMetricLabel string

const (
	LabelChannel SelfMetricLabel = "channel"
	LabelResult  SelfMetricLabel = "result"
)

// SelfMetricHelp holds the help texts.
type SelfMetricHelp string

const (
	HelpEvalErrors    SelfMetricHelp = "Alert rule evaluations skipped because the metrics query failed."
	HelpNotifications SelfMetricHelp = "Alert notification attempts by channel and result."
)

// Metrics are the alerting self-metrics registered on the API registry.
type Metrics struct {
	evalErrors    prometheus.Counter
	notifications *prometheus.CounterVec
}

// NewMetrics registers the counters on reg (nil reg keeps them unregistered, for tests).
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		evalErrors: prometheus.NewCounter(prometheus.CounterOpts{Name: string(MetricEvalErrors), Help: string(HelpEvalErrors)}),
		notifications: prometheus.NewCounterVec(prometheus.CounterOpts{Name: string(MetricNotifications), Help: string(HelpNotifications)},
			[]string{string(LabelChannel), string(LabelResult)}),
	}
	if reg != nil {
		reg.MustRegister(m.evalErrors, m.notifications)
	}
	return m
}

// EvalError implements engine.EvalErrorCounter.
func (m *Metrics) EvalError() {
	m.evalErrors.Inc()
}

// Notified counts one notification attempt.
func (m *Metrics) Notified(channel ChannelType, result string) {
	m.notifications.WithLabelValues(string(channel), result).Inc()
}

// EvalErrors returns the counter (tests).
func (m *Metrics) EvalErrors() prometheus.Counter {
	return m.evalErrors
}

// Notifications returns the counter vector (tests).
func (m *Metrics) Notifications() *prometheus.CounterVec {
	return m.notifications
}
