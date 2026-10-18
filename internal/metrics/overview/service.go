// Package overview builds the route detail Overview: pipeline counts per branch and the four charts.
package overview

import (
	"context"
	"sort"
	"time"

	"github.com/eventrouter/router/internal/metrics"
	in "github.com/eventrouter/router/internal/metrics/instruments"
	"github.com/eventrouter/router/internal/metrics/query"
	"github.com/eventrouter/router/internal/routes"
)

// PipelineRow is one branch's counts along the strip: read → passed filter → sent, plus DLQ.
type PipelineRow struct {
	Branch string  `json:"branch"`
	Read   float64 `json:"read"`
	Passed float64 `json:"passed"`
	Sent   float64 `json:"sent"`
	DLQ    float64 `json:"dlq"`
}

// ChartSeries are the Overview charts, each a list of [unixSec, value] points.
type ChartSeries struct {
	EventsIn       []query.Point            `json:"eventsIn"`
	EventsOut      []query.Point            `json:"eventsOut"`
	LagByPartition map[string][]query.Point `json:"lagByPartition"`
	LatencyP50     []query.Point            `json:"latencyP50"`
	LatencyP95     []query.Point            `json:"latencyP95"`
	ErrorsPerMin   []query.Point            `json:"errorsPerMin"`
}

// RouteMetrics is GET /routes/{id}/metrics.
type RouteMetrics struct {
	Range    metrics.TimeRange `json:"range"`
	Step     string            `json:"step"`
	Pipeline []PipelineRow     `json:"pipeline"`
	Series   ChartSeries       `json:"series"`
}

// RouteLookup confirms the route exists (routes' RouteRepository).
type RouteLookup interface {
	Get(ctx context.Context, id routes.RouteID) (routes.Route, error)
}

// RouteOverviewService loads pipeline counts and chart series for one route.
type RouteOverviewService struct {
	Routes  RouteLookup
	Querier query.Querier
	Now     func() time.Time
}

// overviewResults holds the raw answers of one load.
type overviewResults struct {
	consumed, filtered, produced, dlq query.Vector
	in, out, lag, p50, p95, errors    []query.Series
}

// Load returns the route's metrics for r; a metrics-store failure is an UnavailableError (503).
func (s *RouteOverviewService) Load(ctx context.Context, id routes.RouteID, r metrics.TimeRange) (RouteMetrics, error) {
	if _, err := s.Routes.Get(ctx, id); err != nil {
		return RouteMetrics{}, err
	}
	var res overviewResults
	if err := s.batch(string(id), r, &res).Run(ctx); err != nil {
		return RouteMetrics{}, query.Unavailable(err)
	}
	return RouteMetrics{Range: r, Step: query.PromDuration(r.Step()).String(),
		Pipeline: PipelineAssembler{}.Rows(res.consumed, res.filtered, res.produced, res.dlq),
		Series: ChartSeries{EventsIn: SeriesPoints{}.First(res.in), EventsOut: SeriesPoints{}.First(res.out),
			LagByPartition: PartitionCap{Max: int(MaxPartitions)}.Apply(res.lag), LatencyP50: SeriesPoints{}.First(res.p50),
			LatencyP95: SeriesPoints{}.First(res.p95), ErrorsPerMin: SeriesPoints{}.First(res.errors)}}, nil
}

// batch schedules the 4 pipeline instants and 6 chart ranges.
func (s *RouteOverviewService) batch(id string, r metrics.TimeRange, res *overviewResults) *query.Batch {
	cat := query.ExprCatalog{}
	p := query.ForRoute(id, r.Duration(), r.RateWindow())
	w := query.WindowEnding(s.Now(), r.Duration(), r.Step())
	b := query.NewBatch(s.Querier)
	b.Instant(cat.Build(query.QueryPipelineConsumed, p), &res.consumed).Instant(cat.Build(query.QueryPipelineFiltered, p), &res.filtered).
		Instant(cat.Build(query.QueryPipelineProduced, p), &res.produced).Instant(cat.Build(query.QueryPipelineDLQ, p), &res.dlq)
	b.Range(cat.Build(query.QueryEventsIn, p), w, &res.in).Range(cat.Build(query.QueryEventsOut, p), w, &res.out).
		Range(cat.Build(query.QueryLagByPartition, p), w, &res.lag).Range(cat.Build(query.QueryLatencyP50, p), w, &res.p50).
		Range(cat.Build(query.QueryLatencyP95, p), w, &res.p95).Range(cat.Build(query.QueryErrorsPerMin, p), w, &res.errors)
	return b
}

// PipelineAssembler merges the four by-branch vectors into strip rows. Record-level series (consumed,
// filtered) carry only the primary branch, so with several branches every row reads the route's records and
// "passed" is what that branch sent or dead-lettered.
type PipelineAssembler struct{}

// Rows returns one row per branch, sorted by branch id.
func (a PipelineAssembler) Rows(consumed, filtered, produced, dlq query.Vector) []PipelineRow {
	label := string(in.LabelBranch)
	read, dropped, sent, failed := consumed.ByLabel(label), filtered.ByLabel(label), produced.ByLabel(label), dlq.ByLabel(label)
	branches := a.branches(read, dropped, sent, failed)
	out := make([]PipelineRow, 0, len(branches))
	total := consumed.Sum()
	for _, b := range branches {
		row := PipelineRow{Branch: b, Read: read[b], Passed: max(read[b]-dropped[b], 0), Sent: sent[b], DLQ: failed[b]}
		if len(branches) > 1 {
			row.Read, row.Passed = total, sent[b]+failed[b]
		}
		out = append(out, row)
	}
	return out
}

// branches returns every branch id seen in any vector, sorted.
func (PipelineAssembler) branches(maps ...map[string]float64) []string {
	seen := map[string]bool{}
	for _, m := range maps {
		for b := range m {
			seen[b] = true
		}
	}
	out := make([]string, 0, len(seen))
	for b := range seen {
		out = append(out, b)
	}
	sort.Strings(out)
	return out
}

// SeriesPoints extracts the points of single-series results.
type SeriesPoints struct{}

// First returns the first series' points, or an empty list.
func (SeriesPoints) First(s []query.Series) []query.Point {
	if len(s) == 0 {
		return []query.Point{}
	}
	return s[0].Points
}
