package routes

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
)

// Audit actions recorded by the routes feature; the catalogued ones come from access-control's catalog.
const (
	ActionRouteCreated                     = audit.ActionRouteCreated
	ActionRouteDraftSaved                  = audit.ActionRouteDraftSaved
	ActionRouteArchived                    = audit.ActionRouteArchived
	ActionRoutePaused                      = audit.ActionRoutePaused
	ActionRouteResumed                     = audit.ActionRouteResumed
	ActionRouteDeployed                    = audit.ActionRouteDeployed
	ActionRouteDeployRequest               = audit.ActionRouteDeployRequested
	ActionRouteRolledBack                  = audit.ActionRouteRolledBack
	ActionDeployActivated                  = audit.ActionDeploymentApproved
	ActionDeployRejected                   = audit.ActionDeploymentRejected
	ActionRouteUpdated        audit.Action = "route.updated"
	ActionRouteDuplicated     audit.Action = "route.duplicated"
	ActionRouteTagged         audit.Action = "route.tagged"
	ActionRouteAutoRolledBack audit.Action = "route.auto_rolled_back"
	ActionRolloutActive       audit.Action = "route.rollout_active"
	ActionRolloutFailed       audit.Action = "route.rollout_failed"
	ActionRouteImported                    = audit.ActionRouteImported
	ActionRouteExported       audit.Action = "route.exported"
)

// ExtraAuditActions are the routes actions not in access-control's base catalog; wiring registers them.
var ExtraAuditActions = []audit.Action{ActionRouteUpdated, ActionRouteDuplicated, ActionRouteTagged,
	ActionRouteAutoRolledBack, ActionRolloutActive, ActionRolloutFailed, ActionRouteUnarchived, ActionRouteExported}

// AuditRecord is one routes audit entry.
type AuditRecord struct {
	Action   audit.Action
	Object   audit.ObjectType
	ObjectID string
	Team     TeamID
	Before   any
	After    any
}

// RouteAuditor records into the request's audit scope, or as the system for in-process callers.
type RouteAuditor struct {
	Fallback audit.Recorder
}

// Record writes the entry in ctx's transaction.
func (a RouteAuditor) Record(ctx context.Context, rec AuditRecord) error {
	team := a.team(rec.Team)
	if scope, ok := audit.FromContext(ctx); ok {
		return scope.Record(ctx, audit.Change{Action: rec.Action, Object: rec.Object, ObjectID: rec.ObjectID, TeamID: team, Before: rec.Before, After: rec.After})
	}
	if a.Fallback == nil {
		return nil
	}
	diff, _ := json.Marshal(map[string]any{string(AuditDiffAfter): rec.After})
	return a.Fallback.Record(ctx, audit.Entry{Actor: audit.Actor{Kind: audit.ActorSystem}, Action: rec.Action, Object: rec.Object, ObjectID: rec.ObjectID, TeamID: team, Diff: diff})
}

// RecordIfScoped writes the entry only inside a request audit scope (reads such as exports are not
// recorded as system entries when called in-process).
func (a RouteAuditor) RecordIfScoped(ctx context.Context, rec AuditRecord) error {
	if _, ok := audit.FromContext(ctx); !ok {
		return nil
	}
	return a.Record(ctx, rec)
}

// AuditDiffKey names members of fallback diffs.
type AuditDiffKey string

const AuditDiffAfter AuditDiffKey = "after"

func (RouteAuditor) team(t TeamID) *uuid.UUID {
	id, err := uuid.Parse(string(t))
	if err != nil {
		return nil
	}
	return &id
}

// RouteAuditView is the audited snapshot of a route.
type RouteAuditView struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Tags         []string        `json:"tags"`
	DesiredState DesiredState    `json:"desiredState"`
	Replicas     int             `json:"replicas"`
	Draft        json.RawMessage `json:"draft,omitempty"`
}

// AuditViewOf snapshots r for audit diffs.
func AuditViewOf(r Route) RouteAuditView {
	return RouteAuditView{Name: r.Name, Description: r.Description, Tags: r.Tags, DesiredState: r.DesiredState, Replicas: r.Replicas, Draft: r.Draft}
}
