package preview

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventrouter/router/internal/platform/metrics"
)

// MetricName names the preview service metrics (namespace router).
type MetricName string

const (
	MetricRuns           MetricName = "preview_runs_total"
	MetricRunSeconds     MetricName = "preview_run_seconds"
	MetricLoadSeconds    MetricName = "preview_sample_load_seconds"
	MetricSessionsActive MetricName = "preview_sessions_active"
)

// MetricLabel names metric labels.
type MetricLabel string

const (
	LabelStatus MetricLabel = "status"
	LabelMode   MetricLabel = "mode"
)

// RunOutcome is the status label of a run.
type RunOutcome string

const (
	RunOK         RunOutcome = "ok"
	RunIncomplete RunOutcome = "incomplete"
	RunFailed     RunOutcome = "failed"
	RunRejected   RunOutcome = "busy"
)

// Metrics records preview runs, loads and live sessions.
type Metrics struct {
	runs     *prometheus.CounterVec
	runTime  prometheus.Histogram
	loadTime *prometheus.HistogramVec
	sessions prometheus.Gauge
}

// NewMetrics registers the preview metrics on reg (nil reg → unregistered collectors, for tests).
func NewMetrics(reg prometheus.Registerer) *Metrics {
	ns := string(metrics.RouterNamespace)
	m := &Metrics{
		runs:     prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: ns, Name: string(MetricRuns), Help: "Preview runs by outcome."}, []string{string(LabelStatus)}),
		runTime:  prometheus.NewHistogram(prometheus.HistogramOpts{Namespace: ns, Name: string(MetricRunSeconds), Help: "Preview run duration.", Buckets: prometheus.DefBuckets}),
		loadTime: prometheus.NewHistogramVec(prometheus.HistogramOpts{Namespace: ns, Name: string(MetricLoadSeconds), Help: "Sample load duration.", Buckets: prometheus.DefBuckets}, []string{string(LabelMode)}),
		sessions: prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: string(MetricSessionsActive), Help: "Sample sessions created and not deleted by this replica."}),
	}
	if reg != nil {
		reg.MustRegister(m.runs, m.runTime, m.loadTime, m.sessions)
	}
	return m
}

// Run records one run outcome and duration.
func (m *Metrics) Run(outcome RunOutcome, d time.Duration) {
	m.runs.WithLabelValues(string(outcome)).Inc()
	m.runTime.Observe(d.Seconds())
}

// Load records one sample load.
func (m *Metrics) Load(mode SampleMode, d time.Duration) {
	m.loadTime.WithLabelValues(string(mode)).Observe(d.Seconds())
}

// SessionOpened / SessionClosed track live sessions of this replica.
func (m *Metrics) SessionOpened() { m.sessions.Inc() }
func (m *Metrics) SessionClosed() { m.sessions.Dec() }
