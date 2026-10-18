package runtimestore

import (
	"context"
	"time"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/state"
	"github.com/eventrouter/router/internal/store"
)

// RuntimeRepository persists route_runtime.
type RuntimeRepository struct {
	db *store.DB
}

// NewRuntimeRepository builds the repository.
func NewRuntimeRepository(db *store.DB) *RuntimeRepository { return &RuntimeRepository{db: db} }

// Get returns the runtime row of a route (store.ErrNotFound when never reported).
func (r *RuntimeRepository) Get(ctx context.Context, routeID string) (state.RuntimeRow, error) {
	var row state.RuntimeRow
	var observed, category, reason string
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT rt.route_id::text, rt.observed_state::text, COALESCE(rt.error_message, ''),
			COALESCE(rt.error_category, ''), rt.assigned_workers, COALESCE(rt.running_version_id::text, ''), COALESCE(v.version_no, 0),
			rt.halted_at, COALESCE(rt.halt_reason, ''), rt.lag, rt.partitions, rt.updated_at
		FROM route_runtime rt LEFT JOIN route_versions v ON v.id = rt.running_version_id
		WHERE rt.route_id::text = $1`, routeID).
		Scan(&row.RouteID, &observed, &row.ErrorMessage, &category, &row.AssignedWorkers, &row.RunningVersionID, &row.RunningVersionNo,
			&row.HaltedAt, &reason, &row.Lag, &row.Partitions, &row.UpdatedAt)
	if err != nil {
		return state.RuntimeRow{}, r.db.Translate(err)
	}
	row.Observed, row.ErrorCategory, row.HaltReason = domain.ObservedState(observed), domain.ErrorCategory(category), domain.HaltReason(reason)
	return row, nil
}

// Save upserts the aggregated state; halt columns are left as they are.
func (r *RuntimeRepository) Save(ctx context.Context, row state.RuntimeRow) error {
	workers := row.AssignedWorkers
	if workers == nil {
		workers = []string{}
	}
	_, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO route_runtime (route_id, observed_state, error_message, error_category, assigned_workers,
			running_version_id, lag, partitions, updated_at)
		VALUES ($1::uuid, $2::text::route_observed_state, NULLIF($3, ''), NULLIF($4, ''), $5, NULLIF($6, '')::uuid, $7, $8, now())
		ON CONFLICT (route_id) DO UPDATE SET observed_state = EXCLUDED.observed_state, error_message = EXCLUDED.error_message,
			error_category = EXCLUDED.error_category, assigned_workers = EXCLUDED.assigned_workers,
			running_version_id = EXCLUDED.running_version_id, lag = EXCLUDED.lag,
			partitions = GREATEST(EXCLUDED.partitions, route_runtime.partitions), updated_at = now()`,
		row.RouteID, string(row.Observed), row.ErrorMessage, string(row.ErrorCategory), workers, row.RunningVersionID, row.Lag, row.Partitions)
	return r.db.Translate(err)
}

// Halt records a halt (first halt time wins) and marks the route observed error.
func (r *RuntimeRepository) Halt(ctx context.Context, h domain.HaltReport, now time.Time) error {
	_, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO route_runtime (route_id, observed_state, error_message, error_category, halted_at,
			halt_reason, halted_version_id, updated_at)
		VALUES ($1::uuid, 'error', NULLIF($2, ''), NULLIF($3, ''), $4, $5, NULLIF($6, '')::uuid, $4)
		ON CONFLICT (route_id) DO UPDATE SET observed_state = 'error', error_message = EXCLUDED.error_message,
			error_category = EXCLUDED.error_category, halted_at = COALESCE(route_runtime.halted_at, EXCLUDED.halted_at),
			halt_reason = EXCLUDED.halt_reason, halted_version_id = EXCLUDED.halted_version_id, updated_at = EXCLUDED.updated_at`,
		h.RouteID, h.Message, string(h.ErrorCategory), now, string(h.Reason), h.VersionID)
	return r.db.Translate(err)
}

// ClearHalt removes the halt after a resume or a new deploy.
func (r *RuntimeRepository) ClearHalt(ctx context.Context, routeID string) error {
	_, err := r.db.Q(ctx).Exec(ctx, `UPDATE route_runtime SET halted_at = NULL, halt_reason = NULL, halted_version_id = NULL,
			error_category = NULL, error_message = NULL, updated_at = now()
		WHERE route_id::text = $1 AND halted_at IS NOT NULL`, routeID)
	return r.db.Translate(err)
}
