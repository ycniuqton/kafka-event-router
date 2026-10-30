package membudget

import (
	"sync"
	"sync/atomic"

	"github.com/twmb/franz-go/pkg/kgo"
)

// RecordOverhead is the fixed per-record cost added to its key, value and header bytes (record struct,
// ticket, offsets bookkeeping).
type RecordOverhead int64

const RecordOverheadBytes RecordOverhead = 512

// RecordCost is the budget cost of one polled record.
func RecordCost(r *kgo.Record) int64 {
	n := int64(len(r.Key) + len(r.Value))
	for _, h := range r.Headers {
		n += int64(len(h.Key) + len(h.Value))
	}
	return n + int64(RecordOverheadBytes)
}

// RecordsCost is the budget cost of a polled batch.
func RecordsCost(recs []*kgo.Record) int64 {
	var n int64
	for _, r := range recs {
		n += RecordCost(r)
	}
	return n
}

// Detaching reports whether this route's polled records must be detached (see Budget.Detaching).
func (a *Account) Detaching() bool { return a.budget.Detaching() }

// Account is one route's share of the budget. Charge and Release are lock-free unless a decision is due. A
// pause reaches the Pauser at once from the charging (poll) goroutine so the next poll is already paused; a
// resume is applied from the account's goroutine, never under the budget lock nor inside a client callback.
type Account struct {
	budget  *Budget
	route   string
	pauser  Pauser
	used    atomic.Int64
	desired atomic.Bool
	closed  atomic.Bool
	applyMu sync.Mutex
	applied bool
	signal  chan struct{}
	done    chan struct{}
	closing sync.Once
}

func newAccount(b *Budget, route string, p Pauser) *Account {
	return &Account{budget: b, route: route, pauser: p, signal: make(chan struct{}, 1), done: make(chan struct{})}
}

// Route returns the route the account belongs to.
func (a *Account) Route() string { return a.route }

// Used returns the bytes the route holds.
func (a *Account) Used() int64 { return a.used.Load() }

// Paused reports whether the budget currently holds the route's fetching paused.
func (a *Account) Paused() bool { return a.desired.Load() }

// Charge adds n bytes of polled records; it pauses fetching when the route is over its fair share, and the
// charge that crosses the high watermark pauses every route over its share.
func (a *Account) Charge(n int64) {
	if n <= 0 || a.closed.Load() {
		return
	}
	a.used.Add(n)
	total := a.budget.used.Add(n)
	switch {
	case total >= a.budget.high && total-n < a.budget.high:
		a.budget.pauseAllOver()
	case !a.desired.Load() && a.budget.overShare(a):
		a.budget.pauseIfOver(a)
	}
}

// Release returns n bytes of finished or dropped records; paused routes are re-evaluated.
func (a *Account) Release(n int64) {
	if n <= 0 || a.closed.Load() {
		return
	}
	a.used.Add(-n)
	a.budget.used.Add(-n)
	if a.budget.paused.Load() > 0 {
		a.budget.resumeEligible()
	}
}

// Close returns everything the route still holds and leaves the budget; later charges and releases are ignored.
func (a *Account) Close() {
	a.closing.Do(func() {
		a.closed.Store(true)
		a.budget.used.Add(-a.used.Swap(0))
		a.budget.close(a)
		close(a.done)
	})
}

// want records the desired pause state (caller holds the budget lock); resumes reach the Pauser from the
// account goroutine, pauses from the charging goroutine through sync.
func (a *Account) want(paused bool) {
	a.desired.Store(paused)
	if paused {
		return
	}
	select {
	case a.signal <- struct{}{}:
	default:
	}
}

// apply runs queued resumes until the account closes.
func (a *Account) apply() {
	for {
		select {
		case <-a.done:
			return
		case <-a.signal:
			a.sync()
		}
	}
}

// sync moves the Pauser to the latest desired state; serialised, so the last decision always wins.
func (a *Account) sync() {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	want := a.desired.Load()
	if want == a.applied || a.pauser == nil {
		return
	}
	a.applied = want
	AccountTransition(want).Apply(a.pauser)
}

// AccountTransition is a pause (true) or resume (false) of a route's fetching.
type AccountTransition bool

// Apply calls the matching Pauser method.
func (t AccountTransition) Apply(p Pauser) {
	if t {
		p.PauseFetch()
		return
	}
	p.ResumeFetch()
}
