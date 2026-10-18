package runtimestore

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/runtime/state"
	"github.com/eventrouter/router/internal/store"
)

// RolloutRepository reads rolling deployments and writes the runtime-owned activation columns.
type RolloutRepository struct {
	db *store.DB
}

// NewRolloutRepository builds the repository.
func NewRolloutRepository(db *store.DB) *RolloutRepository { return &RolloutRepository{db: db} }

// Rolling returns rolling_out deployments of versions that are still live.
func (r *RolloutRepository) Rolling(ctx context.Context) ([]state.Rollout, error) {
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT d.id::text, d.route_id::text, d.version_id::text, COALESCE(d.previous_version_id::text, ''), d.activated_at
		FROM deployments d JOIN routes r ON r.id = d.route_id AND r.active_version_id = d.version_id
		WHERE d.status = 'rolling_out' ORDER BY d.requested_at`)
	if err != nil {
		return nil, r.db.Translate(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (state.Rollout, error) {
		var o state.Rollout
		err := row.Scan(&o.DeploymentID, &o.RouteID, &o.VersionID, &o.PreviousVersionID, &o.ActivatedAt)
		return o, err
	})
}

// MarkActivated stamps when every replica first ran the version (start of the auto-rollback window).
func (r *RolloutRepository) MarkActivated(ctx context.Context, deploymentID string, at time.Time) error {
	_, err := r.db.Q(ctx).Exec(ctx, `UPDATE deployments SET activated_at = $2 WHERE id::text = $1 AND activated_at IS NULL`, deploymentID, at)
	return r.db.Translate(err)
}

// SupersedeActive retires the previously active deployment of the route.
func (r *RolloutRepository) SupersedeActive(ctx context.Context, routeID, exceptDeploymentID string) error {
	_, err := r.db.Q(ctx).Exec(ctx, `UPDATE deployments SET status = 'superseded', finished_at = now()
		WHERE route_id::text = $1 AND status = 'active' AND id::text <> $2`, routeID, exceptDeploymentID)
	return r.db.Translate(err)
}
