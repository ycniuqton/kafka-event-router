package runner

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/runtime/delivery"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/failures"
)

// PollCycle is the base of every way to hand one poll to the lanes; selected by the strategy's BatchMode.
type PollCycle interface {
	Batching() delivery.BatchMode
	Run(ctx context.Context, r *RouteRunner, fetches kgo.Fetches)
}

// PipelinedCycle dispatches and returns at once; offsets follow acks across polls (at_least_once).
type PipelinedCycle struct{}

func (PipelinedCycle) Batching() delivery.BatchMode { return delivery.BatchPipelined }

// Run implements PollCycle.
func (PipelinedCycle) Run(ctx context.Context, r *RouteRunner, fetches kgo.Fetches) {
	fetches.EachPartition(func(p kgo.FetchTopicPartition) { r.dispatch(ctx, p) })
}

// TransactionalCycle wraps one poll in one transaction: begin, dispatch, wait until every record of the
// poll is finished, then commit outputs + dead letters + offsets together, or abort and re-read (exactly_once).
type TransactionalCycle struct{}

func (TransactionalCycle) Batching() delivery.BatchMode { return delivery.BatchTransactional }

// Run implements PollCycle.
func (TransactionalCycle) Run(ctx context.Context, r *RouteRunner, fetches kgo.Fetches) {
	if fetches.NumRecords() == 0 {
		PipelinedCycle{}.Run(ctx, r, fetches)
		return
	}
	if err := r.strategy.BeginBatch(ctx); err != nil {
		r.signalHalt(&failures.HaltError{Reason: domain.HaltTransactionFailed, Category: domain.CategorySourceFatal, Message: err.Error()})
		return
	}
	scope := r.flow.openBatch(fetches.NumRecords())
	PipelinedCycle{}.Run(ctx, r, fetches)
	clean := r.flow.barrier.Wait(ctx, r.deps.Timings.Batch)
	if !clean {
		r.rewindBatch(scope)
	}
	endCtx, cancel := context.WithTimeout(context.Background(), r.deps.Timings.Batch)
	defer cancel()
	committed, err := r.strategy.EndBatch(endCtx, clean)
	r.flow.closeBatch(scope)
	if err != nil {
		r.signalHalt(&failures.HaltError{Reason: domain.HaltTransactionFailed, Category: domain.CategorySourceFatal, Message: err.Error()})
	}
	if !committed {
		r.forgetBatch()
		r.deps.Log.Info("runtime: transaction aborted; batch is read again", string(logging.AttrRouteID), r.Version().Ref.RouteID)
	}
}

// PollCycles selects the cycle by batch mode.
type PollCycles struct {
	byMode map[delivery.BatchMode]PollCycle
}

// NewPollCycles registers both cycles.
func NewPollCycles() PollCycles {
	m := PollCycles{byMode: map[delivery.BatchMode]PollCycle{}}
	for _, c := range []PollCycle{PipelinedCycle{}, TransactionalCycle{}} {
		m.byMode[c.Batching()] = c
	}
	return m
}

// For returns the cycle of mode (pipelined when unknown).
func (m PollCycles) For(mode delivery.BatchMode) PollCycle {
	if c, ok := m.byMode[mode]; ok {
		return c
	}
	return PipelinedCycle{}
}

// batchScope is the delivery context of the records of one batch; cancelling it abandons them.
type batchScope struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// BatchBarrier counts the records of one transactional batch until each is finished; an abandoned
// record, a lost partition or a failed produce fails the batch.
type BatchBarrier struct {
	remaining atomic.Int64
	failed    atomic.Bool
	changed   chan struct{}
}

// NewBatchBarrier builds an idle barrier.
func NewBatchBarrier() *BatchBarrier {
	return &BatchBarrier{changed: make(chan struct{}, 1)}
}

// Expect starts a batch of n records.
func (b *BatchBarrier) Expect(n int) {
	b.remaining.Store(int64(n))
	b.failed.Store(false)
	select {
	case <-b.changed:
	default:
	}
}

// Done counts one finished record.
func (b *BatchBarrier) Done() {
	b.remaining.Add(-1)
	b.notify()
}

// Fail marks the batch as failed (it must abort).
func (b *BatchBarrier) Fail() {
	b.failed.Store(true)
	b.notify()
}

// Wait returns true once every record finished, false on failure, ctx end or timeout.
func (b *BatchBarrier) Wait(ctx context.Context, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		if b.failed.Load() {
			return false
		}
		if b.remaining.Load() <= 0 {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			return false
		case <-b.changed:
		}
	}
}

func (b *BatchBarrier) notify() {
	select {
	case b.changed <- struct{}{}:
	default:
	}
}
