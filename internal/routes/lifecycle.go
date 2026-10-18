package routes

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/store"
)

// ErrSelfApproval enforces strict 4-eyes: the requester never approves their own deploy (403).
var ErrSelfApproval = problem.NewError(problem.KindForbidden, "You requested this deploy, so someone else has to approve it.")

// RolloutReporter is how route-runtime reports rollout outcomes (in-process).
type RolloutReporter interface {
	MarkActive(ctx context.Context, id DeploymentID) (Deployment, error)
	MarkFailed(ctx context.Context, id DeploymentID, reason string) (FailureOutcome, error)
}

// ApprovalDecider is how access-control applies approval decisions (P2).
type ApprovalDecider interface {
	Activate(ctx context.Context, id DeploymentID, approver uuid.UUID) (Deployment, error)
	Reject(ctx context.Context, id DeploymentID, reason string) (Deployment, error)
}

// FailureOutcome tells what happened after a failed rollout.
type FailureOutcome struct {
	Failed   Deployment  `json:"failed"`
	Rollback *Activation `json:"rollback,omitempty"`
	Paused   bool        `json:"paused"`
}

// allowedTransitions is the deployment state machine.
var allowedTransitions = map[DeploymentStatus][]DeploymentStatus{
	DeployPendingApproval: {DeployRollingOut, DeployRejected, DeployCancelled, DeploySuperseded},
	DeployRollingOut:      {DeployActive, DeployFailed, DeploySuperseded},
}

// TransitionTable answers whether a status change is allowed.
type TransitionTable struct{}

// Allows reports whether from → to is a legal transition.
func (TransitionTable) Allows(from, to DeploymentStatus) bool {
	for _, s := range allowedTransitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// DeploymentLifecycle drives deployments after creation: rollout outcomes and approval decisions.
type DeploymentLifecycle struct {
	*Core
	Activator VersionActivator
	Notices   *RolloutNotifierManager
}

// NewDeploymentLifecycle builds the lifecycle over the shared core.
func NewDeploymentLifecycle(core *Core) *DeploymentLifecycle {
	return &DeploymentLifecycle{Core: core, Activator: VersionActivator{Repos: core.Repos}}
}

// MarkActive: rolling_out → active.
func (l *DeploymentLifecycle) MarkActive(ctx context.Context, id DeploymentID) (Deployment, error) {
	return l.simple(ctx, StatusChange{ID: id, From: DeployRollingOut, To: DeployActive}, ActionRolloutActive)
}

// Reject: pending_approval → rejected; the version row stays, so its number is consumed.
func (l *DeploymentLifecycle) Reject(ctx context.Context, id DeploymentID, reason string) (Deployment, error) {
	return l.simple(ctx, StatusChange{ID: id, From: DeployPendingApproval, To: DeployRejected, Reason: reason}, ActionDeployRejected)
}

// simple applies a transition without side effects on the route.
func (l *DeploymentLifecycle) simple(ctx context.Context, c StatusChange, action audit.Action) (Deployment, error) {
	var d Deployment
	err := l.Tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		if d, err = l.transition(ctx, c); err != nil {
			return err
		}
		return l.audit(ctx, action, d)
	})
	return d, err
}

// Activate: pending_approval → rolling_out by an approver other than the requester, then go live.
func (l *DeploymentLifecycle) Activate(ctx context.Context, id DeploymentID, approver uuid.UUID) (Deployment, error) {
	var d Deployment
	var route Route
	err := l.Tx.WithinTx(ctx, func(ctx context.Context) error {
		pending, err := l.get(ctx, id)
		if err != nil {
			return err
		}
		if pending.RequestedBy != nil && *pending.RequestedBy == approver {
			return ErrSelfApproval
		}
		if d, err = l.transition(ctx, StatusChange{ID: id, From: DeployPendingApproval, To: DeployRollingOut, ApprovedBy: &approver}); err != nil {
			return err
		}
		if err := l.Repos.Deployments.SupersedeOpen(ctx, d.RouteID, SupersedeRollingOutExcept(d.ID)); err != nil {
			return err
		}
		if route, err = l.Repos.Routes.SetActiveVersion(ctx, ActivationWrite{ID: d.RouteID, VersionID: d.VersionID, Desired: DesiredRunning, UpdatedBy: &approver}); err != nil {
			return err
		}
		return l.audit(ctx, ActionDeployActivated, d)
	})
	if err != nil {
		return Deployment{}, err
	}
	l.publish(ctx, ChangeOf(route, ChangeActivated))
	return d, nil
}

// MarkFailed: rolling_out → failed; if the failed version is still live, roll back to the previous
// version as a new version (origin auto_rollback), or pause when there is none.
func (l *DeploymentLifecycle) MarkFailed(ctx context.Context, id DeploymentID, reason string) (FailureOutcome, error) {
	var out FailureOutcome
	var route Route
	err := l.Tx.WithinTx(ctx, func(ctx context.Context) error {
		failed, err := l.transition(ctx, StatusChange{ID: id, From: DeployRollingOut, To: DeployFailed, Reason: reason})
		if err != nil {
			return err
		}
		out.Failed = failed
		if err := l.audit(ctx, ActionRolloutFailed, failed); err != nil {
			return err
		}
		route, err = l.recover(ctx, failed, reason, &out)
		return err
	})
	if err != nil {
		return FailureOutcome{}, err
	}
	if out.Rollback != nil || out.Paused {
		l.publish(ctx, ChangeOf(route, l.recoveryKind(out)))
		l.Notices.Send(ctx, RolloutNoticeBuilder{}.From(route, out, reason))
	}
	return out, nil
}

// recover restores the previous version or pauses the route; a newer live version is left alone.
func (l *DeploymentLifecycle) recover(ctx context.Context, failed Deployment, reason string, out *FailureOutcome) (Route, error) {
	route, err := l.Repos.Routes.GetForUpdate(ctx, failed.RouteID)
	if err != nil || route.ActiveVersionID == nil || *route.ActiveVersionID != failed.VersionID {
		return route, err
	}
	if failed.PreviousVersionID == nil {
		out.Paused = true
		return l.Repos.Routes.SetDesiredState(ctx, route.ID, DesiredPaused, nil)
	}
	prev, err := l.Repos.Versions.GetByID(ctx, *failed.PreviousVersionID)
	if err != nil {
		return route, err
	}
	note := fmt.Sprintf(string(AutoRollbackNote), failed.VersionNo, reason, prev.No)
	act, err := l.Activator.Activate(ctx, ActivationPlan{Route: route, Config: prev.Config, Origin: OriginAutoRollback, BasedOn: &prev.ID, Note: note})
	if err != nil {
		return route, err
	}
	out.Rollback = &act
	return act.Route, l.audit(ctx, ActionRouteAutoRolledBack, act.Deployment)
}

func (DeploymentLifecycle) recoveryKind(out FailureOutcome) ChangeKind {
	if out.Rollback != nil {
		return ChangeAutoRollback
	}
	return ChangePaused
}

// transition checks the state machine and applies the guarded update.
func (l *DeploymentLifecycle) transition(ctx context.Context, c StatusChange) (Deployment, error) {
	if !(TransitionTable{}).Allows(c.From, c.To) {
		return Deployment{}, ErrTransitionNotAllowed
	}
	d, err := l.Repos.Deployments.Transition(ctx, c)
	if errors.Is(err, store.ErrNotFound) {
		return Deployment{}, ErrDeploymentNotFound
	}
	return d, err
}

func (l *DeploymentLifecycle) get(ctx context.Context, id DeploymentID) (Deployment, error) {
	d, err := l.Repos.Deployments.Get(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return Deployment{}, ErrDeploymentNotFound
	}
	return d, err
}

func (l *DeploymentLifecycle) audit(ctx context.Context, action audit.Action, d Deployment) error {
	route, err := l.Repos.Routes.Get(ctx, d.RouteID)
	if err != nil {
		return err
	}
	return l.Auditor.Record(ctx, AuditRecord{Action: action, Object: audit.ObjectDeployment, ObjectID: string(d.ID), Team: route.TeamID, After: d})
}
