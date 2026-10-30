package runner

import (
	"time"

	"github.com/eventrouter/router/pkg/config"
)

// GradientTuning are the constants of the auto in-flight controller (design 00003-autotune §2).
type GradientTuning struct {
	Tolerance       float64
	MinGradient     float64
	Growth          float64
	MinGain         float64
	PressureBackoff float64
	LongRiseAlpha   float64
	LongFallAlpha   float64
	BindingRatio    float64
	UsageRatio      float64
	MinAcks         int64
	HoldWindows     int
}

// DefaultGradientTuning: grow 25 % per window while the limit binds and pays off; back off when the window
// latency exceeds 2× the long-term latency while the route's records fill at least half the limit (a spike on a
// mostly idle limit is the broker's, not this route's, and only holds), never below the start limit; only worker
// memory pressure halves it down to the floor.
var DefaultGradientTuning = GradientTuning{
	Tolerance:       2.0,
	MinGradient:     0.5,
	Growth:          0.25,
	MinGain:         0.03,
	PressureBackoff: 0.5,
	LongRiseAlpha:   0.05,
	LongFallAlpha:   0.25,
	BindingRatio:    0.01,
	UsageRatio:      0.5,
	MinAcks:         32,
	HoldWindows:     10,
}

// GradientInFlight is the auto controller: a latency-gradient AIMD with throughput probing. It grows the
// limit only while records wait for slots and each growth step raises throughput; it shrinks in proportion
// to the latency excess over the long-term latency, and always under memory pressure.
type GradientInFlight struct {
	bounds config.InFlightBounds
	tuning GradientTuning
	clamp  LimitClamp
	limit  int
	long   time.Duration
	probe  growthProbe
	hold   int
}

// growthProbe remembers the last growth step so the next window can judge whether it paid off.
type growthProbe struct {
	active     bool
	from       int
	throughput float64
}

// NewGradientInFlight starts at current (or b.Start when current is 0), clamped into the bounds.
func NewGradientInFlight(b config.InFlightBounds, current int, t GradientTuning) *GradientInFlight {
	c := &GradientInFlight{bounds: b, tuning: t, clamp: LimitClamp{Min: b.Min, Max: b.Max}}
	start := b.Start
	if current > 0 {
		start = current
	}
	c.limit = c.clamp.Apply(float64(start))
	return c
}

func (c *GradientInFlight) Bounds() config.InFlightBounds { return c.bounds }

// Next returns the limit of the next window.
func (c *GradientInFlight) Next(sig InFlightSignal) int {
	if sig.Pressure {
		return c.shrink(c.tuning.PressureBackoff)
	}
	w := sig.Window
	if w.Acks < c.tuning.MinAcks {
		return c.limit
	}
	short := w.AckLatency()
	c.track(short)
	if g := c.gradient(short); g < 1 {
		return c.backOff(w, g)
	}
	if c.probe.active {
		return c.judge(w)
	}
	if c.hold > 0 {
		c.hold--
		return c.limit
	}
	if c.binding(w) {
		return c.grow(w)
	}
	return c.limit
}

// track folds the window latency into the long-term latency (follows drops faster than rises).
func (c *GradientInFlight) track(short time.Duration) {
	if c.long == 0 {
		c.long = short
		return
	}
	alpha := c.tuning.LongRiseAlpha
	if short < c.long {
		alpha = c.tuning.LongFallAlpha
	}
	c.long += time.Duration(alpha * float64(short-c.long))
}

// gradient is tolerance × long / short bounded to [MinGradient, 1]; below 1 the latency is out of tolerance.
func (c *GradientInFlight) gradient(short time.Duration) float64 {
	if short <= 0 {
		return 1
	}
	g := c.tuning.Tolerance * float64(c.long) / float64(short)
	return min(max(g, c.tuning.MinGradient), 1)
}

// binding reports whether records queued for slots in this window.
func (c *GradientInFlight) binding(w InFlightWindow) bool {
	return w.Waited > 0 && float64(w.Waited) >= c.tuning.BindingRatio*float64(w.Acquired)
}

// grow raises the limit by the growth factor (at least one slot) and opens a probe.
func (c *GradientInFlight) grow(w InFlightWindow) int {
	next := c.clamp.Apply(max(float64(c.limit)*(1+c.tuning.Growth), float64(c.limit+1)))
	if next == c.limit {
		return c.limit
	}
	c.probe = growthProbe{active: true, from: c.limit, throughput: w.Throughput()}
	c.limit = next
	return c.limit
}

// judge keeps a growth that ended the queueing, grows again when it raised throughput, and otherwise
// reverts to the previous limit and holds for HoldWindows before probing again.
func (c *GradientInFlight) judge(w InFlightWindow) int {
	p := c.probe
	c.probe = growthProbe{}
	if !c.binding(w) {
		return c.limit
	}
	if w.Throughput() >= p.throughput*(1+c.tuning.MinGain) {
		return c.grow(w)
	}
	c.limit = p.from
	c.hold = c.tuning.HoldWindows
	return c.limit
}

// backOff shrinks by g when the route's own records fill the limit, never below the start limit (latency alone
// must not throttle a route under the fixed default); otherwise it cancels a probe and holds.
func (c *GradientInFlight) backOff(w InFlightWindow, g float64) int {
	if float64(w.PeakInUse) < c.tuning.UsageRatio*float64(c.limit) || c.limit <= c.bounds.Start {
		c.probe = growthProbe{}
		return c.limit
	}
	c.shrink(g)
	c.limit = max(c.limit, c.bounds.Start)
	return c.limit
}

// shrink multiplies the limit by factor, cancels any open probe and holds before the next growth.
func (c *GradientInFlight) shrink(factor float64) int {
	c.probe = growthProbe{}
	c.hold = c.tuning.HoldWindows
	c.limit = c.clamp.Apply(float64(c.limit) * factor)
	return c.limit
}

// Current returns the current limit.
func (c *GradientInFlight) Current() int { return c.limit }
