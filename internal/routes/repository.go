package routes

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/store"
)

// PageRequest is a keyset page: at most Limit rows after the cursor.
type PageRequest struct {
	Limit int
	After *store.Cursor
}

// ListQuery filters the routes list; an empty Statuses hides archived routes.
type ListQuery struct {
	Search       string
	Statuses     []Status
	ConnectionID string
	Tag          string
	TeamIDs      []TeamID
}

// DraftWrite is a versioned update of a route's settings and/or draft.
type DraftWrite struct {
	ID        RouteID
	Change    RouteChange
	Expected  Revision
	UpdatedBy *uuid.UUID
}

// ActivationWrite switches the live version and desired state of a route.
type ActivationWrite struct {
	ID        RouteID
	VersionID VersionID
	Desired   DesiredState
	Draft     json.RawMessage
	UpdatedBy *uuid.UUID
}

// RouteRepository persists routes (Postgres impl: internal/store/routestore).
type RouteRepository interface {
	Get(ctx context.Context, id RouteID) (Route, error)
	GetForUpdate(ctx context.Context, id RouteID) (Route, error)
	GetSummary(ctx context.Context, id RouteID) (SummaryRow, error)
	ListSummaries(ctx context.Context, q ListQuery, page PageRequest) ([]SummaryRow, *store.Cursor, error)
	Create(ctx context.Context, r Route) (Route, error)
	SaveDraft(ctx context.Context, w DraftWrite) (Route, error)
	SetDesiredState(ctx context.Context, id RouteID, s DesiredState, by *uuid.UUID) (Route, error)
	SetActiveVersion(ctx context.Context, w ActivationWrite) (Route, error)
	AddTag(ctx context.Context, id RouteID, tag string, by *uuid.UUID) (Route, error)
	NamesWithPrefix(ctx context.Context, team TeamID, prefix string) (map[string]bool, error)
	NameTaken(ctx context.Context, team TeamID, name string, except RouteID) (bool, error)
	FindByName(ctx context.Context, team TeamID, name string) (Route, error)
}

// VersionRepository persists immutable versions.
type VersionRepository interface {
	Create(ctx context.Context, v RouteVersion) (RouteVersion, error)
	List(ctx context.Context, id RouteID, page PageRequest) ([]RouteVersion, *store.Cursor, error)
	GetByNo(ctx context.Context, id RouteID, no int) (RouteVersion, error)
	GetByID(ctx context.Context, id VersionID) (RouteVersion, error)
}

// StatusChange is a guarded deployment status transition.
type StatusChange struct {
	ID         DeploymentID
	From       DeploymentStatus
	To         DeploymentStatus
	Reason     string
	ApprovedBy *uuid.UUID
}

// SupersedeScope selects which open deployments of a route a write path retires (Except is kept).
type SupersedeScope struct {
	Statuses []DeploymentStatus
	Except   DeploymentID
}

// SupersedeAllOpen retires every pending and rolling-out deployment (direct deploy, rollback, archive).
func SupersedeAllOpen() SupersedeScope {
	return SupersedeScope{Statuses: []DeploymentStatus{DeployPendingApproval, DeployRollingOut}}
}

// SupersedePending retires only older approval requests: a new request keeps the live version's rollout.
func SupersedePending() SupersedeScope {
	return SupersedeScope{Statuses: []DeploymentStatus{DeployPendingApproval}}
}

// SupersedeRollingOutExcept retires the live version's rollout when an approved deployment takes over.
func SupersedeRollingOutExcept(id DeploymentID) SupersedeScope {
	return SupersedeScope{Statuses: []DeploymentStatus{DeployRollingOut}, Except: id}
}

// Covers reports whether d is retired by the scope.
func (s SupersedeScope) Covers(d Deployment) bool {
	if d.ID == s.Except {
		return false
	}
	for _, st := range s.Statuses {
		if d.Status == st {
			return true
		}
	}
	return false
}

// DeploymentRepository persists deployments.
type DeploymentRepository interface {
	Create(ctx context.Context, d Deployment) (Deployment, error)
	Get(ctx context.Context, id DeploymentID) (Deployment, error)
	SupersedeOpen(ctx context.Context, id RouteID, scope SupersedeScope) error
	Transition(ctx context.Context, c StatusChange) (Deployment, error)
	Latest(ctx context.Context, id RouteID) (*Deployment, error)
	List(ctx context.Context, id RouteID, page PageRequest) ([]Deployment, *store.Cursor, error)
	StatusByVersion(ctx context.Context, ids []VersionID) (map[VersionID]DeploymentStatus, error)
}

// Repositories groups the three route repositories.
type Repositories struct {
	Routes      RouteRepository
	Versions    VersionRepository
	Deployments DeploymentRepository
}
