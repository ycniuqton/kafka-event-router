// Package instruments holds the worker metric set (SysDesign §12): names, labels and the
// RouteInstruments the route runtime calls. No other code path writes these series.
package instruments

import "github.com/eventrouter/router/internal/platform/metrics"

// MetricName is the short name of one router metric (without the router_ namespace).
type MetricName string

const (
	MetricConsumed     MetricName = "events_consumed_total"
	MetricFiltered     MetricName = "events_filtered_total"
	MetricProduced     MetricName = "events_produced_total"
	MetricDLQ          MetricName = "events_dlq_total"
	MetricConsumerLag  MetricName = "consumer_lag"
	MetricLatency      MetricName = "end_to_end_latency_seconds"
	MetricStepDuration MetricName = "step_duration_seconds"
	MetricJSTimeouts   MetricName = "js_timeouts_total"
	MetricJSMemory     MetricName = "js_memory_bytes"
	MetricRunnerState  MetricName = "runner_state"
)

// SeriesSuffix names the derived series of histograms.
type SeriesSuffix string

const SuffixBucket SeriesSuffix = "_bucket"

// NameSeparator joins the namespace and the metric name.
type NameSeparator string

const Separator NameSeparator = "_"

// Full returns the exposed metric name, e.g. router_events_consumed_total.
func (n MetricName) Full() string {
	return string(metrics.RouterNamespace) + string(Separator) + string(n)
}

// Bucket returns the histogram bucket series name.
func (n MetricName) Bucket() string {
	return n.Full() + string(SuffixBucket)
}

// MetricLabel names a label of the router metric set.
type MetricLabel string

const (
	LabelRouteID   MetricLabel = "route_id"
	LabelBranch    MetricLabel = "branch"
	LabelVersion   MetricLabel = "version"
	LabelWorker    MetricLabel = "worker"
	LabelPartition MetricLabel = "partition"
	LabelStep      MetricLabel = "step"
	LabelErrorType MetricLabel = "error_type"
	LabelLe        MetricLabel = "le"
)

// RunnerStateValue is the gauge value of router_runner_state.
type RunnerStateValue float64

const (
	RunnerRunning RunnerStateValue = 1
	RunnerPaused  RunnerStateValue = 2
	RunnerError   RunnerStateValue = 3
)

// ErrorType classifies dead-lettered events (label error_type).
type ErrorType string

const (
	ErrorDecode    ErrorType = "decode"
	ErrorFilter    ErrorType = "filter"
	ErrorTransform ErrorType = "transform"
	ErrorJSTimeout ErrorType = "js_timeout"
	ErrorEncode    ErrorType = "encode"
	ErrorProduce   ErrorType = "produce"
)

// Help texts of the metric set.
type Help string

const (
	HelpConsumed     Help = "Events read from the source topic."
	HelpFiltered     Help = "Events dropped by the route filter."
	HelpProduced     Help = "Events written to a destination topic."
	HelpDLQ          Help = "Events written to the dead-letter topic, by error type."
	HelpConsumerLag  Help = "Consumer lag per source partition."
	HelpLatency      Help = "Time from source record timestamp to successful produce."
	HelpStepDuration Help = "Processing time per pipeline step."
	HelpJSTimeouts   Help = "JavaScript step executions stopped by the time limit."
	HelpJSMemory     Help = "Memory used by the JavaScript sandbox."
	HelpRunnerState  Help = "Runner state: 1 running, 2 paused, 3 error."
)
