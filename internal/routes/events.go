package routes

import "context"

// ChangeKind names why a route_changed event was emitted.
type ChangeKind string

const (
	ChangeDeployed     ChangeKind = "deployed"
	ChangeRolledBack   ChangeKind = "rolled_back"
	ChangeAutoRollback ChangeKind = "auto_rollback"
	ChangePaused       ChangeKind = "paused"
	ChangeResumed      ChangeKind = "resumed"
	ChangeArchived     ChangeKind = "archived"
	ChangeActivated    ChangeKind = "activated"
	ChangeRolloutDone  ChangeKind = "rollout_active"
)

// RouteChanged is the payload of the "route_changed" channel consumed by route-runtime and monitoring.
type RouteChanged struct {
	ID           RouteID      `json:"id"`
	RowVersion   Revision     `json:"row_version"`
	Kind         ChangeKind   `json:"kind"`
	DesiredState DesiredState `json:"desiredState"`
	VersionID    *VersionID   `json:"versionId,omitempty"`
	VersionNo    int          `json:"versionNo,omitempty"`
}

// RouteEventPublisher emits route changes after commit (impl: routestore.PgNotifyPublisher).
type RouteEventPublisher interface {
	Publish(ctx context.Context, ev RouteChanged) error
}

// ChangeOf builds the event of a route's current state.
func ChangeOf(r Route, kind ChangeKind) RouteChanged {
	return RouteChanged{ID: r.ID, RowVersion: r.Revision, Kind: kind, DesiredState: r.DesiredState, VersionID: r.ActiveVersionID}
}
