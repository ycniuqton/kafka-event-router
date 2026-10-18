package dlq

import (
	"context"

	"github.com/eventrouter/router/internal/dlq/record"
)

// RestoreAction un-resolves discarded records still inside DLQ retention (FR-16): tombstones in router.dlq.state.
type RestoreAction struct {
	Gate StateGate
}

// Kind implements ActionHandler.
func (RestoreAction) Kind() ActionKind { return ActionRestore }

// NeedsActiveVersion implements ActionHandler.
func (RestoreAction) NeedsActiveVersion() bool { return false }

// SelectsResolved implements ActionHandler: restore acts on resolved (discarded) records.
func (RestoreAction) SelectsResolved() bool { return true }

// Apply implements ActionHandler; only discarded records are restored, others are skipped(not_discarded).
func (a RestoreAction) Apply(ctx context.Context, ac ActionContext, recs []record.Record) ActionReport {
	report := ActionReport{Kind: ActionRestore}
	resolved, err := a.Gate.Lookup(ctx, ac.Location, recs)
	if err != nil {
		report.Results = ResultsOf{}.All(recs, ItemResult{Outcome: OutcomeFailed, Message: err.Error()})
		return report
	}
	discarded, skipped := (DiscardedSplit{}).Of(recs, resolved)
	if warnings := a.Gate.Clear(ctx, ac.Location, discarded); len(warnings) > 0 {
		report.Warnings = warnings
		report.Results = append(skipped, ResultsOf{}.All(discarded, ItemResult{Outcome: OutcomeFailed, Message: string(WarnStateNotSaved)})...)
		return report
	}
	report.Results = append(skipped, ResultsOf{}.All(discarded, ItemResult{Outcome: OutcomeRestored})...)
	return report
}

// DiscardedSplit separates discarded records from the rest.
type DiscardedSplit struct{}

// Of returns the discarded records and skipped(not_discarded) results for the others.
func (DiscardedSplit) Of(recs []record.Record, resolved Resolutions) ([]record.Record, []ItemResult) {
	discarded, skipped := []record.Record{}, []ItemResult{}
	for _, r := range recs {
		res := resolved.Of(r.Ref)
		if res == nil || res.State != StateDiscarded {
			skipped = append(skipped, ItemResult{Ref: r.Ref, Outcome: OutcomeSkipped, Reason: SkipNotDiscarded})
			continue
		}
		discarded = append(discarded, r)
	}
	return discarded, skipped
}
