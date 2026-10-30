package enginetest

import (
	"syscall"
	"testing"
	"time"
)

// CPUMetric names the CPU-time metrics benchmarks report next to wall-clock ns/op; they
// stay meaningful on a shared host where other processes steal the pinned core.
type CPUMetric string

const (
	MetricCPUNsPerOp      CPUMetric = "cpu-ns/op"
	MetricEventsPerCPUSec CPUMetric = "events/cpu-s"
)

// CPUClock measures the process CPU time (user + system, all threads including GC).
type CPUClock struct {
	start time.Duration
}

// StartCPUClock reads the current process CPU time.
func StartCPUClock() CPUClock { return CPUClock{start: CPUClock{}.now()} }

func (CPUClock) now() time.Duration {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}

// Report adds the CPU metrics of n operations since the clock started.
func (c CPUClock) Report(b *testing.B, n int) {
	used := c.now() - c.start
	if used <= 0 || n == 0 {
		return
	}
	perOp := float64(used.Nanoseconds()) / float64(n)
	b.ReportMetric(perOp, string(MetricCPUNsPerOp))
	b.ReportMetric(float64(time.Second)/perOp, string(MetricEventsPerCPUSec))
}
