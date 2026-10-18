package dlq

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricName names the feature's metrics (design §9).
type MetricName string

const (
	MetricScanDuration MetricName = "router_dlq_api_scan_duration_seconds"
	MetricScanRecords  MetricName = "router_dlq_api_scan_records_total"
	MetricActions      MetricName = "router_dlq_actions_total"
)

// MetricLabel names metric labels.
type MetricLabel string

const (
	LabelAction  MetricLabel = "action"
	LabelOutcome MetricLabel = "outcome"
)

// Metrics records scan and action metrics (no payloads, no route labels).
type Metrics struct {
	scanDuration prometheus.Histogram
	scanRecords  prometheus.Counter
	actions      *prometheus.CounterVec
}

// NewMetrics registers the metrics on reg (nil reg: unregistered collectors for tests).
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		scanDuration: prometheus.NewHistogram(prometheus.HistogramOpts{Name: string(MetricScanDuration), Help: "Duration of DLQ scans served by the API."}),
		scanRecords:  prometheus.NewCounter(prometheus.CounterOpts{Name: string(MetricScanRecords), Help: "DLQ records read by API scans."}),
		actions:      prometheus.NewCounterVec(prometheus.CounterOpts{Name: string(MetricActions), Help: "Dead-letter action results."}, []string{string(LabelAction), string(LabelOutcome)}),
	}
	if reg != nil {
		reg.MustRegister(m.scanDuration, m.scanRecords, m.actions)
	}
	return m
}

// Scan observes one scan.
func (m *Metrics) Scan(elapsed time.Duration, scanned int) {
	m.scanDuration.Observe(elapsed.Seconds())
	m.scanRecords.Add(float64(scanned))
}

// Action counts the outcomes of one action report.
func (m *Metrics) Action(r ActionReport) {
	for _, res := range r.Results {
		m.actions.WithLabelValues(string(r.Kind), string(res.Outcome)).Inc()
	}
}
