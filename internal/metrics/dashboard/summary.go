package dashboard

import (
	"context"
	"log/slog"
	"time"

	"github.com/eventrouter/router/internal/metrics"
	"github.com/eventrouter/router/internal/metrics/query"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/routes"
)

// SummaryWindows are the fixed windows of the health cards.
type SummaryWindows time.Duration

const (
	ThroughputWindow SummaryWindows = SummaryWindows(time.Hour)
	SparkStep        SummaryWindows = SummaryWindows(5 * time.Minute)
	DLQWindow        SummaryWindows = SummaryWindows(24 * time.Hour)
	SummaryRate      SummaryWindows = SummaryWindows(5 * time.Minute)
)

// StateCounts are the routes card numbers.
type StateCounts struct {
	Running int `json:"running"`
	Paused  int `json:"paused"`
	Error   int `json:"error"`
	Draft   int `json:"draft"`
}

// Throughput is the events-forwarded card (total over the last hour + 12 five-minute points).
type Throughput struct {
	Total float64   `json:"total"`
	Spark []float64 `json:"spark"`
}

// LagCard is the total consumer lag with its tone.
type LagCard struct {
	Value float64      `json:"value"`
	Tone  metrics.Tone `json:"tone"`
}

// CountCard is a single-number card.
type CountCard struct {
	Value float64 `json:"value"`
}

// Summary is GET /dashboard/summary; metric cards are null when the metrics store is unavailable.
type Summary struct {
	Routes           StateCounts        `json:"routes"`
	EventsLastHour   *Throughput        `json:"eventsLastHour"`
	TotalLag         *LagCard           `json:"totalLag"`
	DLQ24h           *CountCard         `json:"dlq24h"`
	Thresholds       metrics.Thresholds `json:"thresholds"`
	MetricsAvailable bool               `json:"metricsAvailable"`
}

// RouteStateCounter counts live routes per derived status.
type RouteStateCounter struct {
	Routes RouteSnapshotReader
}

// Count reads the snapshot and tallies statuses.
func (c RouteStateCounter) Count(ctx context.Context) (StateCounts, error) {
	rows, err := c.Routes.LiveRoutes(ctx)
	if err != nil {
		return StateCounts{}, err
	}
	var out StateCounts
	tally := map[routes.Status]*int{routes.StatusRunning: &out.Running, routes.StatusPaused: &out.Paused,
		routes.StatusError: &out.Error, routes.StatusDraft: &out.Draft}
	for _, r := range rows {
		if n, ok := tally[r.Status]; ok {
			*n++
		}
	}
	return out, nil
}

// SummaryService builds the health summary; a metrics-store outage degrades instead of failing.
type SummaryService struct {
	Counter    RouteStateCounter
	Querier    query.Querier
	Thresholds metrics.Thresholds
	Now        func() time.Time
	Log        *slog.Logger
}

// Build returns the summary.
func (s *SummaryService) Build(ctx context.Context) (Summary, error) {
	counts, err := s.Counter.Count(ctx)
	if err != nil {
		return Summary{}, err
	}
	out := Summary{Routes: counts, Thresholds: s.Thresholds}
	if err := s.fillMetrics(ctx, &out); err != nil {
		s.Log.Warn("dashboard: metrics unavailable", string(logging.AttrError), err)
		out.EventsLastHour, out.TotalLag, out.DLQ24h = nil, nil, nil
		return out, nil
	}
	out.MetricsAvailable = true
	return out, nil
}

// fillMetrics runs the four card queries in parallel.
func (s *SummaryService) fillMetrics(ctx context.Context, out *Summary) error {
	cat := query.ExprCatalog{}
	hour, day, rate := time.Duration(ThroughputWindow), time.Duration(DLQWindow), time.Duration(SummaryRate)
	var produced, lag, dlq query.Vector
	var spark []query.Series
	window := query.WindowEnding(s.Now(), hour-time.Duration(SparkStep), time.Duration(SparkStep))
	err := query.NewBatch(s.Querier).
		Instant(cat.Build(query.QueryProducedIncrease, query.Params{Range: hour}), &produced).
		Range(cat.Build(query.QueryEventsOut, query.Params{Rate: rate}), window, &spark).
		Instant(cat.Build(query.QueryLagTotal, query.Params{}), &lag).
		Instant(cat.Build(query.QueryDLQIncrease, query.Params{Range: day}), &dlq).Run(ctx)
	if err != nil {
		return err
	}
	out.EventsLastHour = &Throughput{Total: produced.Sum(), Spark: SparkValues{}.Of(spark)}
	out.TotalLag = &LagCard{Value: lag.Sum(), Tone: s.Thresholds.TotalTone(lag.Sum())}
	out.DLQ24h = &CountCard{Value: dlq.Sum()}
	return nil
}

// SparkValues flattens a single-series range result into plain values.
type SparkValues struct{}

// Of returns the first series' values, or an empty slice.
func (SparkValues) Of(series []query.Series) []float64 {
	out := []float64{}
	if len(series) == 0 {
		return out
	}
	for _, p := range series[0].Points {
		out = append(out, p.Value)
	}
	return out
}
