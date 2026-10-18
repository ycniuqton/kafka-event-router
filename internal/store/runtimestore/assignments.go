package runtimestore

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/state"
	"github.com/eventrouter/router/internal/store"
)

// AssignmentRepository persists leases and replica status.
type AssignmentRepository struct {
	db *store.DB
}

// NewAssignmentRepository builds the repository.
func NewAssignmentRepository(db *store.DB) *AssignmentRepository {
	return &AssignmentRepository{db: db}
}

const assignmentColumns = `a.route_id::text, a.worker_id, a.lease_expires_at, COALESCE(a.target_version_id::text, ''),
	COALESCE(a.reported_version_id::text, ''), COALESCE(a.runner_state, ''), COALESCE(a.error_category, ''),
	COALESCE(a.error_message, ''), a.lag, a.partitions, a.reported_at, a.created_at`

// ForWorker returns the worker's leases with the version each must run.
func (r *AssignmentRepository) ForWorker(ctx context.Context, workerID string) ([]domain.Assignment, error) {
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT a.route_id::text, r.name, r.team_id::text, v.id::text, v.version_no, v.config_hash, a.lease_expires_at
		FROM route_assignments a
		JOIN routes r ON r.id = a.route_id
		JOIN route_versions v ON v.id = a.target_version_id
		WHERE a.worker_id = $1 ORDER BY a.route_id`, workerID)
	if err != nil {
		return nil, r.db.Translate(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Assignment, error) {
		var a domain.Assignment
		err := row.Scan(&a.RouteID, &a.RouteName, &a.TeamID, &a.VersionID, &a.VersionNo, &a.ConfigHash, &a.LeaseExpiresAt)
		return a, err
	})
}

// ReportStatus stores each replica's reported version, state, error and lag.
func (r *AssignmentRepository) ReportStatus(ctx context.Context, workerID string, replicas []domain.ReplicaStatus, now time.Time) error {
	for _, s := range replicas {
		_, err := r.db.Q(ctx).Exec(ctx, `UPDATE route_assignments SET reported_version_id = NULLIF($3, '')::uuid, runner_state = $4,
			error_category = NULLIF($5, ''), error_message = NULLIF($6, ''), lag = $7, partitions = $8, reported_at = $9
			WHERE route_id::text = $1 AND worker_id = $2`,
			s.RouteID, workerID, s.VersionID, string(s.State), string(s.ErrorCategory), s.ErrorMessage, s.Lag, s.Partitions, now)
		if err != nil {
			return r.db.Translate(err)
		}
	}
	return nil
}

// List returns every lease.
func (r *AssignmentRepository) List(ctx context.Context) ([]state.AssignmentRow, error) {
	return r.query(ctx, `SELECT `+assignmentColumns+` FROM route_assignments a ORDER BY a.route_id, a.worker_id`)
}

// ForRoute returns the leases of one route.
func (r *AssignmentRepository) ForRoute(ctx context.Context, routeID string) ([]state.AssignmentRow, error) {
	return r.query(ctx, `SELECT `+assignmentColumns+` FROM route_assignments a WHERE a.route_id = $1::uuid ORDER BY a.worker_id`, routeID)
}

// Insert creates a lease (no-op when it exists).
func (r *AssignmentRepository) Insert(ctx context.Context, a state.AssignmentRow) error {
	_, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO route_assignments (route_id, worker_id, lease_expires_at, target_version_id)
		VALUES ($1::uuid, $2, $3, NULLIF($4, '')::uuid) ON CONFLICT DO NOTHING`, a.RouteID, a.WorkerID, a.LeaseExpiresAt, a.TargetVersionID)
	return r.db.Translate(err)
}

// Delete removes one lease.
func (r *AssignmentRepository) Delete(ctx context.Context, routeID, workerID string) error {
	_, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM route_assignments WHERE route_id = $1::uuid AND worker_id = $2`, routeID, workerID)
	return r.db.Translate(err)
}

// DeleteForWorker removes the given leases of a worker (refused routes).
func (r *AssignmentRepository) DeleteForWorker(ctx context.Context, workerID string, routeIDs []string) error {
	_, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM route_assignments WHERE worker_id = $1 AND route_id::text = ANY($2)`, workerID, routeIDs)
	return r.db.Translate(err)
}

// DeleteExpired removes leases whose lease ended before now (their worker stopped heartbeating).
func (r *AssignmentRepository) DeleteExpired(ctx context.Context, now time.Time) (int, error) {
	tag, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM route_assignments WHERE lease_expires_at < $1`, now)
	if err != nil {
		return 0, r.db.Translate(err)
	}
	return int(tag.RowsAffected()), nil
}

// SetTarget points one replica at a version (rollout).
func (r *AssignmentRepository) SetTarget(ctx context.Context, routeID, workerID, versionID string) error {
	_, err := r.db.Q(ctx).Exec(ctx, `UPDATE route_assignments SET target_version_id = $3::uuid WHERE route_id = $1::uuid AND worker_id = $2`,
		routeID, workerID, versionID)
	return r.db.Translate(err)
}

func (r *AssignmentRepository) query(ctx context.Context, sql string, args ...any) ([]state.AssignmentRow, error) {
	rows, err := r.db.Q(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, r.db.Translate(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (state.AssignmentRow, error) {
		var a state.AssignmentRow
		var runnerState, category string
		err := row.Scan(&a.RouteID, &a.WorkerID, &a.LeaseExpiresAt, &a.TargetVersionID, &a.ReportedVersionID, &runnerState, &category,
			&a.ErrorMessage, &a.Lag, &a.Partitions, &a.ReportedAt, &a.CreatedAt)
		a.RunnerState, a.ErrorCategory = domain.RunnerState(runnerState), domain.ErrorCategory(category)
		return a, err
	})
}
