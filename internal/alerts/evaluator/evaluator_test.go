package evaluator

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/alerts"
	"github.com/eventrouter/router/internal/metrics/query"
)

var routeID = uuid.MustParse("6f1c1f7e-1111-4c1a-9a52-1f0f7bb2c001")

func rule(kind alerts.AlertRuleKind, threshold float64, window alerts.Window) alerts.AlertRule {
	return alerts.AlertRule{ID: uuid.New(), RouteID: routeID, Kind: kind, Threshold: threshold, Window: window}
}

func TestEvaluatorPromQLGolden(t *testing.T) {
	sel := `{route_id="` + routeID.String() + `"}`
	cases := []struct {
		kind alerts.AlertRuleKind
		want string
	}{
		{alerts.KindLagAbove, `min_over_time(sum(router_consumer_lag` + sel + `)[10m:30s])`},
		{alerts.KindErrorRateAbove, `100 * sum(increase(router_events_dlq_total` + sel + `[10m])) / sum(increase(router_events_consumed_total` + sel + `[10m]))`},
		{alerts.KindThroughputZero, `sum(increase(router_events_produced_total` + sel + `[10m]))`},
		{alerts.KindMatchRateChange, `100 * abs((sum(increase(router_events_filtered_total` + sel + `[10m] offset 10m)) / sum(increase(router_events_consumed_total` + sel + `[10m] offset 10m))) - (sum(increase(router_events_filtered_total` + sel + `[10m])) / sum(increase(router_events_consumed_total` + sel + `[10m]))))`},
		{alerts.KindRouterCausedDLQ, `sum(increase(router_events_dlq_total{route_id="` + routeID.String() +
			`",error_type=~"internal_error|sandbox_crash|encode_error|produce_error|config_error|unknown"}[10m]))`},
	}
	m := NewDefaultEvaluatorManager()
	for _, c := range cases {
		ev, err := m.For(c.kind)
		if err != nil {
			t.Fatal(err)
		}
		if got := string(ev.Expr(rule(c.kind, 1, 10))); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.kind, got, c.want)
		}
	}
	if _, err := m.For("nope"); err == nil {
		t.Fatal("unknown kind must fail")
	}
}

func TestBreachedTruthTable(t *testing.T) {
	present := func(v float64, running bool) Sample { return Sample{Value: v, Present: true, RouteRunning: running} }
	absent := func(running bool) Sample { return Sample{RouteRunning: running} }
	cases := []struct {
		name string
		kind alerts.AlertRuleKind
		thr  float64
		s    Sample
		want bool
	}{
		{"lag above", alerts.KindLagAbove, 100, present(101, true), true},
		{"lag equal", alerts.KindLagAbove, 100, present(100, true), false},
		{"lag no data", alerts.KindLagAbove, 100, absent(true), false},
		{"error rate above", alerts.KindErrorRateAbove, 5, present(7.5, true), true},
		{"error rate below", alerts.KindErrorRateAbove, 5, present(1, true), false},
		{"error rate no traffic", alerts.KindErrorRateAbove, 5, absent(true), false},
		{"zero throughput running", alerts.KindThroughputZero, 0, present(0, true), true},
		{"no series running", alerts.KindThroughputZero, 0, absent(true), true},
		{"zero throughput paused", alerts.KindThroughputZero, 0, present(0, false), false},
		{"throughput flowing", alerts.KindThroughputZero, 0, present(12, true), false},
		{"match change above", alerts.KindMatchRateChange, 10, present(15, true), true},
		{"match change below", alerts.KindMatchRateChange, 10, present(3, true), false},
		{"match change no data", alerts.KindMatchRateChange, 10, absent(true), false},
		{"router-caused dead letter", alerts.KindRouterCausedDLQ, 0, present(1, true), true},
		{"no router-caused dead letter", alerts.KindRouterCausedDLQ, 0, present(0, true), false},
		{"router-caused no series", alerts.KindRouterCausedDLQ, 0, absent(false), false},
	}
	m := NewDefaultEvaluatorManager()
	for _, c := range cases {
		ev, _ := m.For(c.kind)
		if got := ev.Breached(c.s, rule(c.kind, c.thr, 5)); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestSampleReader(t *testing.T) {
	r := SampleReader{}
	if s := r.Of(nil, true); s.Present || !s.RouteRunning {
		t.Fatalf("empty: %+v", s)
	}
	if s := r.Of(query.Vector{{Value: math.NaN()}}, false); s.Present {
		t.Fatalf("NaN must be absent: %+v", s)
	}
	if s := r.Of(query.Vector{{Value: 4}}, false); !s.Present || s.Value != 4 {
		t.Fatalf("value: %+v", s)
	}
	if time.Duration(DefaultLagResolution) != 30*time.Second {
		t.Fatal("lag resolution")
	}
}
