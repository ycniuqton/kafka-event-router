package dlq

import (
	"context"
	"time"

	"github.com/eventrouter/router/internal/dlq/record"
)

// ActionKind names a dead-letter action.
type ActionKind string

const (
	ActionRetry   ActionKind = "retry"
	ActionDiscard ActionKind = "discard"
	ActionRestore ActionKind = "restore"
)

// Warning is a non-fatal condition reported with a response.
type Warning string

const (
	WarnStateNotSaved Warning = "state_not_saved"
	WarnTopicMissing  Warning = "dlq_topic_missing"
	WarnScanFailed    Warning = "scan_failed"
)

// Actor is who performs an action.
type Actor struct {
	UserID string
}

// ActionContext is what a handler needs besides the records.
type ActionContext struct {
	Location Location
	Actor    Actor
	Target   *RetryTarget
	Now      time.Time
	JobID    string
}

// ActionReport is the result of one action call.
type ActionReport struct {
	Kind            ActionKind
	ActiveVersionNo int
	Results         []ItemResult
	Remaining       int
	Warnings        []Warning
	Delivery        RetryDelivery
}

// Counts returns how many results have each outcome.
func (r ActionReport) Counts() map[Outcome]int {
	out := map[Outcome]int{}
	for _, o := range Outcomes {
		out[o] = 0
	}
	for _, res := range r.Results {
		out[res.Outcome]++
	}
	return out
}

// ActionHandler applies one action kind to resolved records (one business step each).
type ActionHandler interface {
	Kind() ActionKind
	NeedsActiveVersion() bool
	SelectsResolved() bool
	Apply(ctx context.Context, ac ActionContext, recs []record.Record) ActionReport
}

// StateGate looks up resolved state per DLQ connection of a location.
type StateGate struct {
	Store StateStore
}

// Split returns records not yet resolved and skipped results for resolved ones.
func (g StateGate) Split(ctx context.Context, loc Location, recs []record.Record) ([]record.Record, []ItemResult, error) {
	open, skipped := []record.Record{}, []ItemResult{}
	for conn, group := range (ByConnection{}).Records(loc, recs) {
		resolved, err := g.Store.Lookup(ctx, conn, loc.Route.ID, (ByConnection{}).Refs(group))
		if err != nil {
			return nil, nil, err
		}
		for _, r := range group {
			if _, done := resolved[r.Ref]; done {
				skipped = append(skipped, ItemResult{Ref: r.Ref, Outcome: OutcomeSkipped, Reason: SkipAlreadyResolved})
				continue
			}
			open = append(open, r)
		}
	}
	return open, skipped, nil
}

// Lookup returns the resolutions of recs over their DLQ connections.
func (g StateGate) Lookup(ctx context.Context, loc Location, recs []record.Record) (Resolutions, error) {
	out := Resolutions{}
	for conn, group := range (ByConnection{}).Records(loc, recs) {
		found, err := g.Store.Lookup(ctx, conn, loc.Route.ID, (ByConnection{}).Refs(group))
		if err != nil {
			return nil, err
		}
		for k, v := range found {
			out[k] = v
		}
	}
	return out, nil
}

// Clear removes the resolutions of recs grouped by connection; a failure is reported as a warning.
func (g StateGate) Clear(ctx context.Context, loc Location, recs []record.Record) []Warning {
	for conn, group := range (ByConnection{}).Records(loc, recs) {
		if err := g.Store.Clear(ctx, conn, loc.Route.ID, (ByConnection{}).Refs(group)); err != nil {
			return []Warning{WarnStateNotSaved}
		}
	}
	return nil
}

// Mark writes resolutions grouped by connection; a failure is reported as a warning.
func (g StateGate) Mark(ctx context.Context, loc Location, rs []Resolution) []Warning {
	byConn := map[string][]Resolution{}
	for _, r := range rs {
		if t, ok := loc.TargetOf(r.Ref.Topic); ok {
			byConn[t.ConnectionID] = append(byConn[t.ConnectionID], r)
		}
	}
	for conn, group := range byConn {
		if err := g.Store.Mark(ctx, conn, group); err != nil {
			return []Warning{WarnStateNotSaved}
		}
	}
	return nil
}

// ByConnection groups records by the DLQ connection of their topic.
type ByConnection struct{}

// Records groups recs; records of unknown topics are dropped.
func (ByConnection) Records(loc Location, recs []record.Record) map[string][]record.Record {
	out := map[string][]record.Record{}
	for _, r := range recs {
		if t, ok := loc.TargetOf(r.Ref.Topic); ok {
			out[t.ConnectionID] = append(out[t.ConnectionID], r)
		}
	}
	return out
}

// Refs returns the refs of recs.
func (ByConnection) Refs(recs []record.Record) []Ref {
	out := make([]Ref, 0, len(recs))
	for _, r := range recs {
		out = append(out, r.Ref)
	}
	return out
}

// StateFilter is the scan filter dropping resolved records (unless the query includes them).
type StateFilter struct {
	Gate     StateGate
	Location Location
	Include  bool
}

// Keep implements RecordFilter.
func (f StateFilter) Keep(ctx context.Context, recs []record.Record) ([]record.Record, error) {
	if f.Include || len(recs) == 0 {
		return recs, nil
	}
	open, _, err := f.Gate.Split(ctx, f.Location, recs)
	if err != nil {
		return nil, err
	}
	return NewestFirst{}.Sort(open), nil
}
