package runtimestore

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/runtime/state"
	"github.com/eventrouter/router/internal/store"
)

// RouteRepository reads route demand and immutable versions for the scheduler and workers.
type RouteRepository struct {
	db *store.DB
}

// NewRouteRepository builds the repository.
func NewRouteRepository(db *store.DB) *RouteRepository { return &RouteRepository{db: db} }

// Demands returns every non-archived route plus archived routes that still hold leases.
func (r *RouteRepository) Demands(ctx context.Context) ([]state.RouteDemand, error) {
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT r.id::text, r.name, r.desired_state::text, COALESCE(r.active_version_id::text, ''), r.replicas,
			COALESCE(rt.partitions, 0), r.updated_at, rt.halted_at,
			EXISTS (SELECT 1 FROM route_replays rr WHERE rr.route_id = r.id AND rr.status IN ('stopping', 'resetting'))
		FROM routes r LEFT JOIN route_runtime rt ON rt.route_id = r.id
		WHERE r.desired_state <> 'archived' OR EXISTS (SELECT 1 FROM route_assignments a WHERE a.route_id = r.id)
		ORDER BY r.id`)
	if err != nil {
		return nil, r.db.Translate(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (state.RouteDemand, error) {
		var d state.RouteDemand
		err := row.Scan(&d.RouteID, &d.Name, &d.DesiredState, &d.ActiveVersionID, &d.Replicas, &d.Partitions, &d.UpdatedAt, &d.HaltedAt, &d.Replaying)
		return d, err
	})
}

// VersionConfig returns the immutable config document of a version.
func (r *RouteRepository) VersionConfig(ctx context.Context, versionID string) ([]byte, error) {
	var doc string
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT config::text FROM route_versions WHERE id::text = $1`, versionID).Scan(&doc)
	if err != nil {
		return nil, r.db.Translate(err)
	}
	return []byte(doc), nil
}

// VersionNo returns the number of a version.
func (r *RouteRepository) VersionNo(ctx context.Context, versionID string) (int, error) {
	var no int
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT version_no FROM route_versions WHERE id::text = $1`, versionID).Scan(&no)
	return no, r.db.Translate(err)
}

// Exists reports whether a route row exists.
func (r *RouteRepository) Exists(ctx context.Context, routeID string) (bool, error) {
	var ok bool
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM routes WHERE id::text = $1)`, routeID).Scan(&ok)
	return ok, r.db.Translate(err)
}

// ActiveVersion returns the route's active version id ("" when it has none).
func (r *RouteRepository) ActiveVersion(ctx context.Context, routeID string) (string, error) {
	var id string
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT COALESCE(active_version_id::text, '') FROM routes WHERE id::text = $1`, routeID).Scan(&id)
	return id, r.db.Translate(err)
}
