// Package metrics owns the monitoring domain: the worker metric set (instruments), metrics-store
// queries (query), the dashboard and route overview services and their HTTP handlers.
package metrics

import (
	"time"

	"github.com/eventrouter/router/internal/platform/problem"
)

// TimeRange is a route overview range selectable in the UI.
type TimeRange string

const (
	Range15m TimeRange = "15m"
	Range1h  TimeRange = "1h"
	Range24h TimeRange = "24h"
	Range7d  TimeRange = "7d"
)

// RangeSpec is the window, chart resolution and rate window of one range.
type RangeSpec struct {
	Duration   time.Duration
	Step       time.Duration
	RateWindow time.Duration
}

// RangeTable maps every supported range to its spec.
var RangeTable = map[TimeRange]RangeSpec{
	Range15m: {Duration: 15 * time.Minute, Step: 15 * time.Second, RateWindow: time.Minute},
	Range1h:  {Duration: time.Hour, Step: 30 * time.Second, RateWindow: 2 * time.Minute},
	Range24h: {Duration: 24 * time.Hour, Step: 5 * time.Minute, RateWindow: 10 * time.Minute},
	Range7d:  {Duration: 7 * 24 * time.Hour, Step: time.Hour, RateWindow: 2 * time.Hour},
}

// DefaultRange is used when the request names none.
const DefaultRange = Range1h

// RangeParam names the query parameter and its JSON Pointer.
type RangeParam string

const (
	ParamRange RangeParam = "range"
	PathRange  RangeParam = "/range"
)

// ErrInvalidRange is the 400 returned for an unknown range.
var ErrInvalidRange = problem.NewError(problem.KindMalformedBody, "Unknown range. Use one of 15m, 1h, 24h or 7d.").At(string(PathRange))

// ParseRange returns the range named by raw; empty means DefaultRange.
func ParseRange(raw string) (TimeRange, error) {
	if raw == "" {
		return DefaultRange, nil
	}
	r := TimeRange(raw)
	if _, ok := RangeTable[r]; !ok {
		return "", ErrInvalidRange
	}
	return r, nil
}

// Spec returns the range's spec.
func (r TimeRange) Spec() RangeSpec { return RangeTable[r] }

// Duration is the length of the range.
func (r TimeRange) Duration() time.Duration { return r.Spec().Duration }

// Step is the chart resolution of the range.
func (r TimeRange) Step() time.Duration { return r.Spec().Step }

// RateWindow is the rate() window used for per-second charts of the range.
func (r TimeRange) RateWindow() time.Duration { return r.Spec().RateWindow }
