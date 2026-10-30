package sandbox

import (
	"sync"
	"time"
)

// MemoryCeiling is the worker-wide JS memory budget: every pool slot reserves its per-instance memory cap
// (limits.jsMemoryMb), so the instances a worker can run never exceed the ceiling. Growth that does not fit
// is refused; the slots a route starts with are always granted (Force) so routes keep running.
type MemoryCeiling struct {
	mu    sync.Mutex
	limit int64
	used  int64
}

// NewMemoryCeiling builds a ceiling of limit bytes; a non-positive limit never refuses.
func NewMemoryCeiling(limit int64) *MemoryCeiling {
	return &MemoryCeiling{limit: limit}
}

// TryReserve takes n bytes when they fit and reports whether it did.
func (c *MemoryCeiling) TryReserve(n int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.limit > 0 && c.used+n > c.limit {
		return false
	}
	c.used += n
	return true
}

// Force takes n bytes even above the limit (starting slots of a pool).
func (c *MemoryCeiling) Force(n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.used += n
}

// Free returns n bytes.
func (c *MemoryCeiling) Free(n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.used = max(c.used-n, 0)
}

// Used returns the reserved bytes.
func (c *MemoryCeiling) Used() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.used
}

// Limit returns the ceiling in bytes (non-positive = unlimited).
func (c *MemoryCeiling) Limit() int64 { return c.limit }

// PoolBounds are the autoscaling bounds of one pool; Min == Max (or zero) keeps a fixed size.
type PoolBounds struct {
	Min int
	Max int
}

// Autoscaled reports whether the pool may change size.
func (b PoolBounds) Autoscaled() bool { return b.Min > 0 && b.Max > b.Min }

// Clamp keeps n inside the bounds (n itself when fixed).
func (b PoolBounds) Clamp(n int) int {
	if !b.Autoscaled() {
		return n
	}
	return min(max(n, b.Min), b.Max)
}

// PoolWindow is what one pool observed since the previous window: slot grants, the grants that had to wait
// for a free instance and for how long, and the peak of concurrent calls.
type PoolWindow struct {
	Calls     int64
	Waited    int64
	WaitTotal time.Duration
	PeakInUse int
	Size      int
}
