package dlq

import (
	"context"

	"github.com/eventrouter/router/internal/dlq/record"
)

// DiscardAction marks records discarded (hidden from the default list).
type DiscardAction struct {
	Gate StateGate
}

// Kind implements ActionHandler.
func (DiscardAction) Kind() ActionKind { return ActionDiscard }

// NeedsActiveVersion implements ActionHandler.
func (DiscardAction) NeedsActiveVersion() bool { return false }

// SelectsResolved implements ActionHandler: resolved records are never acted on again.
func (DiscardAction) SelectsResolved() bool { return false }

// Apply implements ActionHandler.
func (a DiscardAction) Apply(ctx context.Context, ac ActionContext, recs []record.Record) ActionReport {
	report := ActionReport{Kind: ActionDiscard}
	open, skipped, err := a.Gate.Split(ctx, ac.Location, recs)
	if err != nil {
		report.Results = ResultsOf{}.All(recs, ItemResult{Outcome: OutcomeFailed, Message: err.Error()})
		return report
	}
	rs := make([]Resolution, 0, len(open))
	for _, r := range open {
		rs = append(rs, Resolution{Ref: r.Ref, RouteID: ac.Location.Route.ID, State: StateDiscarded, By: ac.Actor.UserID, At: ac.Now, JobID: ac.JobID})
	}
	if warnings := a.Gate.Mark(ctx, ac.Location, rs); len(warnings) > 0 {
		report.Warnings = warnings
		report.Results = append(skipped, ResultsOf{}.All(open, ItemResult{Outcome: OutcomeFailed, Message: string(WarnStateNotSaved)})...)
		return report
	}
	report.Results = append(skipped, ResultsOf{}.All(open, ItemResult{Outcome: OutcomeDiscarded})...)
	return report
}
