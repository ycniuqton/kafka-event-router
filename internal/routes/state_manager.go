package routes

import (
	"context"

	"github.com/eventrouter/router/internal/audit"
)

// stateTransition describes one desired-state change requested by an operator.
type stateTransition struct {
	target DesiredState
	action audit.Action
	change ChangeKind
}

var (
	pauseTransition  = stateTransition{target: DesiredPaused, action: ActionRoutePaused, change: ChangePaused}
	resumeTransition = stateTransition{target: DesiredRunning, action: ActionRouteResumed, change: ChangeResumed}
)

// StateManager pauses and resumes deployed routes (flow 5.8); runners follow the desired state.
type StateManager struct {
	*Core
}

// NewStateManager builds the manager over the shared core.
func NewStateManager(core *Core) *StateManager {
	return &StateManager{Core: core}
}

// Pause sets desired_state = paused.
func (m *StateManager) Pause(ctx context.Context, id RouteID) (RouteDetail, error) {
	return m.apply(ctx, id, pauseTransition)
}

// Resume sets desired_state = running.
func (m *StateManager) Resume(ctx context.Context, id RouteID) (RouteDetail, error) {
	return m.apply(ctx, id, resumeTransition)
}

// apply guards the transition, writes it with its audit entry and publishes after commit.
func (m *StateManager) apply(ctx context.Context, id RouteID, t stateTransition) (RouteDetail, error) {
	before, actor, err := m.loadWritable(ctx, id)
	if err != nil {
		return RouteDetail{}, err
	}
	if err := m.guard(before); err != nil {
		return RouteDetail{}, err
	}
	if before.DesiredState == t.target {
		return m.unchanged(ctx, before, t)
	}
	var after Route
	err = m.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if after, err = m.Repos.Routes.SetDesiredState(ctx, id, t.target, actor.UserID); err != nil {
			return err
		}
		return m.Auditor.Record(ctx, AuditRecord{Action: t.action, Object: audit.ObjectRoute, ObjectID: string(id), Team: before.TeamID, Before: AuditViewOf(before), After: AuditViewOf(after)})
	})
	if err != nil {
		return RouteDetail{}, err
	}
	m.publish(ctx, ChangeOf(after, t.change))
	return m.detail(ctx, id)
}

// unchanged answers an idempotent request; the audit entry records that it was asked for.
func (m *StateManager) unchanged(ctx context.Context, r Route, t stateTransition) (RouteDetail, error) {
	rec := AuditRecord{Action: t.action, Object: audit.ObjectRoute, ObjectID: string(r.ID), Team: r.TeamID, Before: AuditViewOf(r), After: AuditViewOf(r)}
	if err := m.Auditor.Record(ctx, rec); err != nil {
		return RouteDetail{}, err
	}
	return m.detail(ctx, r.ID)
}

// guard rejects archived and never-deployed routes.
func (StateManager) guard(r Route) error {
	if r.DesiredState == DesiredArchived {
		return ErrArchived
	}
	if !r.HasActiveVersion() {
		return ErrNotDeployed
	}
	return nil
}
