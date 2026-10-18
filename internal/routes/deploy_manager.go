package routes

import (
	"context"
	"errors"
	"fmt"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/routes/validation"
	"github.com/eventrouter/router/internal/store"
)

// RollbackNoteFormat is the default change note of a rollback.
type RollbackNoteFormat string

const (
	RollbackNote     RollbackNoteFormat = "Roll back to v%d"
	AutoRollbackNote RollbackNoteFormat = "Automatic rollback: v%d failed (%s); restored v%d"
)

// DeployResult is the 202 body of deploy and rollback.
type DeployResult struct {
	Deployment Deployment   `json:"deployment"`
	Version    RouteVersion `json:"version"`
	Route      RouteDetail  `json:"route"`
}

// DeployManager deploys drafts and rolls back to earlier versions (flows 5.5 and 5.10).
type DeployManager struct {
	*Core
	Approval  *ApprovalPolicyManager
	Activator VersionActivator
	Requests  ApprovalRequester
}

// NewDeployManager builds the manager.
func NewDeployManager(core *Core, approval *ApprovalPolicyManager) *DeployManager {
	return &DeployManager{Core: core, Approval: approval, Activator: VersionActivator{Repos: core.Repos}}
}

// Deploy validates the draft (errors block), then creates version + deployment and switches the live version.
func (m *DeployManager) Deploy(ctx context.Context, id RouteID, expected Revision, note string) (DeployResult, error) {
	route, actor, err := m.loadWritable(ctx, id)
	if err != nil {
		return DeployResult{}, err
	}
	if err := m.deployable(ctx, route); err != nil {
		return DeployResult{}, err
	}
	if err := m.validate(ctx, route, validation.ModeDeploy, route.Draft); err != nil {
		return DeployResult{}, err
	}
	needs, err := m.Approval.Requires(ctx, route, route.Draft)
	if err != nil {
		return DeployResult{}, err
	}
	plan := ActivationPlan{Config: route.Draft, Origin: OriginDeploy, Note: note, Actor: actor.UserID, NeedsApproval: needs}
	return m.run(ctx, id, &expected, plan, m.deployAction(needs), ChangeDeployed)
}

// deployable rejects archived routes and drafts identical to the live version.
func (m *DeployManager) deployable(ctx context.Context, r Route) error {
	if r.DesiredState == DesiredArchived {
		return ErrArchived
	}
	if r.Draft == nil {
		return ErrNothingToDeploy
	}
	if r.ActiveVersionID == nil {
		return nil
	}
	active, err := m.Repos.Versions.GetByID(ctx, *r.ActiveVersionID)
	if err != nil {
		return err
	}
	if (ConfigFingerprint{}).Same(active.Config, r.Draft) {
		return ErrNothingToDeploy
	}
	return nil
}

// Rollback redeploys version no as a new version (origin rollback) and replaces the draft with it.
func (m *DeployManager) Rollback(ctx context.Context, id RouteID, no int, note string) (DeployResult, error) {
	route, actor, err := m.loadWritable(ctx, id)
	if err != nil {
		return DeployResult{}, err
	}
	if route.DesiredState == DesiredArchived {
		return DeployResult{}, ErrArchived
	}
	target, err := m.Repos.Versions.GetByNo(ctx, id, no)
	if errors.Is(err, store.ErrNotFound) {
		return DeployResult{}, ErrVersionNotFound
	}
	if err != nil {
		return DeployResult{}, err
	}
	if route.ActiveVersionID != nil && *route.ActiveVersionID == target.ID {
		return DeployResult{}, ErrAlreadyActive
	}
	if err := m.validate(ctx, route, validation.ModeRollback, target.Config); err != nil {
		return DeployResult{}, err
	}
	if note == "" {
		note = fmt.Sprintf(string(RollbackNote), target.No)
	}
	plan := ActivationPlan{Config: target.Config, Origin: OriginRollback, BasedOn: &target.ID, Note: note, Actor: actor.UserID, ReplaceDraft: true}
	return m.run(ctx, id, nil, plan, ActionRouteRolledBack, ChangeRolledBack)
}

// validate runs mode and turns error issues into a 422.
func (m *DeployManager) validate(ctx context.Context, r Route, mode validation.ValidationMode, raw []byte) error {
	res := m.Validator.Run(ctx, validation.Request{Mode: mode, TeamID: string(r.TeamID), RouteID: string(r.ID), RouteName: r.Name, Raw: raw})
	if !res.Valid {
		return NewInvalidConfigError(res.Issues)
	}
	return nil
}

// run locks the route, checks the revision, activates the plan, audits and publishes after commit.
func (m *DeployManager) run(ctx context.Context, id RouteID, expected *Revision, plan ActivationPlan, action audit.Action, kind ChangeKind) (DeployResult, error) {
	var act Activation
	err := m.Tx.WithinTx(ctx, func(ctx context.Context) error {
		locked, err := m.Repos.Routes.GetForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if expected != nil && *expected != locked.Revision {
			return &store.PreconditionFailedError{Current: store.RowVersion(locked.Revision)}
		}
		plan.Route = locked
		if act, err = m.Activator.Activate(ctx, plan); err != nil {
			return err
		}
		return m.Auditor.Record(ctx, AuditRecord{Action: action, Object: audit.ObjectRouteVersion, ObjectID: string(act.Version.ID), Team: locked.TeamID, After: VersionAuditView{Route: locked.ID, No: act.Version.No, Origin: act.Version.Origin, Deployment: act.Deployment.Status, Note: plan.Note}})
	})
	if err != nil {
		return DeployResult{}, err
	}
	m.announce(ctx, plan, act, kind)
	detail, err := m.detail(ctx, id)
	return DeployResult{Deployment: act.Deployment, Version: act.Version, Route: detail}, err
}

// announce tells workers about a new live version, or access-control about a pending approval request.
func (m *DeployManager) announce(ctx context.Context, plan ActivationPlan, act Activation, kind ChangeKind) {
	if !plan.NeedsApproval {
		ev := ChangeOf(act.Route, kind)
		ev.VersionNo = act.Version.No
		m.publish(ctx, ev)
		return
	}
	if m.Requests == nil {
		return
	}
	if err := m.Requests.RequestApproval(ctx, act.Deployment); err != nil {
		m.Log.Warn("routes: approval request not registered", string(logging.AttrError), err)
	}
}

func (DeployManager) deployAction(needsApproval bool) audit.Action {
	if needsApproval {
		return ActionRouteDeployRequest
	}
	return ActionRouteDeployed
}

// VersionAuditView is the audited snapshot of a created version.
type VersionAuditView struct {
	Route      RouteID          `json:"routeId"`
	No         int              `json:"no"`
	Origin     VersionOrigin    `json:"origin"`
	Deployment DeploymentStatus `json:"deployment"`
	Note       string           `json:"changeNote,omitempty"`
}
