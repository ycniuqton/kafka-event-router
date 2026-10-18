package livetail

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/store"
)

// TailRoute is what the tail endpoint needs to know about a route.
type TailRoute struct {
	ID                 string
	Name               string
	TeamID             string
	Status             routes.Status
	SourceConnectionID string
}

// TailableStatuses are the route statuses whose runners produce tail records (error routes may still run).
var TailableStatuses = map[routes.Status]bool{routes.StatusRunning: true, routes.StatusError: true}

// Tailable reports whether the route's runners can produce records.
func (r TailRoute) Tailable() bool {
	return TailableStatuses[r.Status]
}

// RouteDirectory finds a route for the tail endpoint.
type RouteDirectory interface {
	TailRoute(ctx context.Context, id string) (TailRoute, error)
}

// tailRouteSQL reads status inputs and the active version's source connection.
const tailRouteSQL = `SELECT r.id::text, r.name, r.team_id::text, r.desired_state::text, r.active_version_id IS NOT NULL,
       COALESCE(rt.observed_state::text, ''), COALESCE(v.config->'source'->>'connectionId', '')
  FROM routes r LEFT JOIN route_versions v ON v.id = r.active_version_id
  LEFT JOIN route_runtime rt ON rt.route_id = r.id
 WHERE r.id = $1`

// PgRouteDirectory implements RouteDirectory on Postgres.
type PgRouteDirectory struct {
	DB *store.DB
}

// TailRoute implements RouteDirectory; a missing route is routes.ErrRouteNotFound.
func (d PgRouteDirectory) TailRoute(ctx context.Context, id string) (TailRoute, error) {
	var r TailRoute
	var desired, observed string
	var active bool
	err := d.DB.Q(ctx).QueryRow(ctx, tailRouteSQL, id).Scan(&r.ID, &r.Name, &r.TeamID, &desired, &active, &observed, &r.SourceConnectionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return TailRoute{}, routes.ErrRouteNotFound
	}
	if err != nil {
		return TailRoute{}, d.DB.Translate(err)
	}
	r.Status = routes.RouteStatusResolver{}.Resolve(routes.DesiredState(desired), active, routes.ObservedState(observed))
	return r, nil
}
