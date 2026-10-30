package lookup

import (
	"context"
	"errors"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// BreakerState is the state of a circuit breaker (gauge value of router_lookup_breaker_state).
type BreakerState int

const (
	BreakerClosed   BreakerState = 0
	BreakerHalfOpen BreakerState = 1
	BreakerOpen     BreakerState = 2
)

// CircuitBreaker opens after Threshold consecutive transient failures, lets one probe through after
// OpenFor, and closes again on its success. A zero Threshold disables it.
type CircuitBreaker struct {
	mu        sync.Mutex
	threshold int
	openFor   time.Duration
	now       func() time.Time
	onChange  func(BreakerState)
	state     BreakerState
	failures  int
	openedAt  time.Time
	probing   bool
}

// NewCircuitBreaker builds a closed breaker; onChange (optional) observes state changes.
func NewCircuitBreaker(threshold int, openFor time.Duration, now func() time.Time, onChange func(BreakerState)) *CircuitBreaker {
	if now == nil {
		now = time.Now
	}
	if onChange == nil {
		onChange = func(BreakerState) {}
	}
	return &CircuitBreaker{threshold: threshold, openFor: openFor, now: now, onChange: onChange}
}

// Allow reports whether a call may proceed.
func (b *CircuitBreaker) Allow() bool {
	if b.threshold <= 0 {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case BreakerOpen:
		if b.now().Sub(b.openedAt) < b.openFor {
			return false
		}
		b.set(BreakerHalfOpen)
		b.probing = true
		return true
	case BreakerHalfOpen:
		if b.probing {
			return false
		}
		b.probing = true
	}
	return true
}

// Record reports the outcome of an allowed call; only transient failures count against the source.
func (b *CircuitBreaker) Record(failed bool) {
	if b.threshold <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.probing = false
	if !failed {
		b.failures = 0
		b.set(BreakerClosed)
		return
	}
	b.failures++
	if b.state == BreakerHalfOpen || b.failures >= b.threshold {
		b.openedAt = b.now()
		b.set(BreakerOpen)
	}
}

// Cancel ends an allowed call that never reached the source (no outcome is recorded).
func (b *CircuitBreaker) Cancel() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.probing = false
}

// State returns the current state.
func (b *CircuitBreaker) State() BreakerState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

func (b *CircuitBreaker) set(s BreakerState) {
	if b.state != s {
		b.state = s
		b.onChange(s)
	}
}

// ErrConcurrency is the cause when no call slot frees up before the deadline.
var ErrConcurrency = errors.New("source concurrency limit reached")

// Limiter bounds concurrent calls (semaphore) and the call rate (token bucket) of one source.
type Limiter struct {
	slots chan struct{}
	rate  *rate.Limiter
}

// NewLimiter builds the limiter of l (zero values disable each part).
func NewLimiter(l Limits) *Limiter {
	lim := &Limiter{}
	if l.MaxConcurrency > 0 {
		lim.slots = make(chan struct{}, l.MaxConcurrency)
	}
	if l.RatePerSec > 0 {
		burst := l.Burst
		if burst <= 0 {
			burst = 1
		}
		lim.rate = rate.NewLimiter(rate.Limit(l.RatePerSec), burst)
	}
	return lim
}

// Acquire waits for a token and a slot until ctx ends; release must be called after the call.
func (l *Limiter) Acquire(ctx context.Context) (func(), error) {
	if l.rate != nil {
		if err := l.rate.Wait(ctx); err != nil {
			return nil, Failures{}.Transient(ReasonRateLimited, err)
		}
	}
	if l.slots == nil {
		return func() {}, nil
	}
	select {
	case l.slots <- struct{}{}:
		return func() { <-l.slots }, nil
	case <-ctx.Done():
		return nil, Failures{}.Transient(ReasonRateLimited, errors.Join(ErrConcurrency, ctx.Err()))
	}
}
