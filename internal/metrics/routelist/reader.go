// Package routelist implements routes' RouteMetricsReader over the metrics store (routes list columns).
package routelist

import (
	"context"
	"math"
	"time"

	in "github.com/eventrouter/router/internal/metrics/instruments"
	"github.com/eventrouter/router/internal/metrics/query"
	"github.com/eventrouter/router/internal/routes"
)

// ReaderKind is the ROUTER_ROUTE_METRICS_READER value selecting this reader.
const ReaderKind routes.MetricsReaderKind = "prometheus"

// ListWindows are the windows of the routes list columns.
type ListWindows time.Duration

const (
	RateWindow  ListWindows = ListWindows(5 * time.Minute)
	MatchWindow ListWindows = ListWindows(time.Hour)
	SparkWindow ListWindows = ListWindows(time.Hour)
	SparkStep   ListWindows = ListWindows(5 * time.Minute)
)

// Percent scales ratios to percentages.
type Percent float64

const FullPercent Percent = 100

// PromRouteMetricsReader reads events/s, match %, lag and a sparkline for many routes in four queries.
type PromRouteMetricsReader struct {
	Querier query.Querier
	Now     func() time.Time
}

// Kind implements routes.RouteMetricsReader.
func (r *PromRouteMetricsReader) Kind() routes.MetricsReaderKind { return ReaderKind }

// listResults holds the raw answers of one Summaries call.
type listResults struct {
	rate, consumed, filtered, lag query.Vector
	spark                         []query.Series
}

// Summaries implements routes.RouteMetricsReader.
func (r *PromRouteMetricsReader) Summaries(ctx context.Context, ids []routes.RouteID) (map[routes.RouteID]routes.MetricsSummary, error) {
	out := make(map[routes.RouteID]routes.MetricsSummary, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	raw := make([]string, len(ids))
	for i, id := range ids {
		raw[i] = string(id)
	}
	var res listResults
	if err := r.batch(raw, &res).Run(ctx); err != nil {
		return nil, err
	}
	return ListAssembler{}.Build(ids, res), nil
}

func (r *PromRouteMetricsReader) batch(ids []string, res *listResults) *query.Batch {
	cat := query.ExprCatalog{}
	rate := query.Params{RouteIDs: ids, Rate: time.Duration(RateWindow)}
	match := query.Params{RouteIDs: ids, Range: time.Duration(MatchWindow)}
	window := query.WindowEnding(r.Now(), time.Duration(SparkWindow)-time.Duration(SparkStep), time.Duration(SparkStep))
	return query.NewBatch(r.Querier).
		Instant(cat.Build(query.QueryEventsInByRoute, rate), &res.rate).
		Instant(cat.Build(query.QueryConsumedByRoute, match), &res.consumed).
		Instant(cat.Build(query.QueryFilteredByRoute, match), &res.filtered).
		Instant(cat.Build(query.QueryLagByRoute, query.Params{RouteIDs: ids}), &res.lag).
		Range(cat.Build(query.QueryEventsInByRoute, rate), window, &res.spark)
}

// ListAssembler turns the per-route vectors into summaries.
type ListAssembler struct{}

// Build returns one summary per id that has any series.
func (ListAssembler) Build(ids []routes.RouteID, res listResults) map[routes.RouteID]routes.MetricsSummary {
	label := string(in.LabelRouteID)
	rate, consumed, filtered, lag := res.rate.ByLabel(label), res.consumed.ByLabel(label), res.filtered.ByLabel(label), res.lag.ByLabel(label)
	sparks := map[string][]float64{}
	for _, s := range res.spark {
		values := make([]float64, len(s.Points))
		for i, p := range s.Points {
			values[i] = p.Value
		}
		sparks[s.Labels[label]] = values
	}
	out := make(map[routes.RouteID]routes.MetricsSummary, len(ids))
	for _, id := range ids {
		key := string(id)
		_, hasRate := rate[key]
		_, hasLag := lag[key]
		if !hasRate && !hasLag {
			continue
		}
		out[id] = routes.MetricsSummary{EventsPerSec: rate[key], MatchPct: MatchRate{}.Of(consumed[key], filtered[key]),
			Lag: int64(math.Round(lag[key])), Sparkline: append([]float64{}, sparks[key]...)}
	}
	return out
}

// MatchRate is 1 − filtered/consumed, as a percentage (100 when nothing was read).
type MatchRate struct{}

// Of returns the match percentage rounded to one decimal.
func (MatchRate) Of(consumed, filtered float64) float64 {
	if consumed <= 0 {
		return float64(FullPercent)
	}
	pct := (1 - filtered/consumed) * float64(FullPercent)
	return math.Round(max(pct, 0)*10) / 10
}
