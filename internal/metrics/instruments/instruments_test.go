package instruments

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func newTestInstruments(t *testing.T) (*RouteInstruments, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	return NewRouteInstrumentsOn(reg), reg
}

func TestRouteInstrumentsRecordLabelledValues(t *testing.T) {
	i, reg := newTestInstruments(t)
	l := RouteLabels{RouteID: "r1", Branch: "main", Version: "3", Worker: "w1"}
	i.Consumed(l)
	i.Consumed(l)
	i.Filtered(l)
	i.Produced(l)
	i.DeadLettered(l, ErrorTransform)
	i.SetLag(l, "0", 42)
	i.ObserveLatency(l, 20*time.Millisecond)
	i.ObserveStep(l, "filter", time.Millisecond)
	i.JSTimeout(l)
	i.SetJSMemory(l, 1024)
	i.SetRunnerState(l, RunnerError)

	if got := testutil.ToFloat64(i.consumed.WithLabelValues("r1", "main", "3", "w1")); got != 2 {
		t.Fatalf("consumed = %v, want 2", got)
	}
	if got := testutil.ToFloat64(i.dlq.WithLabelValues("r1", "main", "3", "w1", string(ErrorTransform))); got != 1 {
		t.Fatalf("dlq = %v, want 1", got)
	}
	if got := testutil.ToFloat64(i.lag.WithLabelValues("r1", "main", "3", "w1", "0")); got != 42 {
		t.Fatalf("lag = %v, want 42", got)
	}
	if got := testutil.ToFloat64(i.runnerState.WithLabelValues("r1", "main", "3", "w1")); got != float64(RunnerError) {
		t.Fatalf("runner state = %v", got)
	}
	expected := `
# HELP router_events_filtered_total Events dropped by the route filter.
# TYPE router_events_filtered_total counter
router_events_filtered_total{branch="main",route_id="r1",version="3",worker="w1"} 1
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(expected), MetricFiltered.Full()); err != nil {
		t.Fatal(err)
	}
}

func TestForgetRemovesEverySeriesOfTheRunner(t *testing.T) {
	i, reg := newTestInstruments(t)
	gone := RouteLabels{RouteID: "r1", Branch: "main", Version: "1", Worker: "w1"}
	kept := RouteLabels{RouteID: "r2", Branch: "main", Version: "1", Worker: "w1"}
	for _, l := range []RouteLabels{gone, kept} {
		i.Consumed(l)
		i.SetLag(l, "0", 5)
		i.ObserveLatency(l, time.Second)
		i.DeadLettered(l, ErrorDecode)
	}
	i.Forget(gone)
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		for _, m := range f.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == string(LabelRouteID) && lp.GetValue() == gone.RouteID {
					t.Fatalf("%s still has a series for %s", f.GetName(), gone.RouteID)
				}
			}
		}
	}
	if got := testutil.ToFloat64(i.consumed.WithLabelValues("r2", "main", "1", "w1")); got != 1 {
		t.Fatalf("kept route lost its series: %v", got)
	}
}

func TestMetricNamesUseTheRouterNamespace(t *testing.T) {
	if MetricConsumed.Full() != "router_events_consumed_total" || MetricLatency.Bucket() != "router_end_to_end_latency_seconds_bucket" {
		t.Fatalf("unexpected names %s %s", MetricConsumed.Full(), MetricLatency.Bucket())
	}
}

func TestInitialiseExposesZeroCounters(t *testing.T) {
	i, reg := newTestInstruments(t)
	i.Initialise(RouteLabels{RouteID: "r1", Branch: "main", Version: "1", Worker: "w1"})
	expected := `
# HELP router_events_produced_total Events written to a destination topic.
# TYPE router_events_produced_total counter
router_events_produced_total{branch="main",route_id="r1",version="1",worker="w1"} 0
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(expected), MetricProduced.Full()); err != nil {
		t.Fatal(err)
	}
}
