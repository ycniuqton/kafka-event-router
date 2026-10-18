package dlq

import (
	"context"

	"github.com/eventrouter/router/internal/dlq/record"
)

// RetryAction re-runs records through the active version: skip resolved, lock, execute, mark, release.
type RetryAction struct {
	Gate     StateGate
	Lock     RetryLock
	Executor RetryExecutor
}

// Kind implements ActionHandler.
func (RetryAction) Kind() ActionKind { return ActionRetry }

// NeedsActiveVersion implements ActionHandler: retries run through the active version.
func (RetryAction) NeedsActiveVersion() bool { return true }

// SelectsResolved implements ActionHandler: resolved records are never acted on again.
func (RetryAction) SelectsResolved() bool { return false }

// Apply implements ActionHandler; sent → retried, filtered → filtered, failed stays unresolved.
func (a RetryAction) Apply(ctx context.Context, ac ActionContext, recs []record.Record) ActionReport {
	report := ActionReport{Kind: ActionRetry, ActiveVersionNo: ac.Target.Version.No, Delivery: RetryDeliveryOf{}.Target(ac.Target)}
	open, skipped, err := a.Gate.Split(ctx, ac.Location, recs)
	if err != nil {
		report.Results = ResultsOf{}.All(recs, ItemResult{Outcome: OutcomeFailed, ErrorType: FailDestination, Message: err.Error()})
		return report
	}
	lease, busy, err := a.Lock.Acquire(ctx, ac.Location.Route.ID, ByConnection{}.Refs(open))
	if err != nil {
		report.Results = append(skipped, ResultsOf{}.All(open, ItemResult{Outcome: OutcomeSkipped, Reason: SkipLocked})...)
		return report
	}
	defer lease.Release(ctx)
	runnable, locked := LockSplit{}.Of(open, busy)
	results := a.Executor.Execute(ctx, *ac.Target, runnable)
	report.Warnings = a.Gate.Mark(ctx, ac.Location, ResolutionsOf{}.Retry(ac, results))
	report.Results = append(append(skipped, locked...), results...)
	return report
}

// LockSplit separates records whose lock is held elsewhere.
type LockSplit struct{}

// Of returns the runnable records and skipped(locked) results.
func (LockSplit) Of(recs []record.Record, busy []Ref) ([]record.Record, []ItemResult) {
	run, locked := []record.Record{}, []ItemResult{}
	for _, r := range recs {
		if (RefsOf{}).Contains(busy, r.Ref) {
			locked = append(locked, ItemResult{Ref: r.Ref, Outcome: OutcomeSkipped, Reason: SkipLocked})
			continue
		}
		run = append(run, r)
	}
	return run, locked
}

// ResolutionsOf converts results into resolutions.
type ResolutionsOf struct{}

// Retry returns resolutions for sent and filtered results.
func (ResolutionsOf) Retry(ac ActionContext, results []ItemResult) []Resolution {
	states := map[Outcome]ResolutionState{OutcomeSent: StateRetried, OutcomeFiltered: StateFiltered}
	out := []Resolution{}
	for _, res := range results {
		st, ok := states[res.Outcome]
		if !ok {
			continue
		}
		out = append(out, Resolution{Ref: res.Ref, RouteID: ac.Location.Route.ID, State: st, By: ac.Actor.UserID, At: ac.Now, Destination: res.Destination, JobID: ac.JobID})
	}
	return out
}
