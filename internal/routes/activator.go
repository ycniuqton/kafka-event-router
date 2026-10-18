package routes

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

// ActivationPlan describes a new version to create and (unless approval is needed) make live.
type ActivationPlan struct {
	Route         Route
	Config        json.RawMessage
	Origin        VersionOrigin
	BasedOn       *VersionID
	Note          string
	Actor         *uuid.UUID
	NeedsApproval bool
	ReplaceDraft  bool
}

// Activation is what one plan produced.
type Activation struct {
	Deployment Deployment
	Version    RouteVersion
	Route      Route
}

// VersionActivator is the shared write path of deploy, rollback and auto-rollback; call it inside a tx
// after locking the route row.
type VersionActivator struct {
	Repos Repositories
}

// Activate supersedes open deployments, creates the version and deployment and switches the live version.
func (a VersionActivator) Activate(ctx context.Context, p ActivationPlan) (Activation, error) {
	hash, err := ConfigFingerprint{}.Of(p.Config)
	if err != nil {
		return Activation{}, err
	}
	if err := a.Repos.Deployments.SupersedeOpen(ctx, p.Route.ID, a.supersedeScope(p)); err != nil {
		return Activation{}, err
	}
	v, err := a.Repos.Versions.Create(ctx, RouteVersion{RouteID: p.Route.ID, Config: p.Config, ConfigHash: hash,
		Origin: p.Origin, BasedOn: p.BasedOn, CreatedBy: p.Actor, ChangeNote: p.Note})
	if err != nil {
		return Activation{}, err
	}
	d, err := a.Repos.Deployments.Create(ctx, Deployment{RouteID: p.Route.ID, VersionID: v.ID, VersionNo: v.No,
		PreviousVersionID: p.Route.ActiveVersionID, Status: a.initialStatus(p), RequestedBy: p.Actor})
	if err != nil {
		return Activation{}, err
	}
	route, err := a.switchLive(ctx, p, v)
	if err != nil {
		return Activation{}, err
	}
	return Activation{Deployment: d, Version: v, Route: route}, nil
}

// supersedeScope keeps the live version's rollout while a new version only waits for approval.
func (VersionActivator) supersedeScope(p ActivationPlan) SupersedeScope {
	if p.NeedsApproval {
		return SupersedePending()
	}
	return SupersedeAllOpen()
}

// initialStatus is pending_approval when a second person must approve, else rolling_out.
func (VersionActivator) initialStatus(p ActivationPlan) DeploymentStatus {
	if p.NeedsApproval {
		return DeployPendingApproval
	}
	return DeployRollingOut
}

// switchLive points the route at v and sets it running; approval keeps the current live version.
func (a VersionActivator) switchLive(ctx context.Context, p ActivationPlan, v RouteVersion) (Route, error) {
	if p.NeedsApproval {
		return p.Route, nil
	}
	w := ActivationWrite{ID: p.Route.ID, VersionID: v.ID, Desired: DesiredRunning, UpdatedBy: p.Actor}
	if p.ReplaceDraft {
		w.Draft = p.Config
	}
	return a.Repos.Routes.SetActiveVersion(ctx, w)
}
