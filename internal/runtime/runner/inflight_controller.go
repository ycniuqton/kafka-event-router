package runner

import (
	"fmt"
	"time"

	"github.com/eventrouter/router/pkg/config"
)

// InFlightWindow is what one runner observed during one tuning window.
type InFlightWindow struct {
	Elapsed   time.Duration
	Acquired  int64
	Waited    int64
	WaitTotal time.Duration
	Acks      int64
	AckTotal  time.Duration
	PeakInUse int
	Limit     int
}

// AckLatency is the mean produce latency of the window (zero without acks).
func (w InFlightWindow) AckLatency() time.Duration {
	if w.Acks == 0 {
		return 0
	}
	return w.AckTotal / time.Duration(w.Acks)
}

// Throughput is the delivered records per second of the window.
func (w InFlightWindow) Throughput() float64 {
	if w.Elapsed <= 0 {
		return 0
	}
	return float64(w.Acks) / w.Elapsed.Seconds()
}

// QueueWait is the mean time a record waited for a slot, over every admitted record.
func (w InFlightWindow) QueueWait() time.Duration {
	if w.Acquired == 0 {
		return 0
	}
	return w.WaitTotal / time.Duration(w.Acquired)
}

// InFlightSignal is the input of one controller step: the window plus the worker memory pressure.
type InFlightSignal struct {
	Window   InFlightWindow
	Pressure bool
}

// InFlightController is the base of in-flight sizing policies: it turns one window into the next limit.
type InFlightController interface {
	Bounds() config.InFlightBounds
	Current() int
	Next(sig InFlightSignal) int
}

// InFlightControllerFactory builds a controller for bounds, continuing from the current limit (0 = start).
type InFlightControllerFactory func(b config.InFlightBounds, current int) InFlightController

// InFlightControllers is the registry of controllers by limits.inFlightMode.
type InFlightControllers struct {
	byMode map[config.InFlightMode]InFlightControllerFactory
}

// NewInFlightControllers registers the fixed and the gradient (auto) controllers.
func NewInFlightControllers(t GradientTuning) *InFlightControllers {
	r := &InFlightControllers{byMode: map[config.InFlightMode]InFlightControllerFactory{}}
	r.Register(config.InFlightModeFixed, func(b config.InFlightBounds, _ int) InFlightController { return FixedInFlight{bounds: b} })
	r.Register(config.InFlightModeAuto, func(b config.InFlightBounds, current int) InFlightController {
		return NewGradientInFlight(b, current, t)
	})
	return r
}

// Register adds or replaces the controller of mode.
func (r *InFlightControllers) Register(mode config.InFlightMode, f InFlightControllerFactory) {
	r.byMode[mode] = f
}

// For builds the controller of b.Mode.
func (r *InFlightControllers) For(b config.InFlightBounds, current int) (InFlightController, error) {
	f, ok := r.byMode[b.Mode]
	if !ok {
		return nil, fmt.Errorf("runtime: unknown in-flight mode %q", b.Mode)
	}
	return f(b, current), nil
}

// FixedInFlight keeps the configured size (limits.inFlightMode fixed, the pre-autotune behaviour).
type FixedInFlight struct {
	bounds config.InFlightBounds
}

func (f FixedInFlight) Bounds() config.InFlightBounds { return f.bounds }

func (f FixedInFlight) Current() int { return f.bounds.Start }

func (f FixedInFlight) Next(InFlightSignal) int { return f.bounds.Start }

// LimitClamp keeps a limit inside the bounds of a controller.
type LimitClamp struct {
	Min, Max int
}

// Apply rounds v and clamps it into [Min, Max].
func (c LimitClamp) Apply(v float64) int {
	return min(max(int(v+0.5), c.Min), c.Max)
}
