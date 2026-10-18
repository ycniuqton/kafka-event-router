// Package query reads the metrics store (Prometheus / VictoriaMetrics, PromQL) through the
// Querier port; services build expressions only through ExprCatalog.
package query

import (
	"context"
	"encoding/json"
	"time"
)

// Expr is one PromQL expression built by ExprCatalog.
type Expr string

// BackendKind selects the Querier implementation (ROUTER_METRICS_BACKEND).
type BackendKind string

const (
	BackendPrometheus BackendKind = "prometheus"
	BackendMock       BackendKind = "mock"
)

// Labels are the label set of one sample or series.
type Labels map[string]string

// Sample is one instant-vector element.
type Sample struct {
	Labels Labels
	Value  float64
}

// Vector is the result of an instant query.
type Vector []Sample

// Sum adds every sample's value.
func (v Vector) Sum() float64 {
	total := 0.0
	for _, s := range v {
		total += s.Value
	}
	return total
}

// ByLabel indexes values by one label (later duplicates win).
func (v Vector) ByLabel(label string) map[string]float64 {
	out := make(map[string]float64, len(v))
	for _, s := range v {
		out[s.Labels[label]] = s.Value
	}
	return out
}

// Point is one (time, value) pair; it encodes as [unixSec, value].
type Point struct {
	At    time.Time
	Value float64
}

// MarshalJSON renders the point as a two-element array.
func (p Point) MarshalJSON() ([]byte, error) {
	return json.Marshal([2]float64{float64(p.At.Unix()), p.Value})
}

// Series is one range-query result line.
type Series struct {
	Labels Labels
	Points []Point
}

// Window is the time span and resolution of a range query.
type Window struct {
	Start time.Time
	End   time.Time
	Step  time.Duration
}

// WindowEnding returns the window of length d ending at end.
func WindowEnding(end time.Time, d, step time.Duration) Window {
	return Window{Start: end.Add(-d), End: end, Step: step}
}

// Querier is the base of every metrics-store backend.
type Querier interface {
	Kind() BackendKind
	Instant(ctx context.Context, e Expr) (Vector, error)
	Range(ctx context.Context, e Expr, w Window) ([]Series, error)
}
