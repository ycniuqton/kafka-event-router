package query

import (
	"testing"
	"time"
)

func TestExprCatalogGolden(t *testing.T) {
	one := ForRoute("r1", time.Hour, 2*time.Minute)
	cases := []struct {
		q    MetricQuery
		p    Params
		want string
	}{
		{QueryPipelineConsumed, one, `sum by (branch) (increase(router_events_consumed_total{route_id="r1"}[1h]))`},
		{QueryPipelineFiltered, one, `sum by (branch) (increase(router_events_filtered_total{route_id="r1"}[1h]))`},
		{QueryPipelineProduced, one, `sum by (branch) (increase(router_events_produced_total{route_id="r1"}[1h]))`},
		{QueryPipelineDLQ, one, `sum by (branch) (increase(router_events_dlq_total{route_id="r1"}[1h]))`},
		{QueryEventsIn, one, `sum(rate(router_events_consumed_total{route_id="r1"}[2m]))`},
		{QueryEventsOut, one, `sum(rate(router_events_produced_total{route_id="r1"}[2m]))`},
		{QueryLagByPartition, one, `sum by (partition) (router_consumer_lag{route_id="r1"})`},
		{QueryLatencyP50, one, `histogram_quantile(0.5, sum by (le) (rate(router_end_to_end_latency_seconds_bucket{route_id="r1"}[2m])))`},
		{QueryLatencyP95, one, `histogram_quantile(0.95, sum by (le) (rate(router_end_to_end_latency_seconds_bucket{route_id="r1"}[2m])))`},
		{QueryErrorsPerMin, one, `60 * sum(rate(router_events_dlq_total{route_id="r1"}[2m]))`},
		{QueryProducedIncrease, Params{Range: time.Hour}, `sum(increase(router_events_produced_total{}[1h]))`},
		{QueryDLQIncrease, Params{Range: 24 * time.Hour}, `sum(increase(router_events_dlq_total{}[1d]))`},
		{QueryLagTotal, Params{}, `sum(router_consumer_lag{})`},
		{QueryLagByRoute, Params{RouteIDs: []string{"a", "b"}}, `sum by (route_id) (router_consumer_lag{route_id=~"a|b"})`},
		{QueryLagDerivByRoute, Params{Range: 15 * time.Minute}, `deriv((sum by (route_id) (router_consumer_lag{}))[15m:])`},
		{QueryConsumedByRoute, Params{Range: 30 * time.Minute}, `sum by (route_id) (increase(router_events_consumed_total{}[30m]))`},
		{QueryFilteredByRoute, Params{Range: 30 * time.Minute}, `sum by (route_id) (increase(router_events_filtered_total{}[30m]))`},
		{QueryEventsInByRoute, Params{RouteIDs: []string{"a"}, Rate: 5 * time.Minute}, `sum by (route_id) (rate(router_events_consumed_total{route_id="a"}[5m]))`},
		{QueryAlertLagMin, Params{RouteIDs: []string{"a"}, Range: 5 * time.Minute, Step: 30 * time.Second}, `min_over_time(sum(router_consumer_lag{route_id="a"})[5m:30s])`},
		{QueryAlertErrorRate, Params{RouteIDs: []string{"a"}, Range: 10 * time.Minute}, `100 * sum(increase(router_events_dlq_total{route_id="a"}[10m])) / sum(increase(router_events_consumed_total{route_id="a"}[10m]))`},
		{QueryAlertMatchDelta, Params{RouteIDs: []string{"a"}, Range: time.Hour}, `100 * abs((sum(increase(router_events_filtered_total{route_id="a"}[1h] offset 1h)) / sum(increase(router_events_consumed_total{route_id="a"}[1h] offset 1h))) - (sum(increase(router_events_filtered_total{route_id="a"}[1h])) / sum(increase(router_events_consumed_total{route_id="a"}[1h]))))`},
		{QueryInFlightLimit, one, `sum(router_route_inflight_limit{route_id="r1"})`},
		{QueryJSInstances, one, `sum(router_js_pool_instances{route_id="r1"})`},
		{QueryJSQueueWaitP95, one, `histogram_quantile(0.95, sum by (le) (rate(router_js_queue_wait_seconds_bucket{route_id="r1"}[2m])))`},
		{QueryAlertRouterDLQ, Params{RouteIDs: []string{"a"}, Range: 15 * time.Minute, ErrorTypes: []string{"internal_error", "sandbox_crash"}},
			`sum(increase(router_events_dlq_total{route_id="a",error_type=~"internal_error|sandbox_crash"}[15m]))`},
	}
	if len(cases) != len(Templates) {
		t.Fatalf("golden covers %d of %d queries", len(cases), len(Templates))
	}
	for _, c := range cases {
		if got := string(ExprCatalog{}.Build(c.q, c.p)); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.q, got, c.want)
		}
	}
}

func TestExprCatalogPerRange(t *testing.T) {
	ranges := []struct {
		d, rate  time.Duration
		inc, rat string
	}{
		{15 * time.Minute, time.Minute, "15m", "1m"},
		{time.Hour, 2 * time.Minute, "1h", "2m"},
		{24 * time.Hour, 10 * time.Minute, "1d", "10m"},
		{7 * 24 * time.Hour, 2 * time.Hour, "7d", "2h"},
	}
	for _, r := range ranges {
		p := ForRoute("r", r.d, r.rate)
		got := string(ExprCatalog{}.Build(QueryPipelineDLQ, p))
		want := `sum by (branch) (increase(router_events_dlq_total{route_id="r"}[` + r.inc + `]))`
		if got != want {
			t.Errorf("range %s: got %s", r.inc, got)
		}
		if got := string(ExprCatalog{}.Build(QueryEventsIn, p)); got != `sum(rate(router_events_consumed_total{route_id="r"}[`+r.rat+`]))` {
			t.Errorf("rate %s: got %s", r.rat, got)
		}
	}
}

func TestRouteSelectorEscapes(t *testing.T) {
	got := RouteSelector{IDs: []string{`a"b`, "c.d"}}.String()
	if got != `route_id=~"a\"b|c\\.d"` {
		t.Fatalf("got %s", got)
	}
	if PromDuration(90*time.Second).String() != "90s" || PromDuration(0).String() != "0s" {
		t.Fatalf("durations: %s %s", PromDuration(90*time.Second), PromDuration(0))
	}
}
