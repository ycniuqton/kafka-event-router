package sandbox

import (
	"context"
	"sync"
	"time"
)

// ScopedPool is one resizable pool with the route version it serves.
type ScopedPool struct {
	Scope RouteScope
	Pool  ScalablePool
}

// ScalablePools lists the pools the autoscaler manages and republishes the pool gauges after a step
// (implemented by DefaultManager).
type ScalablePools interface {
	ScalablePools() []ScopedPool
	PublishUsage()
}

// PoolAutoscaler is the worker-wide loop that resizes every autoscaled pool once per interval from its
// window (queue wait, peak use) and the worker CPU share; the memory ceiling is enforced by the pools.
type PoolAutoscaler struct {
	tuning  AutoscaleTuning
	scalers *PoolScalers
	cpu     CPUGauge
	source  ScalablePools
	mu      sync.Mutex
	state   map[ScalablePool]PoolScaler
}

// NewPoolAutoscaler builds the loop over source.
func NewPoolAutoscaler(t AutoscaleTuning, scalers *PoolScalers, cpu CPUGauge, source ScalablePools) *PoolAutoscaler {
	return &PoolAutoscaler{tuning: t, scalers: scalers, cpu: cpu, source: source, state: map[ScalablePool]PoolScaler{}}
}

// Run steps every interval until ctx ends.
func (a *PoolAutoscaler) Run(ctx context.Context) {
	tick := time.NewTicker(a.tuning.Interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick.C:
			a.Step(now)
		}
	}
}

// Step resizes every autoscaled pool once and republishes the gauges.
func (a *PoolAutoscaler) Step(now time.Time) {
	cpu := a.cpu.Busy(now)
	a.mu.Lock()
	live := map[ScalablePool]bool{}
	for _, sp := range a.source.ScalablePools() {
		if !sp.Pool.Bounds().Autoscaled() {
			continue
		}
		live[sp.Pool] = true
		a.resize(sp.Pool, cpu)
	}
	for p := range a.state {
		if !live[p] {
			delete(a.state, p)
		}
	}
	a.mu.Unlock()
	a.source.PublishUsage()
}

// resize applies the scaler's decision for one pool (caller holds mu).
func (a *PoolAutoscaler) resize(p ScalablePool, cpu float64) {
	s, ok := a.state[p]
	if !ok {
		var err error
		if s, err = a.scalers.For(p.Bounds(), a.tuning); err != nil {
			return
		}
		a.state[p] = s
	}
	p.Resize(s.Next(PoolSignal{Window: p.TakeWindow(), CPU: cpu}))
}
