package runner

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/runtime/membudget"
)

// RecordHandler processes one record of a lane; it returns once the record is dispatched (acks are async).
type RecordHandler interface {
	Handle(ctx context.Context, lane *PartitionLane, rec *kgo.Record)
}

// PartitionLane processes the records of one partition in order on its own goroutine.
type PartitionLane struct {
	tp       TopicPartition
	gate     *BackpressureGate
	account  MemoryAccount
	mu       sync.Mutex
	cond     *sync.Cond
	queue    []*kgo.Record
	stopped  bool
	paused   bool
	inflight atomic.Int64
	bytes    atomic.Int64
	done     chan struct{}
	cancel   context.CancelFunc
}

// NewPartitionLane builds a lane; Run starts its goroutine.
func NewPartitionLane(tp TopicPartition, gate *BackpressureGate) *PartitionLane {
	l := &PartitionLane{tp: tp, gate: gate, account: NoMemoryAccount{}, done: make(chan struct{})}
	l.cond = sync.NewCond(&l.mu)
	return l
}

// WithAccount charges the lane's records to a (the route's share of the worker memory budget).
func (l *PartitionLane) WithAccount(a MemoryAccount) *PartitionLane {
	l.account = a
	return l
}

// TP returns the lane's partition.
func (l *PartitionLane) TP() TopicPartition { return l.tp }

// Enqueue appends polled records, detached from their fetch response once the budget is filling (so the
// charge is what stays alive); a long queue pauses fetching of the partition.
func (l *PartitionLane) Enqueue(recs []*kgo.Record) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stopped {
		return
	}
	if l.account.Detaching() {
		membudget.RecordDetach{}.Detach(recs)
	}
	for _, r := range recs {
		l.bytes.Add(int64(len(r.Key) + len(r.Value)))
	}
	l.account.Charge(membudget.RecordsCost(recs))
	l.queue = append(l.queue, recs...)
	if !l.paused && l.overHighWater() {
		l.paused = true
		l.gate.Pause(l.tp, PauseQueueFull)
	}
	l.cond.Broadcast()
}

// Run hands records to h in order until Stop.
func (l *PartitionLane) Run(ctx context.Context, h RecordHandler) {
	defer close(l.done)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	l.mu.Lock()
	l.cancel = cancel
	l.mu.Unlock()
	for {
		rec, ok := l.next()
		if !ok {
			return
		}
		h.Handle(ctx, l, rec)
	}
}

// next blocks for the next record (clearing its slot, so the queue's backing array does not keep handed-out
// records alive); it resumes fetching once the queue is short again.
func (l *PartitionLane) next() (*kgo.Record, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for len(l.queue) == 0 && !l.stopped {
		l.cond.Wait()
	}
	if l.stopped {
		return nil, false
	}
	rec := l.queue[0]
	l.queue[0] = nil
	l.queue = l.queue[1:]
	l.bytes.Add(-int64(len(rec.Key) + len(rec.Value)))
	if l.paused && l.underLowWater() {
		l.paused = false
		l.gate.Resume(l.tp, PauseQueueFull)
	}
	return rec, true
}

// overHighWater reports whether the queue is long or large enough to pause fetching (caller holds mu).
func (l *PartitionLane) overHighWater() bool {
	return len(l.queue) >= int(LaneHighWater) || l.bytes.Load() >= int64(LaneHighWaterBytes)
}

// underLowWater reports whether the queue is short and small enough to resume fetching (caller holds mu).
func (l *PartitionLane) underLowWater() bool {
	return len(l.queue) <= int(LaneLowWater) && l.bytes.Load() <= int64(LaneLowWaterBytes)
}

// Stop drops queued records (they were not tracked, so they are re-read by the next owner) and ends Run.
func (l *PartitionLane) Stop() {
	l.mu.Lock()
	l.stopped = true
	l.account.Release(membudget.RecordsCost(l.queue))
	l.queue = nil
	l.bytes.Store(0)
	l.cond.Broadcast()
	cancel := l.cancel
	l.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Done is closed when Run has returned.
func (l *PartitionLane) Done() <-chan struct{} { return l.done }

// Discard returns the budget of a dequeued record that will not be processed (halt, stop).
func (l *PartitionLane) Discard(rec *kgo.Record) { l.account.Release(membudget.RecordCost(rec)) }

// Settle returns the budget of a finished or abandoned record of cost bytes.
func (l *PartitionLane) Settle(cost int64) { l.account.Release(cost) }

// Begin counts one dispatched record of the lane.
func (l *PartitionLane) Begin() { l.inflight.Add(1) }

// End releases one dispatched record of the lane.
func (l *PartitionLane) End() { l.inflight.Add(-1) }

// InFlight returns the records dispatched and not yet finished.
func (l *PartitionLane) InFlight() int64 { return l.inflight.Load() }

// QueuedBytes estimates the memory held by queued records.
func (l *PartitionLane) QueuedBytes() int64 { return l.bytes.Load() }
