package pgstore

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/alerts"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/store"
)

const routeInfoSQL = `SELECT r.id, r.name, r.team_id, r.desired_state::text, r.active_version_id IS NOT NULL,
       COALESCE(rt.observed_state::text, '')
  FROM routes r LEFT JOIN route_runtime rt ON rt.route_id = r.id WHERE r.id = ANY($1)`

// RouteDirectory implements alerts.RouteDirectory with routes' status resolution.
type RouteDirectory struct {
	db *store.DB
}

// NewRouteDirectory builds the directory.
func NewRouteDirectory(db *store.DB) *RouteDirectory {
	return &RouteDirectory{db: db}
}

// Route returns one route or alerts.ErrRouteUnknown.
func (d *RouteDirectory) Route(ctx context.Context, id uuid.UUID) (alerts.RouteInfo, error) {
	m, err := d.Routes(ctx, []uuid.UUID{id})
	if err != nil {
		return alerts.RouteInfo{}, err
	}
	r, ok := m[id]
	if !ok {
		return alerts.RouteInfo{}, alerts.ErrRouteUnknown
	}
	return r, nil
}

// Routes returns the known routes among ids.
func (d *RouteDirectory) Routes(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]alerts.RouteInfo, error) {
	rows, err := d.db.Q(ctx).Query(ctx, routeInfoSQL, ids)
	if err != nil {
		return nil, d.db.Translate(err)
	}
	defer rows.Close()
	out := make(map[uuid.UUID]alerts.RouteInfo, len(ids))
	for rows.Next() {
		r, err := RouteInfoScanner{}.Scan(rows)
		if err != nil {
			return nil, err
		}
		out[r.ID] = r
	}
	if err := rows.Err(); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, d.db.Translate(err)
	}
	return out, nil
}

// RouteInfoScanner decodes one route row and resolves its status.
type RouteInfoScanner struct{}

// Scan reads routeInfoSQL.
func (RouteInfoScanner) Scan(row pgx.Row) (alerts.RouteInfo, error) {
	var r alerts.RouteInfo
	var desired, observed string
	var active bool
	if err := row.Scan(&r.ID, &r.Name, &r.TeamID, &desired, &active, &observed); err != nil {
		return alerts.RouteInfo{}, err
	}
	status := routes.RouteStatusResolver{}.Resolve(routes.DesiredState(desired), active, routes.ObservedState(observed))
	r.Running = status == routes.StatusRunning
	r.Archived = routes.DesiredState(desired) == routes.DesiredArchived
	return r, nil
}
