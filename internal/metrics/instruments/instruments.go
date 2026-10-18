package instruments

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventrouter/router/internal/platform/metrics"
)

// RouteLabels are the labels every per-route series carries.
type RouteLabels struct {
	RouteID string
	Branch  string
	Version string
	Worker  string
}

func (l RouteLabels) values(extra ...string) []string {
	return append([]string{l.RouteID, l.Branch, l.Version, l.Worker}, extra...)
}

func (l RouteLabels) match() prometheus.Labels {
	return prometheus.Labels{string(LabelRouteID): l.RouteID, string(LabelBranch): l.Branch,
		string(LabelVersion): l.Version, string(LabelWorker): l.Worker}
}

// Buckets are the histogram buckets (seconds) of latency and step duration.
var (
	LatencyBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60}
	StepBuckets    = []float64{.0001, .0005, .001, .0025, .005, .01, .025, .05, .1, .25, .5, 1}
)

// RouteInstruments is the §12 metric set; route-runtime calls it, nothing else writes these series.
type RouteInstruments struct {
	consumed, filtered, produced, dlq, jsTimeouts *prometheus.CounterVec
	lag, jsMemory, runnerState                    *prometheus.GaugeVec
	latency, step                                 *prometheus.HistogramVec
}

// NewRouteInstruments registers the metric set on the service registry.
func NewRouteInstruments(reg *metrics.Registry) *RouteInstruments {
	return NewRouteInstrumentsOn(reg.Registerer())
}

// NewRouteInstrumentsOn registers the metric set on any registerer (tests use a fresh registry).
func NewRouteInstrumentsOn(reg prometheus.Registerer) *RouteInstruments {
	f := vecFactory{}
	i := &RouteInstruments{
		consumed: f.counter(MetricConsumed, HelpConsumed), filtered: f.counter(MetricFiltered, HelpFiltered),
		produced: f.counter(MetricProduced, HelpProduced), dlq: f.counter(MetricDLQ, HelpDLQ, LabelErrorType),
		jsTimeouts: f.counter(MetricJSTimeouts, HelpJSTimeouts),
		lag:        f.gauge(MetricConsumerLag, HelpConsumerLag, LabelPartition), jsMemory: f.gauge(MetricJSMemory, HelpJSMemory),
		runnerState: f.gauge(MetricRunnerState, HelpRunnerState),
		latency:     f.histogram(MetricLatency, HelpLatency, LatencyBuckets),
		step:        f.histogram(MetricStepDuration, HelpStepDuration, StepBuckets, LabelStep),
	}
	reg.MustRegister(i.consumed, i.filtered, i.produced, i.dlq, i.jsTimeouts, i.lag, i.jsMemory, i.runnerState, i.latency, i.step)
	return i
}

// Consumed counts one event read from the source.
func (i *RouteInstruments) Consumed(l RouteLabels) { i.consumed.WithLabelValues(l.values()...).Inc() }

// Filtered counts one event dropped by the filter.
func (i *RouteInstruments) Filtered(l RouteLabels) { i.filtered.WithLabelValues(l.values()...).Inc() }

// Produced counts one event written to a destination.
func (i *RouteInstruments) Produced(l RouteLabels) { i.produced.WithLabelValues(l.values()...).Inc() }

// DeadLettered counts one event written to the DLQ.
func (i *RouteInstruments) DeadLettered(l RouteLabels, t ErrorType) {
	i.dlq.WithLabelValues(l.values(string(t))...).Inc()
}

// SetLag records the consumer lag of one partition.
func (i *RouteInstruments) SetLag(l RouteLabels, partition string, lag int64) {
	i.lag.WithLabelValues(l.values(partition)...).Set(float64(lag))
}

// ObserveLatency records the end-to-end latency of one produced event.
func (i *RouteInstruments) ObserveLatency(l RouteLabels, d time.Duration) {
	i.latency.WithLabelValues(l.values()...).Observe(d.Seconds())
}

// ObserveStep records the duration of one pipeline step.
func (i *RouteInstruments) ObserveStep(l RouteLabels, step string, d time.Duration) {
	i.step.WithLabelValues(l.values(step)...).Observe(d.Seconds())
}

// JSTimeout counts one JavaScript execution stopped by the time limit.
func (i *RouteInstruments) JSTimeout(l RouteLabels) {
	i.jsTimeouts.WithLabelValues(l.values()...).Inc()
}

// SetJSMemory records the sandbox memory in bytes.
func (i *RouteInstruments) SetJSMemory(l RouteLabels, bytes int64) {
	i.jsMemory.WithLabelValues(l.values()...).Set(float64(bytes))
}

// Initialise exposes the runner's event counters at 0 so increase() also counts its first events.
func (i *RouteInstruments) Initialise(l RouteLabels) {
	for _, c := range []*prometheus.CounterVec{i.consumed, i.filtered, i.produced, i.jsTimeouts} {
		c.WithLabelValues(l.values()...).Add(0)
	}
}

// SetRunnerState records the runner state gauge.
func (i *RouteInstruments) SetRunnerState(l RouteLabels, s RunnerStateValue) {
	i.runnerState.WithLabelValues(l.values()...).Set(float64(s))
}

// Forget drops every series of the labels so a stopped runner leaves no stale gauges.
func (i *RouteInstruments) Forget(l RouteLabels) {
	match := l.match()
	for _, c := range []*prometheus.CounterVec{i.consumed, i.filtered, i.produced, i.dlq, i.jsTimeouts} {
		c.DeletePartialMatch(match)
	}
	for _, g := range []*prometheus.GaugeVec{i.lag, i.jsMemory, i.runnerState} {
		g.DeletePartialMatch(match)
	}
	for _, h := range []*prometheus.HistogramVec{i.latency, i.step} {
		h.DeletePartialMatch(match)
	}
}

// vecFactory builds vectors with the route labels plus extra ones.
type vecFactory struct{}

func (vecFactory) labels(extra ...MetricLabel) []string {
	out := []string{string(LabelRouteID), string(LabelBranch), string(LabelVersion), string(LabelWorker)}
	for _, l := range extra {
		out = append(out, string(l))
	}
	return out
}

func (f vecFactory) counter(n MetricName, h Help, extra ...MetricLabel) *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: string(metrics.RouterNamespace), Name: string(n), Help: string(h)}, f.labels(extra...))
}

func (f vecFactory) gauge(n MetricName, h Help, extra ...MetricLabel) *prometheus.GaugeVec {
	return prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: string(metrics.RouterNamespace), Name: string(n), Help: string(h)}, f.labels(extra...))
}

func (f vecFactory) histogram(n MetricName, h Help, buckets []float64, extra ...MetricLabel) *prometheus.HistogramVec {
	return prometheus.NewHistogramVec(prometheus.HistogramOpts{Namespace: string(metrics.RouterNamespace), Name: string(n), Help: string(h),
		Buckets: buckets}, f.labels(extra...))
}
