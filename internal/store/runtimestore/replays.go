package runtimestore

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/runtime/state"
	"github.com/eventrouter/router/internal/store"
)

// replayColumns is the select list every replay query scans with scanReplay.
const replayColumns = `id::text, route_id::text, from_ts, status::text, COALESCE(requested_by::text, ''), requested_at, started_at,
	finished_at, COALESCE(error, ''), COALESCE(consumer_group, ''), partitions`

// ReplayRepository persists route_replays (migration 2060).
type ReplayRepository struct {
	db *store.DB
}

// NewReplayRepository builds the repository.
func NewReplayRepository(db *store.DB) *ReplayRepository { return &ReplayRepository{db: db} }

// Create inserts a pending replay; a second open replay of the route is a store.ErrConflict.
func (r *ReplayRepository) Create(ctx context.Context, rp state.Replay) (state.Replay, error) {
	var by *uuid.UUID
	if id, err := uuid.Parse(rp.RequestedBy); err == nil {
		by = &id
	}
	row := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO route_replays (route_id, from_ts, requested_by)
		VALUES ($1::uuid, $2, $3) RETURNING `+replayColumns, rp.RouteID, rp.From, by)
	out, err := scanReplay(row)
	return out, r.db.Translate(err)
}

// Get returns one replay of a route.
func (r *ReplayRepository) Get(ctx context.Context, routeID, id string) (state.Replay, error) {
	row := r.db.Q(ctx).QueryRow(ctx, `SELECT `+replayColumns+` FROM route_replays WHERE route_id::text = $1 AND id::text = $2`, routeID, id)
	out, err := scanReplay(row)
	return out, r.db.Translate(err)
}

// Open returns every replay still in progress, oldest first.
func (r *ReplayRepository) Open(ctx context.Context) ([]state.Replay, error) {
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT `+replayColumns+` FROM route_replays
		WHERE status IN ('pending', 'stopping', 'resetting') ORDER BY requested_at`)
	if err != nil {
		return nil, r.db.Translate(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (state.Replay, error) { return scanReplay(row) })
}

// Transition moves a replay from t.From to t.To (compare-and-set); false when another pass moved it first.
func (r *ReplayRepository) Transition(ctx context.Context, t state.ReplayTransition) (bool, error) {
	tag, err := r.db.Q(ctx).Exec(ctx, `UPDATE route_replays SET status = $3::replay_status,
			started_at = CASE WHEN $3 = 'stopping' THEN $4 ELSE started_at END,
			finished_at = CASE WHEN $3 IN ('done', 'failed') THEN $4 ELSE finished_at END,
			error = NULLIF($5, ''), consumer_group = COALESCE(NULLIF($6, ''), consumer_group),
			partitions = GREATEST(partitions, $7)
		WHERE id::text = $1 AND status = $2::replay_status`,
		t.ID, string(t.From), string(t.To), t.At, t.Error, t.Group, t.Partitions)
	if err != nil {
		return false, r.db.Translate(err)
	}
	return tag.RowsAffected() == 1, nil
}

func scanReplay(row pgx.Row) (state.Replay, error) {
	var rp state.Replay
	var status string
	err := row.Scan(&rp.ID, &rp.RouteID, &rp.From, &status, &rp.RequestedBy, &rp.RequestedAt, &rp.StartedAt, &rp.FinishedAt,
		&rp.Error, &rp.ConsumerGroup, &rp.Partitions)
	rp.Status = state.ReplayStatusOf(status)
	return rp, err
}
