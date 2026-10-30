package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/dlq"
	"github.com/eventrouter/router/internal/store"
)

// PostgresRepository implements Repository on dlq_jobs (migration 1100).
type PostgresRepository struct {
	db *store.DB
}

// NewPostgresRepository builds the repository.
func NewPostgresRepository(db *store.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// jobColumns are the selected columns in scan order (route name joined for notifications).
const jobColumns = `j.id, j.route_id, COALESCE(r.name, ''), j.action::text, j.selection, j.status::text, j.total, j.processed,
	j.counts, COALESCE(j.cursor, ''), COALESCE(j.error, ''), COALESCE(j.requested_by::text, ''), j.heartbeat_at,
	j.created_at, j.started_at, j.finished_at, j.rate_per_second, j.note, j.edit, COALESCE(j.decided_by::text, ''),
	j.decided_at, j.decision_note`

const selectJobSQL = `SELECT ` + jobColumns + ` FROM dlq_jobs j LEFT JOIN routes r ON r.id = j.route_id`

// returningJobSQL re-reads a job after a write (routes joined) by id.
const returningJobSQL = selectJobSQL + ` WHERE j.id = $1`

// Create implements Repository.
func (p *PostgresRepository) Create(ctx context.Context, n NewJob) (Job, error) {
	sel, err := json.Marshal(n.Match)
	if err != nil {
		return Job{}, err
	}
	var by *uuid.UUID
	if id, err := uuid.Parse(n.RequestedBy); err == nil {
		by = &id
	}
	edit, err := (EditColumn{}).Value(n.Edit)
	if err != nil {
		return Job{}, err
	}
	st, rate := n.initial()
	var id uuid.UUID
	err = p.db.Q(ctx).QueryRow(ctx, `INSERT INTO dlq_jobs (route_id, action, selection, requested_by, status, rate_per_second, note, edit)
		VALUES ($1, $2::dlq_job_action, $3, $4, $5::dlq_job_status, $6, $7, $8) RETURNING id`,
		n.RouteID, string(n.Action), sel, by, string(st), rate, n.Note, edit).Scan(&id)
	if err != nil {
		return Job{}, p.db.Translate(err)
	}
	return p.Get(ctx, id.String())
}

// Get implements Repository.
func (p *PostgresRepository) Get(ctx context.Context, id string) (Job, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Job{}, ErrNotFound
	}
	return p.one(ctx, returningJobSQL, id)
}

// ListByRoute implements Repository (newest first).
func (p *PostgresRepository) ListByRoute(ctx context.Context, routeID string, limit int) ([]Job, error) {
	rows, err := p.db.Q(ctx).Query(ctx, selectJobSQL+` WHERE j.route_id = $1 ORDER BY j.created_at DESC LIMIT $2`, routeID, limit)
	if err != nil {
		return nil, p.db.Translate(err)
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		j, err := (JobScanner{}).Scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// Claim implements Repository: the oldest queued job is locked (SKIP LOCKED) and marked running.
func (p *PostgresRepository) Claim(ctx context.Context) (Job, bool, error) {
	var id uuid.UUID
	err := p.db.Q(ctx).QueryRow(ctx, `UPDATE dlq_jobs SET status = 'running', heartbeat_at = now(),
		started_at = COALESCE(started_at, now())
		WHERE id = (SELECT id FROM dlq_jobs WHERE status = 'queued' ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1)
		RETURNING id`).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	j, err := p.Get(ctx, id.String())
	return j, err == nil, err
}

// Progress implements Repository: stores processed/counts/cursor and returns the current status.
func (p *PostgresRepository) Progress(ctx context.Context, j Job) (Status, error) {
	counts, err := json.Marshal(j.Counts)
	if err != nil {
		return "", err
	}
	var st string
	err = p.db.Q(ctx).QueryRow(ctx, `UPDATE dlq_jobs SET processed = $2, counts = $3, cursor = NULLIF($4, ''),
		total = GREATEST(total, $5), heartbeat_at = now() WHERE id = $1 RETURNING status::text`,
		j.ID, j.Processed, counts, j.Cursor, j.Total).Scan(&st)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return Status(st), err
}

// Heartbeat implements Repository.
func (p *PostgresRepository) Heartbeat(ctx context.Context, id string) error {
	_, err := p.db.Q(ctx).Exec(ctx, `UPDATE dlq_jobs SET heartbeat_at = now() WHERE id = $1`, id)
	return err
}

// Finish implements Repository.
func (p *PostgresRepository) Finish(ctx context.Context, id string, st Status, errText string) (Job, error) {
	_, err := p.db.Q(ctx).Exec(ctx, `UPDATE dlq_jobs SET status = $2::dlq_job_status, error = NULLIF($3, ''), finished_at = now(),
		edit = NULL WHERE id = $1`, id, string(st), errText)
	if err != nil {
		return Job{}, err
	}
	return p.Get(ctx, id)
}

// RequestCancel implements Repository: queued → cancelled, running → cancel_requested.
func (p *PostgresRepository) RequestCancel(ctx context.Context, id string) (Job, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Job{}, ErrNotFound
	}
	_, err := p.db.Q(ctx).Exec(ctx, `UPDATE dlq_jobs SET
		status = CASE WHEN status IN ('queued', 'pending_approval') THEN 'cancelled'::dlq_job_status ELSE 'cancel_requested'::dlq_job_status END,
		finished_at = CASE WHEN status IN ('queued', 'pending_approval') THEN now() ELSE finished_at END,
		edit = CASE WHEN status IN ('queued', 'pending_approval') THEN NULL ELSE edit END
		WHERE id = $1 AND status IN ('queued', 'running', 'pending_approval')`, id)
	if err != nil {
		return Job{}, err
	}
	return p.Get(ctx, id)
}

// Requeue implements Repository (reaper): running jobs whose runner stopped heart-beating are queued again.
func (p *PostgresRepository) Requeue(ctx context.Context, staleBefore time.Time) (int, error) {
	tag, err := p.db.Q(ctx).Exec(ctx, `UPDATE dlq_jobs SET status = 'queued'
		WHERE status IN ('running', 'cancel_requested') AND heartbeat_at < $1`, staleBefore)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (p *PostgresRepository) one(ctx context.Context, sql string, args ...any) (Job, error) {
	j, err := (JobScanner{}).Scan(p.db.Q(ctx).QueryRow(ctx, sql, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	return j, err
}

// Decide implements Repository: pending → queued (approved) or rejected, under the row lock of the UPDATE.
func (p *PostgresRepository) Decide(ctx context.Context, id string, d Decision) (Job, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Job{}, ErrNotFound
	}
	var by *uuid.UUID
	if u, err := uuid.Parse(d.By); err == nil {
		by = &u
	}
	tag, err := p.db.Q(ctx).Exec(ctx, `UPDATE dlq_jobs SET status = $2::dlq_job_status, decided_by = $3, decided_at = now(),
		decision_note = $4, finished_at = CASE WHEN $5 THEN NULL ELSE now() END, edit = CASE WHEN $5 THEN edit ELSE NULL END
		WHERE id = $1 AND status = 'pending_approval'`, id, string(d.StatusOf()), by, d.Note, d.Approve)
	if err != nil {
		return Job{}, p.db.Translate(err)
	}
	j, err := p.Get(ctx, id)
	if err == nil && tag.RowsAffected() == 0 {
		return j, dlq.ErrJobNotPending
	}
	return j, err
}

// ListPending implements Repository (oldest first).
func (p *PostgresRepository) ListPending(ctx context.Context, limit int) ([]Job, error) {
	rows, err := p.db.Q(ctx).Query(ctx, selectJobSQL+` WHERE j.status = 'pending_approval' ORDER BY j.created_at LIMIT $1`, limit)
	if err != nil {
		return nil, p.db.Translate(err)
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		j, err := (JobScanner{}).Scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// EditColumn converts the stored edit of a pending replay.
type EditColumn struct{}

// Value returns the jsonb value of e (nil = SQL NULL).
func (EditColumn) Value(e *dlq.RecordEdit) ([]byte, error) {
	if e == nil {
		return nil, nil
	}
	return json.Marshal(e)
}

// Parse reads a stored edit.
func (EditColumn) Parse(raw []byte) (*dlq.RecordEdit, error) {
	if raw == nil {
		return nil, nil
	}
	var e dlq.RecordEdit
	return &e, json.Unmarshal(raw, &e)
}

// JobScanner reads one dlq_jobs row.
type JobScanner struct{}

// Scan reads the jobColumns of row.
func (JobScanner) Scan(row pgx.Row) (Job, error) {
	var j Job
	var id, route uuid.UUID
	var action, status string
	var sel, counts, edit []byte
	err := row.Scan(&id, &route, &j.RouteName, &action, &sel, &status, &j.Total, &j.Processed, &counts, &j.Cursor,
		&j.Error, &j.RequestedBy, &j.HeartbeatAt, &j.CreatedAt, &j.StartedAt, &j.FinishedAt, &j.Rate, &j.Note, &edit,
		&j.Approval.DecidedBy, &j.Approval.DecidedAt, &j.Approval.DecisionNote)
	if err != nil {
		return Job{}, err
	}
	if j.Edit, err = (EditColumn{}).Parse(edit); err != nil {
		return Job{}, err
	}
	j.Approval.Required = Status(status) == StatusPendingApproval || j.Approval.DecidedAt != nil
	j.ID, j.RouteID, j.Action, j.Status = id.String(), route.String(), dlq.ActionKind(action), Status(status)
	j.Counts = map[dlq.Outcome]int{}
	if err := json.Unmarshal(sel, &j.Match); err != nil {
		return Job{}, err
	}
	return j, json.Unmarshal(counts, &j.Counts)
}
