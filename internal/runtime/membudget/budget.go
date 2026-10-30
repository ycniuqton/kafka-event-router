// Package membudget bounds the memory a worker holds in records (queued in lanes or in flight) with one
// worker-wide byte budget shared fairly between routes: a route over its share while the worker is over budget
// has its source fetching paused until usage falls back under the low watermark (route-runtime 00002).
package membudget

import (
	"math"
	"sync"
	"sync/atomic"
)

// Watermark names the hysteresis of the budget, in percent.
type Watermark int64

const (
	HighWatermarkPercent Watermark = 100
	LowWatermarkPercent  Watermark = 80
	ResumeSharePercent   Watermark = 50
	DetachPercent        Watermark = 10
	Percent              Watermark = 100
)

// Pauser is the base of what the budget throttles: the source fetching of one route.
type Pauser interface {
	PauseFetch()
	ResumeFetch()
}

// Budget is the worker-wide byte budget; runners open one Account each.
type Budget struct {
	limit    int64
	high     int64
	low      int64
	used     atomic.Int64
	open     atomic.Int64
	paused   atomic.Int64
	pauses   atomic.Int64
	mu       sync.Mutex
	accounts map[*Account]struct{}
	held     map[*Account]struct{}
}

// New builds a budget of limit bytes (non-positive = unlimited).
func New(limit int64) *Budget {
	if limit <= 0 {
		limit = math.MaxInt64
	}
	return &Budget{limit: limit, high: Watermark(HighWatermarkPercent).of(limit), low: Watermark(LowWatermarkPercent).of(limit),
		accounts: map[*Account]struct{}{}, held: map[*Account]struct{}{}}
}

// OrUnlimited returns b, or an unlimited budget when b is nil (tests, tools).
func OrUnlimited(b *Budget) *Budget {
	if b == nil {
		return New(0)
	}
	return b
}

// of returns w percent of n without overflowing.
func (w Watermark) of(n int64) int64 {
	if n > math.MaxInt64/int64(Percent) {
		return n / int64(Percent) * int64(w)
	}
	return n * int64(w) / int64(Percent)
}

// Limit returns the budget in bytes.
func (b *Budget) Limit() int64 { return b.limit }

// Used returns the bytes currently held by every account.
func (b *Budget) Used() int64 { return b.used.Load() }

// PausedRoutes returns how many accounts are paused by the budget.
func (b *Budget) PausedRoutes() int64 { return b.paused.Load() }

// Pauses returns how many times the budget paused a route since start.
func (b *Budget) Pauses() int64 { return b.pauses.Load() }

// UnderPressure reports worker memory pressure for adaptive in-flight tuning: usage at or over the high
// watermark, or any route still paused by the budget (released with the same hysteresis as the pauses).
func (b *Budget) UnderPressure() bool {
	return b.used.Load() >= b.high || b.paused.Load() > 0
}

// Detaching reports whether polled records must be copied out of their fetch response: once the worker holds
// DetachPercent of the budget, records may wait long enough for their pinned response buffers to matter.
func (b *Budget) Detaching() bool {
	return b.used.Load() >= DetachPercent.of(b.limit)
}

// Share returns the fair share of one route: the budget split evenly between open accounts.
func (b *Budget) Share() int64 {
	return b.limit / max(b.open.Load(), 1)
}

// Open registers a route's account; p is paused and resumed by the budget from the account's own goroutine.
func (b *Budget) Open(route string, p Pauser) *Account {
	a := newAccount(b, route, p)
	b.mu.Lock()
	b.accounts[a] = struct{}{}
	b.open.Add(1)
	b.mu.Unlock()
	go a.apply()
	return a
}

// overShare reports whether a must pause: the worker is over the high watermark and a holds its share.
func (b *Budget) overShare(a *Account) bool {
	return b.used.Load() >= b.high && a.used.Load() >= b.Share()
}

// mayResume reports whether a paused account may fetch again.
func (b *Budget) mayResume(a *Account) bool {
	return b.used.Load() <= b.low || a.used.Load() < Watermark(ResumeSharePercent).of(b.Share())
}

// pauseIfOver pauses a when it is over its share.
func (b *Budget) pauseIfOver(a *Account) {
	b.mu.Lock()
	held := b.holdLocked(a)
	b.mu.Unlock()
	if held {
		a.sync()
	}
}

// pauseAllOver pauses every route over its share (the worker just crossed the high watermark).
func (b *Budget) pauseAllOver() {
	b.mu.Lock()
	held := []*Account{}
	for a := range b.accounts {
		if b.holdLocked(a) {
			held = append(held, a)
		}
	}
	b.mu.Unlock()
	for _, a := range held {
		a.sync()
	}
}

// holdLocked pauses a when it is over its share and not yet held, and reports it (caller holds mu).
func (b *Budget) holdLocked(a *Account) bool {
	if _, held := b.held[a]; held || !b.overShare(a) {
		return false
	}
	b.held[a] = struct{}{}
	b.paused.Add(1)
	b.pauses.Add(1)
	a.want(true)
	return true
}

// resumeEligible resumes every paused account allowed to fetch again.
func (b *Budget) resumeEligible() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for a := range b.held {
		if b.mayResume(a) {
			b.releaseLocked(a)
		}
	}
}

// releaseLocked drops the budget hold of a (caller holds mu).
func (b *Budget) releaseLocked(a *Account) {
	delete(b.held, a)
	b.paused.Add(-1)
	a.want(false)
}

// close removes a and returns its usage; the remaining routes' shares grow, so paused ones are re-evaluated.
func (b *Budget) close(a *Account) {
	b.mu.Lock()
	if _, ok := b.accounts[a]; ok {
		delete(b.accounts, a)
		b.open.Add(-1)
		if _, held := b.held[a]; held {
			b.releaseLocked(a)
		}
	}
	b.mu.Unlock()
	b.resumeEligible()
}
