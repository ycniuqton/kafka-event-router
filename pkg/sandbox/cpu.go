package sandbox

import (
	"runtime"
	"sync"
	"time"
)

// CPUGauge reports the busy share of the worker's cores since the previous sample (0..1 of GOMAXPROCS);
// a negative value means unknown.
type CPUGauge interface {
	Busy(now time.Time) float64
}

// UnknownCPU never knows the CPU share (platforms without rusage, tests).
type UnknownCPU struct{}

func (UnknownCPU) Busy(time.Time) float64 { return -1 }

// ProcessCPU samples the process user + system time (getrusage) against wall time and GOMAXPROCS.
type ProcessCPU struct {
	mu     sync.Mutex
	lastAt time.Time
	last   time.Duration
	read   func() (time.Duration, bool)
	cores  func() int
}

// NewProcessCPU starts sampling now.
func NewProcessCPU(now time.Time) *ProcessCPU {
	g := &ProcessCPU{read: processCPUTime, cores: func() int { return runtime.GOMAXPROCS(0) }}
	g.last, _ = g.read()
	g.lastAt = now
	return g
}

// Busy implements CPUGauge.
func (g *ProcessCPU) Busy(now time.Time) float64 {
	used, ok := g.read()
	if !ok {
		return -1
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	elapsed := now.Sub(g.lastAt)
	delta := used - g.last
	g.last, g.lastAt = used, now
	if elapsed <= 0 {
		return -1
	}
	return float64(delta) / (float64(elapsed) * float64(g.cores()))
}
