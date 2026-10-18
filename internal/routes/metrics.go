package routes

import (
	"context"
	"fmt"
)

// MetricsSummary is the per-route throughput shown in the list (from monitoring).
type MetricsSummary struct {
	EventsPerSec float64   `json:"eventsPerSec"`
	MatchPct     float64   `json:"matchPct"`
	Lag          int64     `json:"lag"`
	Sparkline    []float64 `json:"sparkline"`
}

// RouteMetricsReader reads summary metrics; monitoring implements it (M-BE-20).
type RouteMetricsReader interface {
	Kind() MetricsReaderKind
	Summaries(ctx context.Context, ids []RouteID) (map[RouteID]MetricsSummary, error)
}

// MetricsReaderKind selects the reader through ROUTER_ROUTE_METRICS_READER.
type MetricsReaderKind string

const MetricsReaderNull MetricsReaderKind = "null"

// NullMetricsReader returns no metrics; the list renders "—".
type NullMetricsReader struct{}

func (NullMetricsReader) Kind() MetricsReaderKind { return MetricsReaderNull }

func (NullMetricsReader) Summaries(context.Context, []RouteID) (map[RouteID]MetricsSummary, error) {
	return map[RouteID]MetricsSummary{}, nil
}

// MetricsReaderRegistry holds the available readers; other folders register theirs at wiring time.
type MetricsReaderRegistry struct {
	readers map[MetricsReaderKind]RouteMetricsReader
}

// NewMetricsReaderRegistry starts with the null reader.
func NewMetricsReaderRegistry() *MetricsReaderRegistry {
	r := &MetricsReaderRegistry{readers: map[MetricsReaderKind]RouteMetricsReader{}}
	r.Register(NullMetricsReader{})
	return r
}

// Register adds a reader under its kind.
func (r *MetricsReaderRegistry) Register(reader RouteMetricsReader) {
	r.readers[reader.Kind()] = reader
}

// Select returns the configured reader.
func (r *MetricsReaderRegistry) Select(kind MetricsReaderKind) (RouteMetricsReader, error) {
	reader, ok := r.readers[kind]
	if !ok {
		return nil, fmt.Errorf("routes: no metrics reader %q registered", kind)
	}
	return reader, nil
}
