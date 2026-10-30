package runner

import (
	"context"
	"sync"
	"time"

	"github.com/eventrouter/router/internal/runtime/membudget"
	"github.com/eventrouter/router/pkg/config"
)

// The worker byte budget is the production MemoryPressure.
var _ MemoryPressure = (*membudget.Budget)(nil)

// MemoryPressure is the worker memory signal the in-flight tuner always obeys: while it reports pressure the
// limit only shrinks. The worker byte budget (route-runtime 00002-memory-budget) implements it.
type MemoryPressure interface {
	UnderPressure() bool
}

// NoMemoryPressure never reports pressure (no worker budget configured).
type NoMemoryPressure struct{}

func (NoMemoryPressure) UnderPressure() bool { return false }

// TunerTiming holds the cadence of in-flight tuning.
type TunerTiming time.Duration

// DefaultTunerInterval is the tuning window length.
const DefaultTunerInterval TunerTiming = TunerTiming(time.Second)

// InFlightTuning is the worker-wide configuration of in-flight tuning (part of Deps; zero value = defaults).
type InFlightTuning struct {
	Controllers *InFlightControllers
	Pressure    MemoryPressure
	Interval    time.Duration
}

// withDefaults fills the gradient controllers, no pressure and the default interval.
func (t InFlightTuning) withDefaults() InFlightTuning {
	if t.Controllers == nil {
		t.Controllers = NewInFlightControllers(DefaultGradientTuning)
	}
	if t.Pressure == nil {
		t.Pressure = NoMemoryPressure{}
	}
	if t.Interval <= 0 {
		t.Interval = time.Duration(DefaultTunerInterval)
	}
	return t
}

// InFlightGauge receives the current limit of a runner (router_route_inflight_limit).
type InFlightGauge interface {
	InFlightLimit(limit int)
}

// InFlightTuner closes the loop of one runner: every window it reads the limiter statistics and the
// memory pressure, asks the controller for the next limit and resizes the limiter.
type InFlightTuner struct {
	limiter  *InFlightLimiter
	tuning   InFlightTuning
	mu       sync.Mutex
	ctrl     InFlightController
	gauge    InFlightGauge
	lastSeen InFlightWindow
}

// NewInFlightTuner builds the tuner of b and a limiter sized to the controller's starting limit.
func NewInFlightTuner(t InFlightTuning, b config.InFlightBounds, gauge InFlightGauge) (*InFlightTuner, error) {
	t = t.withDefaults()
	ctrl, err := t.Controllers.For(b, 0)
	if err != nil {
		return nil, err
	}
	start := ctrl.Current()
	tuner := &InFlightTuner{limiter: NewInFlightLimiter(start), tuning: t, ctrl: ctrl, gauge: gauge}
	gauge.InFlightLimit(start)
	return tuner, nil
}

// Limiter returns the semaphore the record flow uses.
func (t *InFlightTuner) Limiter() *InFlightLimiter { return t.limiter }

// Run steps the tuner every interval until ctx ends.
func (t *InFlightTuner) Run(ctx context.Context) {
	tick := time.NewTicker(t.tuning.Interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick.C:
			t.Step(now)
		}
	}
}

// Step closes one window at now and applies the next limit; it returns the limit.
func (t *InFlightTuner) Step(now time.Time) int {
	w := t.limiter.TakeWindow(now)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastSeen = w
	next := t.ctrl.Next(InFlightSignal{Window: w, Pressure: t.tuning.Pressure.UnderPressure()})
	t.limiter.SetLimit(next)
	t.gauge.InFlightLimit(next)
	return next
}

// Rebind applies new bounds (hot reload) and a new gauge, continuing from the current limit.
func (t *InFlightTuner) Rebind(b config.InFlightBounds, gauge InFlightGauge) error {
	ctrl, err := t.tuning.Controllers.For(b, t.limiter.Limit())
	if err != nil {
		return err
	}
	start := ctrl.Current()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ctrl, t.gauge = ctrl, gauge
	t.limiter.SetLimit(start)
	gauge.InFlightLimit(start)
	return nil
}

// LastWindow returns the statistics of the latest window (status, tests).
func (t *InFlightTuner) LastWindow() InFlightWindow {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastSeen
}
