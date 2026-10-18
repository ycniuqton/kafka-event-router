package runner

import (
	"context"
	"sync/atomic"
)

// InFlightLimiter is the semaphore of limits.maxInFlight records shared by the lanes of one runner.
type InFlightLimiter struct {
	slots chan struct{}
	inUse atomic.Int64
}

// NewInFlightLimiter sizes the limiter; non-positive sizes fall back to one slot.
func NewInFlightLimiter(size int) *InFlightLimiter {
	return &InFlightLimiter{slots: make(chan struct{}, max(size, 1))}
}

// Acquire blocks until a slot is free or ctx ends.
func (l *InFlightLimiter) Acquire(ctx context.Context) error {
	select {
	case l.slots <- struct{}{}:
		l.inUse.Add(1)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release frees one slot.
func (l *InFlightLimiter) Release() {
	select {
	case <-l.slots:
		l.inUse.Add(-1)
	default:
	}
}

// InUse returns the number of records in flight.
func (l *InFlightLimiter) InUse() int64 { return l.inUse.Load() }

// Full reports whether every slot is taken.
func (l *InFlightLimiter) Full() bool { return len(l.slots) == cap(l.slots) }
