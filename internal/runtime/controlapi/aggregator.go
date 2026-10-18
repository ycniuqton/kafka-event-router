package controlapi

import (
	"context"
	"errors"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/state"
	"github.com/eventrouter/router/internal/store"
)

// RuntimeStatusAggregator folds per-replica statuses into route_runtime; the 0008 trigger turns changes
// of observed state, halt and running version into router_runtime events for features/monitoring.
type RuntimeStatusAggregator struct {
	Assignments state.AssignmentStore
	Runtime     state.RuntimeStore
}

// Refresh recomputes one route's runtime row.
func (a *RuntimeStatusAggregator) Refresh(ctx context.Context, routeID string) error {
	rows, err := a.Assignments.ForRoute(ctx, routeID)
	if err != nil {
		return err
	}
	cur, err := a.Runtime.Get(ctx, routeID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	row := ReplicaFold{}.Fold(routeID, rows, cur.HaltedAt != nil)
	if cur.HaltedAt != nil && row.ErrorMessage == "" {
		row.ErrorMessage, row.ErrorCategory = cur.ErrorMessage, cur.ErrorCategory
	}
	return a.Runtime.Save(ctx, row)
}

// ReplicaFold computes the aggregated runtime of a route from its replicas.
type ReplicaFold struct{}

// Fold: halted → error; no replicas → paused; any replica error → error; any running → running; else paused.
func (ReplicaFold) Fold(routeID string, rows []state.AssignmentRow, halted bool) state.RuntimeRow {
	out := state.RuntimeRow{RouteID: routeID, Observed: domain.ObservedPaused, AssignedWorkers: []string{}}
	running, failing := false, false
	for _, r := range rows {
		out.AssignedWorkers = append(out.AssignedWorkers, r.WorkerID)
		out.Lag += r.Lag
		out.Partitions = max(out.Partitions, r.Partitions)
		if r.RunnerState == "" {
			continue
		}
		switch r.RunnerState.Observed() {
		case domain.ObservedError:
			failing = true
			out.ErrorMessage, out.ErrorCategory = r.ErrorMessage, r.ErrorCategory
		case domain.ObservedRunning:
			running = true
			out.RunningVersionID = r.ReportedVersionID
		}
	}
	out.Observed = ReplicaFold{}.observed(halted, failing, running)
	return out
}

// observed picks the observed state by precedence.
func (ReplicaFold) observed(halted, failing, running bool) domain.ObservedState {
	switch {
	case halted || failing:
		return domain.ObservedError
	case running:
		return domain.ObservedRunning
	default:
		return domain.ObservedPaused
	}
}
