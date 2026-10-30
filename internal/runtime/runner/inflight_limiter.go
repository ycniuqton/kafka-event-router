package runner

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// InFlightLimiter is the resizable semaphore of in-flight records shared by the lanes of one runner. Waiters
// are served in arrival order; a lowered limit never revokes taken slots, they drain on Release. It also
// collects the window statistics the InFlightTuner feeds to its controller (queue wait, ack latency).
type InFlightLimiter struct {
	mu      sync.Mutex
	limit   int
	inUse   int
	waiters []chan struct{}
	window  limiterWindow
	waits   atomic.Int64
	waitNs  atomic.Int64
	acks    atomic.Int64
	ackNs   atomic.Int64
	started time.Time
}

// limiterWindow holds the counters taken under the limiter lock since the last TakeWindow.
type limiterWindow struct {
	acquired int64
	peak     int
}

// NewInFlightLimiter sizes the limiter; non-positive sizes fall back to one slot.
func NewInFlightLimiter(size int) *InFlightLimiter {
	return &InFlightLimiter{limit: max(size, 1), started: time.Now()}
}

// Acquire blocks until a slot is free or ctx ends; the time spent waiting is the queue wait.
func (l *InFlightLimiter) Acquire(ctx context.Context) error {
	l.mu.Lock()
	if l.inUse < l.limit && len(l.waiters) == 0 {
		l.grantLocked()
		l.mu.Unlock()
		return nil
	}
	ch := make(chan struct{})
	l.waiters = append(l.waiters, ch)
	l.mu.Unlock()
	return l.wait(ctx, ch)
}

// wait parks until ch is granted; a cancelled waiter leaves the queue or hands back a late grant.
func (l *InFlightLimiter) wait(ctx context.Context, ch chan struct{}) error {
	began := time.Now()
	select {
	case <-ch:
		l.waits.Add(1)
		l.waitNs.Add(int64(time.Since(began)))
		return nil
	case <-ctx.Done():
	}
	l.mu.Lock()
	removed := l.removeWaiterLocked(ch)
	l.mu.Unlock()
	if !removed {
		l.Release()
	}
	return ctx.Err()
}

// removeWaiterLocked drops ch from the queue; false means it was already granted.
func (l *InFlightLimiter) removeWaiterLocked(ch chan struct{}) bool {
	for i, w := range l.waiters {
		if w == ch {
			l.waiters = append(l.waiters[:i], l.waiters[i+1:]...)
			return true
		}
	}
	return false
}

// grantLocked takes one slot and updates the window counters.
func (l *InFlightLimiter) grantLocked() {
	l.inUse++
	l.window.acquired++
	l.window.peak = max(l.window.peak, l.inUse)
}

// wakeLocked grants slots to queued waiters while the limit allows.
func (l *InFlightLimiter) wakeLocked() {
	for len(l.waiters) > 0 && l.inUse < l.limit {
		ch := l.waiters[0]
		l.waiters = l.waiters[1:]
		l.grantLocked()
		close(ch)
	}
}

// Release frees one slot and hands it to the oldest waiter.
func (l *InFlightLimiter) Release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inUse == 0 {
		return
	}
	l.inUse--
	l.wakeLocked()
}

// SetLimit resizes the semaphore (at least one slot); growing wakes waiters at once.
func (l *InFlightLimiter) SetLimit(n int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.limit = max(n, 1)
	l.wakeLocked()
}

// Limit returns the current size.
func (l *InFlightLimiter) Limit() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.limit
}

// ObserveAck records the produce latency of one delivered record (dispatch to last ack).
func (l *InFlightLimiter) ObserveAck(d time.Duration) {
	l.acks.Add(1)
	l.ackNs.Add(int64(d))
}

// TakeWindow returns the statistics since the previous call and starts a new window.
func (l *InFlightLimiter) TakeWindow(now time.Time) InFlightWindow {
	l.mu.Lock()
	w := InFlightWindow{Elapsed: now.Sub(l.started), Acquired: l.window.acquired, PeakInUse: l.window.peak, Limit: l.limit}
	l.window = limiterWindow{peak: l.inUse}
	l.started = now
	l.mu.Unlock()
	w.Waited, w.WaitTotal = l.waits.Swap(0), time.Duration(l.waitNs.Swap(0))
	w.Acks, w.AckTotal = l.acks.Swap(0), time.Duration(l.ackNs.Swap(0))
	return w
}

// InUse returns the number of records in flight.
func (l *InFlightLimiter) InUse() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return int64(l.inUse)
}

// Full reports whether every slot is taken.
func (l *InFlightLimiter) Full() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.inUse >= l.limit
}
