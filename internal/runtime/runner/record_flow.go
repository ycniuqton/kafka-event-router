package runner

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/runtime/delivery"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/failures"
	"github.com/eventrouter/router/internal/runtime/retry"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
)

// versionState is everything that changes on a pipeline swap.
type versionState struct {
	version   CompiledVersion
	processor *RecordProcessor
	policies  *failures.ErrorPolicyManager
	metrics   *RunnerMetrics
}

// policy returns the route's error policy.
func (s *versionState) policy() *config.ErrorPolicy { return s.version.Config.ErrorPolicy }

// RecordFlow is the per-record path of design 5.1/5.2: limiter, tracker, processing with in-memory retries,
// async delivery, error policies and halts. Swap takes the write side of swapLock between records.
type RecordFlow struct {
	state      atomic.Pointer[versionState]
	swapLock   sync.RWMutex
	delivery   delivery.Strategy
	classifier failures.ErrorClassifier
	table      failures.DispositionTable
	tracker    *OffsetTracker
	limiter    *InFlightLimiter
	gate       *BackpressureGate
	retry      retry.Policy
	halt       func(*failures.HaltError)
	halted     atomic.Bool
	cpu        atomic.Int64
	base       *batchScope
	scope      atomic.Pointer[batchScope]
	barrier    *BatchBarrier
	tail       TailHook
}

// useBase sets the runner-lifetime delivery context of the flow.
func (f *RecordFlow) useBase(ctx context.Context, cancel context.CancelFunc) {
	f.base = &batchScope{ctx: ctx, cancel: cancel}
	f.scope.Store(f.base)
}

// openBatch starts a transactional batch of n records with its own delivery context.
func (f *RecordFlow) openBatch(n int) *batchScope {
	ctx, cancel := context.WithCancel(f.base.ctx)
	b := &batchScope{ctx: ctx, cancel: cancel}
	f.barrier.Expect(n)
	f.scope.Store(b)
	return b
}

// closeBatch ends the batch context and returns to the runner-lifetime context.
func (f *RecordFlow) closeBatch(b *batchScope) {
	b.cancel()
	f.scope.Store(f.base)
}

// Handle implements RecordHandler; after a halt no further record is processed (they are re-read on resume).
func (f *RecordFlow) Handle(ctx context.Context, lane *PartitionLane, rec *kgo.Record) {
	if f.halted.Load() {
		f.barrier.Fail()
		return
	}
	if f.limiter.Acquire(ctx) != nil {
		return
	}
	f.swapLock.RLock()
	defer f.swapLock.RUnlock()
	st := f.state.Load()
	t := &recordTicket{flow: f, lane: lane, state: st, ctx: f.deliverCtx(), raw: delivery.RecordMapper{}.FromKafka(rec), started: time.Now()}
	f.tracker.Track(lane.TP(), rec.Offset)
	lane.Begin()
	st.metrics.Consumed()
	t.process()
}

// deliverCtx is the delivery context of records handled now (the open batch for exactly_once).
func (f *RecordFlow) deliverCtx() context.Context {
	return f.scope.Load().ctx
}

// recordTicket follows one record from processing to its final outcome.
type recordTicket struct {
	flow     *RecordFlow
	lane     *PartitionLane
	state    *versionState
	ctx      context.Context
	raw      engine.RawRecord
	started  time.Time
	outs     []delivery.Outgoing
	failure  *failures.Failure
	produced map[engine.BranchID]int
	pending  atomic.Int64
	ended    sync.Once
}

// process runs the pipeline and dispatches the result.
func (t *recordTicket) process() {
	ctx := t.ctx
	outs, failure := t.run(ctx)
	switch {
	case failure != nil:
		t.fail(ctx, *failure)
	case len(outs) == 0:
		t.finish(domain.OutcomeFiltered, OutcomeDetail{})
	default:
		t.deliver(ctx, outs)
	}
}

// run processes the record, retrying step timeouts in memory and transient source errors with backoff.
func (t *recordTicket) run(ctx context.Context) ([]delivery.Outgoing, *failures.Failure) {
	maxRetries := t.state.policies.MaxRetries(t.state.policy())
	for attempt := 0; ; attempt++ {
		began := time.Now()
		outs, err := t.state.processor.Process(ctx, t.raw)
		t.flow.cpu.Add(time.Since(began).Milliseconds())
		if err == nil {
			return outs, nil
		}
		f := failures.Failure{Source: t.raw, Err: err, Category: t.flow.classifier.Classify(err), Attempts: attempt + 1}
		if !t.retryable(ctx, f, attempt, maxRetries) {
			return nil, &f
		}
	}
}

// retryable decides whether to process the record again (and waits when the disposition asks for backoff).
func (t *recordTicket) retryable(ctx context.Context, f failures.Failure, attempt, maxRetries int) bool {
	switch t.flow.table.For(f.Category) {
	case domain.DispositionRetryInMemory:
		t.state.metrics.JSTimeout()
		return attempt < maxRetries
	case domain.DispositionBackoffRetry:
		return t.flow.retry.Wait(ctx, attempt) == nil
	default:
		return false
	}
}

// fail applies the disposition of a processing failure.
func (t *recordTicket) fail(ctx context.Context, f failures.Failure) {
	if t.flow.table.For(f.Category) == domain.DispositionHaltRoute {
		t.haltWith(failures.NewHalt(t.flow.table.HaltReasonOf(f.Category), f))
		return
	}
	t.failure = &f
	detail := OutcomeDetail{ErrorType: string(failures.ErrorTypes{}.Of(f.Category)), Branch: engine.BranchID(f.Branch())}
	t.state.policies.Resolve(t.state.policy()).Handle(ctx, f, func(outcome domain.RecordOutcome, err error) {
		var h *failures.HaltError
		if errors.As(err, &h) {
			t.haltWith(h)
			return
		}
		if err != nil {
			t.abandon(err)
			return
		}
		t.finish(outcome, detail)
	})
}

// deliver produces every output of every branch; the record is done when all of them are acked.
func (t *recordTicket) deliver(ctx context.Context, outs []delivery.Outgoing) {
	t.outs = outs
	t.produced = make(map[engine.BranchID]int, len(outs))
	for _, out := range outs {
		t.produced[out.Branch]++
	}
	t.pending.Store(int64(len(outs)))
	for _, out := range outs {
		t.produce(ctx, out, 0)
	}
}

// produce sends one output; transient failures pause the partition and retry after backoff.
func (t *recordTicket) produce(ctx context.Context, out delivery.Outgoing, attempt int) {
	t.flow.delivery.Produce(ctx, out, func(err error) {
		if err == nil {
			if t.pending.Add(-1) == 0 {
				t.finish(domain.OutcomeProduced, OutcomeDetail{Produced: t.produced})
			}
			return
		}
		t.produceFailed(ctx, out, attempt, err)
	})
}

// produceFailed halts on permanent destination errors and re-produces transient ones after backoff.
func (t *recordTicket) produceFailed(ctx context.Context, out delivery.Outgoing, attempt int, err error) {
	f := failures.Failure{Source: t.raw, Err: err, Category: t.flow.classifier.Classify(err), Attempts: attempt + 1}
	if ctx.Err() != nil {
		t.abandon(ctx.Err())
		return
	}
	if t.flow.table.For(f.Category) == domain.DispositionHaltRoute {
		t.haltWith(failures.NewHalt(t.flow.table.HaltReasonOf(f.Category), f))
		return
	}
	tp := t.lane.TP()
	t.flow.gate.Pause(tp, PauseBackoff)
	go func() {
		defer t.flow.gate.Resume(tp, PauseBackoff)
		if t.flow.retry.Wait(ctx, attempt) != nil {
			t.abandon(ctx.Err())
			return
		}
		t.produce(ctx, out, attempt+1)
	}()
}

// haltWith stops the route; the record stays unfinished so it is reprocessed after resume.
func (t *recordTicket) haltWith(h *failures.HaltError) {
	t.flow.halted.Store(true)
	t.flow.halt(h)
	t.abandon(h)
}

// finish marks the record done and frees its slot.
func (t *recordTicket) finish(outcome domain.RecordOutcome, detail OutcomeDetail) {
	t.ended.Do(func() {
		t.flow.tracker.MarkDone(t.lane.TP(), t.raw.Offset)
		t.state.metrics.Outcome(outcome, detail, time.Since(t.started))
		t.flow.tail.Observe(t, outcome)
		t.release()
		t.flow.barrier.Done()
	})
}

// abandon frees the slot without marking the record done (halt, stop): it will be read again.
func (t *recordTicket) abandon(error) {
	t.ended.Do(func() {
		t.release()
		t.flow.barrier.Fail()
	})
}

func (t *recordTicket) release() {
	t.lane.End()
	t.flow.limiter.Release()
}
