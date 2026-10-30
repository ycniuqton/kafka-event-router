package runner

import (
	"context"
	"errors"
	"time"

	"github.com/eventrouter/router/internal/runtime/delivery"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/failures"
	"github.com/eventrouter/router/internal/runtime/retrytopic"
	"github.com/eventrouter/router/pkg/engine"
)

// RetryRoute is the retry policy of one route version: planner, tier scheduler and metric labels.
// A nil *RetryRoute never diverts (routes without errorPolicy.retry).
type RetryRoute struct {
	planner   retrytopic.Planner
	scheduler retrytopic.Scheduler
	metrics   *retrytopic.Metrics
	labels    retrytopic.Labels
}

// Diverts reports whether failures of category c go to the tier topics.
func (r *RetryRoute) Diverts(c domain.ErrorCategory) bool {
	return r != nil && r.planner.Diverts(c)
}

// After decides the next step of a tier record that already used `used` attempts.
func (r *RetryRoute) After(used int, c domain.ErrorCategory) retrytopic.Decision {
	if r == nil {
		return retrytopic.DecisionNone
	}
	return r.planner.After(used, c)
}

// schedule writes f to tier `used+1` through producer; ack receives the produce outcome.
func (r *RetryRoute) schedule(ctx context.Context, producer retrytopic.Producer, src engine.RawRecord, used int, f failures.Failure, ack delivery.AckFunc) {
	et := failures.DeadLetterTypes{}.Of(f)
	s := r.scheduler
	s.Producer = producer
	s.Schedule(ctx, src, used, retrytopic.Failed{ErrorType: et, Message: f.Message()}, func(err error) {
		if err == nil {
			r.metrics.Scheduled(r.labels, et, used+1)
		}
		ack(err)
	})
}

// schedule diverts a transient failure of the source record to tier 1; a failed tier produce falls back to
// the error policy, so the record is never lost.
func (t *recordTicket) schedule(ctx context.Context, f failures.Failure) {
	t.state.retries.schedule(ctx, t.flow.delivery, t.raw, 0, f, func(err error) {
		if err != nil {
			t.applyPolicy(ctx, f)
			return
		}
		t.finish(domain.OutcomeRetryScheduled, OutcomeDetail{})
	})
}

// RetryHandler is the retry consumer's Handler: reprocess a due tier record with the current version, then
// deliver, reschedule or park it. It produces through the retry consumer's own at-least-once strategy.
type RetryHandler struct {
	flow     *RecordFlow
	strategy delivery.Strategy
	writers  func(v CompiledVersion, producer failures.Producer) failures.DLQWriter
}

// Handle implements retrytopic.Handler.
func (h *RetryHandler) Handle(ctx context.Context, env retrytopic.Envelope) error {
	if h.flow.halted.Load() {
		return ErrRunnerHalted
	}
	h.flow.swapLock.RLock()
	defer h.flow.swapLock.RUnlock()
	st := h.flow.state.Load()
	outs, err := st.processor.Process(ctx, env.Source)
	if err == nil {
		return h.deliver(ctx, st, env, outs)
	}
	f := failures.Failure{Source: env.Source, Err: err, Category: h.flow.classifier.Classify(err), RetryAttempts: env.Attempt}
	switch st.retries.After(env.Attempt, f.Category) {
	case retrytopic.DecisionSchedule:
		return AckWait{}.Run(ctx, func(ack delivery.AckFunc) { st.retries.schedule(ctx, h.strategy, env.Source, env.Attempt, f, ack) })
	case retrytopic.DecisionExhausted:
		st.retries.metrics.Exhausted(st.retries.labels, failures.DeadLetterTypes{}.Of(f))
	}
	return h.park(ctx, st, env, f)
}

// ErrRunnerHalted keeps tier records unfinished while the route is halted.
var ErrRunnerHalted = errors.New("route halted: retry postponed")

// deliver produces every output and waits for all acks; a produce error retries the tier record later.
func (h *RetryHandler) deliver(ctx context.Context, st *versionState, env retrytopic.Envelope, outs []delivery.Outgoing) error {
	produced := map[engine.BranchID]int{}
	for _, out := range outs {
		if err := (AckWait{}).Run(ctx, func(ack delivery.AckFunc) { h.strategy.Produce(ctx, out, ack) }); err != nil {
			return err
		}
		produced[out.Branch]++
	}
	outcome := domain.OutcomeProduced
	if len(outs) == 0 {
		outcome = domain.OutcomeFiltered
	}
	st.metrics.Outcome(outcome, OutcomeDetail{Produced: produced}, time.Since(env.DueAt))
	st.retries.metrics.Recovered(st.retries.labels, env.ErrorType)
	return nil
}

// park applies the route's onError to f (dlq writes router.retry_attempts); a pause halts the route.
func (h *RetryHandler) park(ctx context.Context, st *versionState, env retrytopic.Envelope, f failures.Failure) error {
	policies := failures.NewDefaultPolicyManager(h.writers(st.version, h.strategy))
	var outcome domain.RecordOutcome
	err := AckWait{}.Run(ctx, func(ack delivery.AckFunc) {
		policies.Resolve(st.policy()).Handle(ctx, f, func(o domain.RecordOutcome, err error) { outcome = o; ack(err) })
	})
	var halt *failures.HaltError
	if errors.As(err, &halt) {
		h.flow.halted.Store(true)
		h.flow.halt(halt)
		return err
	}
	if err == nil {
		detail := OutcomeDetail{ErrorType: string(failures.DeadLetterTypes{}.Of(f)), Branch: engine.BranchID(f.Branch())}
		st.metrics.Outcome(outcome, detail, time.Since(env.DueAt))
	}
	return err
}

// AckWait turns one asynchronous ack into a blocking call bounded by ctx.
type AckWait struct{}

// Run starts send and waits for its ack.
func (AckWait) Run(ctx context.Context, send func(ack delivery.AckFunc)) error {
	done := make(chan error, 1)
	send(func(err error) { done <- err })
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
