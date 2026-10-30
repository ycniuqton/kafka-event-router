package sandbox

import (
	"fmt"
	"time"
)

// AutoscaleTuning configures JS pool autoscaling (js-sandbox 00001-pool-autoscale): cadence, growth step,
// the share of calls that must wait for an instance before growing, idle windows before shrinking, the
// worker CPU share above which pools do not grow, the operator cap of instances per pool and the worker
// JS memory ceiling.
type AutoscaleTuning struct {
	Interval       time.Duration
	Growth         float64
	WaitRatio      float64
	IdleWindows    int
	CPUCeiling     float64
	MaxInstances   int
	MemoryCeiling  int64
	StartInstances int
}

// AutoscaleDefault are the documented defaults of the autoscaling settings.
var AutoscaleDefault = AutoscaleTuning{
	Interval:       time.Second,
	Growth:         0.25,
	WaitRatio:      0.05,
	IdleWindows:    30,
	CPUCeiling:     0.85,
	MaxInstances:   32,
	MemoryCeiling:  int64(4096 * MiB),
	StartInstances: 4,
}

// PoolSignal is the input of one scaling step: the pool's window and the worker CPU busy share (0..1 of
// GOMAXPROCS; negative when unknown, which never blocks growth).
type PoolSignal struct {
	Window PoolWindow
	CPU    float64
}

// PoolScaler is the base of pool sizing policies: it turns one window into the desired capacity.
type PoolScaler interface {
	Next(sig PoolSignal) int
}

// PoolScaleMode selects a PoolScaler.
type PoolScaleMode string

const (
	PoolScaleFixed     PoolScaleMode = "fixed"
	PoolScaleQueueWait PoolScaleMode = "queue_wait"
)

// Mode is queue_wait for autoscaled bounds, fixed otherwise.
func (b PoolBounds) Mode() PoolScaleMode {
	if b.Autoscaled() {
		return PoolScaleQueueWait
	}
	return PoolScaleFixed
}

// PoolScalerFactory builds the scaler of one pool.
type PoolScalerFactory func(b PoolBounds, t AutoscaleTuning) PoolScaler

// PoolScalers is the registry of pool scalers by mode.
type PoolScalers struct {
	byMode map[PoolScaleMode]PoolScalerFactory
}

// NewPoolScalers registers the fixed and the queue-wait scalers.
func NewPoolScalers() *PoolScalers {
	r := &PoolScalers{byMode: map[PoolScaleMode]PoolScalerFactory{}}
	r.Register(PoolScaleFixed, func(PoolBounds, AutoscaleTuning) PoolScaler { return FixedPoolScaler{} })
	r.Register(PoolScaleQueueWait, func(b PoolBounds, t AutoscaleTuning) PoolScaler { return &QueueWaitScaler{tuning: t} })
	return r
}

// Register adds or replaces the scaler of mode.
func (r *PoolScalers) Register(mode PoolScaleMode, f PoolScalerFactory) {
	r.byMode[mode] = f
}

// For builds the scaler of b.
func (r *PoolScalers) For(b PoolBounds, t AutoscaleTuning) (PoolScaler, error) {
	f, ok := r.byMode[b.Mode()]
	if !ok {
		return nil, fmt.Errorf("sandbox: unknown pool scale mode %q", b.Mode())
	}
	return f(b, t), nil
}

// FixedPoolScaler keeps the current size.
type FixedPoolScaler struct{}

func (FixedPoolScaler) Next(sig PoolSignal) int { return sig.Window.Size }

// QueueWaitScaler grows a pool while calls wait for an instance and the worker has CPU left, and shrinks it
// by one after IdleWindows windows in which fewer instances than the size were busy at once.
type QueueWaitScaler struct {
	tuning AutoscaleTuning
	idle   int
}

// Next returns the desired capacity (the pool clamps it into its bounds and the memory ceiling).
func (s *QueueWaitScaler) Next(sig PoolSignal) int {
	w := sig.Window
	if s.queued(w) {
		s.idle = 0
		if s.cpuAllows(sig.CPU) {
			return w.Size + max(1, int(float64(w.Size)*s.tuning.Growth))
		}
		return w.Size
	}
	if w.PeakInUse >= w.Size {
		s.idle = 0
		return w.Size
	}
	s.idle++
	if s.idle < s.tuning.IdleWindows {
		return w.Size
	}
	s.idle = 0
	return w.Size - 1
}

// queued reports whether enough calls waited for an instance in the window.
func (s *QueueWaitScaler) queued(w PoolWindow) bool {
	return w.Waited > 0 && float64(w.Waited) >= s.tuning.WaitRatio*float64(w.Calls)
}

// cpuAllows reports whether the worker CPU share leaves room for one more busy instance.
func (s *QueueWaitScaler) cpuAllows(cpu float64) bool {
	return cpu < 0 || cpu < s.tuning.CPUCeiling
}
