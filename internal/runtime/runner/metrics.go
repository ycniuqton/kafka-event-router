package runner

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventrouter/router/internal/metrics/instruments"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/pkg/engine"
)

// MetricName names the worker-only metrics; the §12 route series come from features/monitoring instruments.
type MetricName string

const (
	MetricSkipped MetricName = "router_events_skipped_total"
	MetricReloads MetricName = "router_runner_reloads_total"
)

// MetricLabel names worker-only metric labels.
type MetricLabel string

const LabelKind MetricLabel = "kind"

// MainBranch is the branch label of routes without a typed config (tests, partial versions).
const MainBranch = "main"

// stateValues maps runner states to the monitoring router_runner_state gauge; unmapped states clear the series.
var stateValues = map[domain.RunnerState]instruments.RunnerStateValue{
	domain.RunnerStarting: instruments.RunnerRunning,
	domain.RunnerRunning:  instruments.RunnerRunning,
	domain.RunnerDraining: instruments.RunnerPaused,
	domain.RunnerPaused:   instruments.RunnerPaused,
	domain.RunnerError:    instruments.RunnerError,
}

// Metrics holds the worker-wide metric set: monitoring's RouteInstruments plus worker-only series.
type Metrics struct {
	route   *instruments.RouteInstruments
	skipped *prometheus.CounterVec
	worker  string
}

// NewMetrics registers the metric set (nil registerer = private registry, for tests).
func NewMetrics(reg prometheus.Registerer, worker string) *Metrics {
	if reg == nil {
		reg = prometheus.NewRegistry()
	}
	labels := []string{string(instruments.LabelRouteID), string(instruments.LabelBranch),
		string(instruments.LabelVersion), string(instruments.LabelWorker)}
	m := &Metrics{worker: worker, route: instruments.NewRouteInstrumentsOn(reg),
		skipped: prometheus.NewCounterVec(prometheus.CounterOpts{Name: string(MetricSkipped)}, labels)}
	reg.MustRegister(m.skipped)
	return m
}

// For binds the metric set to one runner's route version and its branches.
func (m *Metrics) For(v CompiledVersion) *RunnerMetrics {
	b := RouteBranches{}.Of(v)
	l := instruments.RouteLabels{RouteID: v.Ref.RouteID, Branch: b.Primary, Version: strconv.Itoa(v.Ref.VersionNo), Worker: m.worker}
	return &RunnerMetrics{m: m, labels: l, branches: b}
}

// RouteBranches are the branch labels of one route version: record-level series (consumed, filtered,
// latency, lag, state) carry the primary (first) branch so route sums count each record once;
// produced and branch failures carry the branch of the output.
type RouteBranches struct {
	Primary string
	All     []string
}

// Of returns the branch labels of v (MainBranch when v has no typed config).
func (RouteBranches) Of(v CompiledVersion) RouteBranches {
	if v.Config == nil || len(v.Config.Branches) == 0 {
		return RouteBranches{Primary: MainBranch, All: []string{MainBranch}}
	}
	out := RouteBranches{Primary: v.Config.Branches[0].ID}
	for _, b := range v.Config.Branches {
		out.All = append(out.All, b.ID)
	}
	return out
}

// OutcomeDetail is what a finished record contributes beyond its outcome: produced outputs per branch,
// and the DLQ error type and failing branch of a failure.
type OutcomeDetail struct {
	Produced  map[engine.BranchID]int
	ErrorType string
	Branch    engine.BranchID
}

// RunnerMetrics emits the metrics of one runner.
type RunnerMetrics struct {
	m        *Metrics
	labels   instruments.RouteLabels
	branches RouteBranches
}

// Consumed counts one polled record.
func (r *RunnerMetrics) Consumed() { r.m.route.Consumed(r.labels) }

// JSTimeout counts one in-memory step-timeout retry.
func (r *RunnerMetrics) JSTimeout() { r.m.route.JSTimeout(r.labels) }

// Outcome counts a finished record and observes its latency.
func (r *RunnerMetrics) Outcome(o domain.RecordOutcome, d OutcomeDetail, took time.Duration) {
	switch o {
	case domain.OutcomeProduced:
		r.produced(d.Produced)
	case domain.OutcomeFiltered, domain.OutcomeDropped:
		r.m.route.Filtered(r.labels)
	case domain.OutcomeDeadLettered:
		r.m.route.DeadLettered(r.branch(d.Branch), instruments.ErrorType(d.ErrorType))
	case domain.OutcomeSkipped:
		l := r.branch(d.Branch)
		r.m.skipped.WithLabelValues(l.RouteID, l.Branch, l.Version, l.Worker).Inc()
	}
	r.m.route.ObserveLatency(r.labels, took)
}

// produced counts every acked output on its branch.
func (r *RunnerMetrics) produced(perBranch map[engine.BranchID]int) {
	for b, n := range perBranch {
		l := r.branch(b)
		for i := 0; i < n; i++ {
			r.m.route.Produced(l)
		}
	}
}

// branch returns the labels of branch b (the primary branch when b is empty).
func (r *RunnerMetrics) branch(b engine.BranchID) instruments.RouteLabels {
	l := r.labels
	if b != "" {
		l.Branch = string(b)
	}
	return l
}

// Lag sets the consumer lag of one partition.
func (r *RunnerMetrics) Lag(partition int32, lag int64) {
	r.m.route.SetLag(r.labels, strconv.Itoa(int(partition)), lag)
}

// State records the runner's current state; a state without gauge value (stopped) drops every series
// of the runner, on all of its branches.
func (r *RunnerMetrics) State(_, next domain.RunnerState) {
	v, ok := stateValues[next]
	if !ok {
		r.eachBranch(r.m.route.Forget)
		return
	}
	r.eachBranch(r.m.route.Initialise)
	r.m.route.SetRunnerState(r.labels, v)
}

// eachBranch applies fn to the labels of every branch of the runner.
func (r *RunnerMetrics) eachBranch(fn func(instruments.RouteLabels)) {
	for _, b := range r.branches.All {
		fn(r.branch(engine.BranchID(b)))
	}
}
