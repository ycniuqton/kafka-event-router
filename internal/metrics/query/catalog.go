package query

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	in "github.com/eventrouter/router/internal/metrics/instruments"
)

// MetricQuery names one query the dashboard, overview or routes list needs.
type MetricQuery string

const (
	QueryPipelineConsumed MetricQuery = "pipeline_consumed"
	QueryPipelineFiltered MetricQuery = "pipeline_filtered"
	QueryPipelineProduced MetricQuery = "pipeline_produced"
	QueryPipelineDLQ      MetricQuery = "pipeline_dlq"
	QueryEventsIn         MetricQuery = "events_in"
	QueryEventsOut        MetricQuery = "events_out"
	QueryLagByPartition   MetricQuery = "lag_by_partition"
	QueryLatencyP50       MetricQuery = "latency_p50"
	QueryLatencyP95       MetricQuery = "latency_p95"
	QueryErrorsPerMin     MetricQuery = "errors_per_min"
	QueryProducedIncrease MetricQuery = "produced_increase"
	QueryDLQIncrease      MetricQuery = "dlq_increase"
	QueryLagTotal         MetricQuery = "lag_total"
	QueryLagByRoute       MetricQuery = "lag_by_route"
	QueryLagDerivByRoute  MetricQuery = "lag_deriv_by_route"
	QueryConsumedByRoute  MetricQuery = "consumed_by_route"
	QueryFilteredByRoute  MetricQuery = "filtered_by_route"
	QueryEventsInByRoute  MetricQuery = "events_in_by_route"
	QueryAlertLagMin      MetricQuery = "alert_lag_min"
	QueryAlertErrorRate   MetricQuery = "alert_error_rate"
	QueryAlertMatchDelta  MetricQuery = "alert_match_delta"
)

// Placeholder is a template slot ExprCatalog fills.
type Placeholder string

const (
	SlotSelector Placeholder = "{sel}"
	SlotRange    Placeholder = "{range}"
	SlotRate     Placeholder = "{rate}"
	SlotStep     Placeholder = "{step}"
)

// Params are the values of one expression: which routes, the increase window and the rate window.
type Params struct {
	RouteIDs []string
	Range    time.Duration
	Rate     time.Duration
	Step     time.Duration
}

// ForRoute returns params scoped to one route.
func ForRoute(id string, rng, rate time.Duration) Params {
	return Params{RouteIDs: []string{id}, Range: rng, Rate: rate}
}

// tpl builds templates from metric names so no metric literal appears outside instruments.
type tpl struct{}

func (tpl) sel(n in.MetricName) string    { return n.Full() + "{" + string(SlotSelector) + "}" }
func (tpl) bucket(n in.MetricName) string { return n.Bucket() + "{" + string(SlotSelector) + "}" }
func (t tpl) incBy(by in.MetricLabel, n in.MetricName) string {
	return "sum by (" + string(by) + ") (increase(" + t.sel(n) + "[" + string(SlotRange) + "]))"
}
func (t tpl) rate(n in.MetricName) string {
	return "sum(rate(" + t.sel(n) + "[" + string(SlotRate) + "]))"
}
func (t tpl) inc(n in.MetricName, offset bool) string {
	expr := "sum(increase(" + t.sel(n) + "[" + string(SlotRange) + "]"
	if offset {
		expr += " offset " + string(SlotRange)
	}
	return expr + "))"
}
func (t tpl) dropRatio(offset bool) string {
	return "(" + t.inc(in.MetricFiltered, offset) + " / " + t.inc(in.MetricConsumed, offset) + ")"
}
func (t tpl) quantile(q string) string {
	return "histogram_quantile(" + q + ", sum by (" + string(in.LabelLe) + ") (rate(" + t.bucket(in.MetricLatency) + "[" + string(SlotRate) + "])))"
}

// Templates is the PromQL of every MetricQuery (golden-tested).
var Templates = func() map[MetricQuery]string {
	t := tpl{}
	return map[MetricQuery]string{
		QueryPipelineConsumed: t.incBy(in.LabelBranch, in.MetricConsumed),
		QueryPipelineFiltered: t.incBy(in.LabelBranch, in.MetricFiltered),
		QueryPipelineProduced: t.incBy(in.LabelBranch, in.MetricProduced),
		QueryPipelineDLQ:      t.incBy(in.LabelBranch, in.MetricDLQ),
		QueryEventsIn:         t.rate(in.MetricConsumed),
		QueryEventsOut:        t.rate(in.MetricProduced),
		QueryLagByPartition:   "sum by (" + string(in.LabelPartition) + ") (" + t.sel(in.MetricConsumerLag) + ")",
		QueryLatencyP50:       t.quantile("0.5"),
		QueryLatencyP95:       t.quantile("0.95"),
		QueryErrorsPerMin:     "60 * " + t.rate(in.MetricDLQ),
		QueryProducedIncrease: "sum(increase(" + t.sel(in.MetricProduced) + "[" + string(SlotRange) + "]))",
		QueryDLQIncrease:      "sum(increase(" + t.sel(in.MetricDLQ) + "[" + string(SlotRange) + "]))",
		QueryLagTotal:         "sum(" + t.sel(in.MetricConsumerLag) + ")",
		QueryLagByRoute:       "sum by (" + string(in.LabelRouteID) + ") (" + t.sel(in.MetricConsumerLag) + ")",
		QueryLagDerivByRoute:  "deriv((sum by (" + string(in.LabelRouteID) + ") (" + t.sel(in.MetricConsumerLag) + "))[" + string(SlotRange) + ":])",
		QueryConsumedByRoute:  t.incBy(in.LabelRouteID, in.MetricConsumed),
		QueryFilteredByRoute:  t.incBy(in.LabelRouteID, in.MetricFiltered),
		QueryAlertLagMin:      "min_over_time(sum(" + t.sel(in.MetricConsumerLag) + ")[" + string(SlotRange) + ":" + string(SlotStep) + "])",
		QueryAlertErrorRate:   "100 * " + t.inc(in.MetricDLQ, false) + " / " + t.inc(in.MetricConsumed, false),
		QueryAlertMatchDelta:  "100 * abs(" + t.dropRatio(true) + " - " + t.dropRatio(false) + ")",
		QueryEventsInByRoute:  "sum by (" + string(in.LabelRouteID) + ") (rate(" + t.sel(in.MetricConsumed) + "[" + string(SlotRate) + "]))",
	}
}()

// ExprCatalog builds PromQL for a MetricQuery; services never write PromQL themselves.
type ExprCatalog struct{}

// Build fills the query's template with params.
func (ExprCatalog) Build(q MetricQuery, p Params) Expr {
	r := strings.NewReplacer(string(SlotSelector), RouteSelector{IDs: p.RouteIDs}.String(),
		string(SlotRange), PromDuration(p.Range).String(), string(SlotRate), PromDuration(p.Rate).String(),
		string(SlotStep), PromDuration(p.Step).String())
	return Expr(r.Replace(Templates[q]))
}

// RouteSelector renders the route_id matcher: none, one id, or a regex alternation.
type RouteSelector struct {
	IDs []string
}

// String implements fmt.Stringer.
func (s RouteSelector) String() string {
	switch len(s.IDs) {
	case 0:
		return ""
	case 1:
		return string(in.LabelRouteID) + `="` + s.escape(s.IDs[0]) + `"`
	}
	quoted := make([]string, len(s.IDs))
	for i, id := range s.IDs {
		quoted[i] = s.escape(regexp.QuoteMeta(id))
	}
	return string(in.LabelRouteID) + `=~"` + strings.Join(quoted, "|") + `"`
}

func (RouteSelector) escape(v string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v)
}

// PromDuration renders a duration in PromQL notation (2h, 15m, 30s).
type PromDuration time.Duration

// DurationUnit pairs a PromQL unit suffix with its length.
type DurationUnit struct {
	Suffix string
	Length time.Duration
}

// DurationUnits are tried largest first.
var DurationUnits = []DurationUnit{{"d", 24 * time.Hour}, {"h", time.Hour}, {"m", time.Minute}, {"s", time.Second}}

// String implements fmt.Stringer.
func (d PromDuration) String() string {
	v := time.Duration(d)
	for _, u := range DurationUnits {
		if v >= u.Length && v%u.Length == 0 {
			return strconv.FormatInt(int64(v/u.Length), 10) + u.Suffix
		}
	}
	return strconv.FormatInt(int64(v/time.Second), 10) + DurationUnits[len(DurationUnits)-1].Suffix
}
