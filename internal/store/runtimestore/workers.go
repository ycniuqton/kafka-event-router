// Package runtimestore holds the Postgres repositories of route-runtime (workers, route_assignments,
// route_runtime, deployment activation) implementing the internal/runtime/state ports.
package runtimestore

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/state"
	"github.com/eventrouter/router/internal/store"
)

// WorkerRepository persists workers.
type WorkerRepository struct {
	db *store.DB
}

// NewWorkerRepository builds the repository.
func NewWorkerRepository(db *store.DB) *WorkerRepository { return &WorkerRepository{db: db} }

const workerColumns = `w.id, w.host, w.zone, w.version, w.capacity, w.capacity_used, w.draining, w.last_heartbeat_at, w.started_at,
	(SELECT count(*) FROM route_assignments a WHERE a.worker_id = w.id)`

// UpsertHeartbeat records liveness, identity and load.
func (r *WorkerRepository) UpsertHeartbeat(ctx context.Context, id string, hb domain.Heartbeat, now time.Time) error {
	_, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO workers (id, host, version, capacity, zone, capacity_used, draining, last_heartbeat_at, started_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
		ON CONFLICT (id) DO UPDATE SET host = EXCLUDED.host, version = EXCLUDED.version, capacity = EXCLUDED.capacity,
			zone = EXCLUDED.zone, capacity_used = EXCLUDED.capacity_used, draining = EXCLUDED.draining,
			last_heartbeat_at = EXCLUDED.last_heartbeat_at`,
		id, hb.Host, hb.Version, hb.Capacity, hb.Zone, hb.CapacityUsed, hb.Draining, now)
	return r.db.Translate(err)
}

// ExtendLeases renews every lease of the worker.
func (r *WorkerRepository) ExtendLeases(ctx context.Context, workerID string, until time.Time) error {
	_, err := r.db.Q(ctx).Exec(ctx, `UPDATE route_assignments SET lease_expires_at = $2 WHERE worker_id = $1`, workerID, until)
	return r.db.Translate(err)
}

// ListWorkers returns every worker.
func (r *WorkerRepository) ListWorkers(ctx context.Context) ([]state.Worker, error) {
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT `+workerColumns+` FROM workers w ORDER BY w.id`)
	if err != nil {
		return nil, r.db.Translate(err)
	}
	return pgx.CollectRows(rows, workerScanner{}.scan)
}

// PageWorkers returns one id-ordered page.
func (r *WorkerRepository) PageWorkers(ctx context.Context, limit int, after *store.Cursor) ([]state.Worker, *store.Cursor, error) {
	from := ""
	if after != nil {
		from = after.ID
	}
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT `+workerColumns+` FROM workers w WHERE w.id > $1 ORDER BY w.id LIMIT $2`, from, limit+1)
	if err != nil {
		return nil, nil, r.db.Translate(err)
	}
	list, err := pgx.CollectRows(rows, workerScanner{}.scan)
	if err != nil || len(list) <= limit {
		return list, nil, err
	}
	list = list[:limit]
	last := list[len(list)-1].ID
	return list, &store.Cursor{SortKey: []string{last}, ID: last}, nil
}

// workerScanner reads worker rows.
type workerScanner struct{}

func (workerScanner) scan(row pgx.CollectableRow) (state.Worker, error) {
	var w state.Worker
	err := row.Scan(&w.ID, &w.Host, &w.Zone, &w.Version, &w.Capacity, &w.CapacityUsed, &w.Draining, &w.LastHeartbeatAt, &w.StartedAt, &w.Routes)
	return w, err
}
