package audit

import (
	"context"
	"fmt"
	"time"

	"github.com/eventrouter/router/internal/store"
)

// Recorder writes audit entries; the Postgres implementation joins the ambient transaction.
type Recorder interface {
	Record(ctx context.Context, e Entry) error
}

// EntryHook validates or enriches an entry before it is written (access-control plugs its Catalog here).
type EntryHook interface {
	Before(ctx context.Context, e Entry) (Entry, error)
}

// PostgresRecorder inserts entries into audit_log using the transaction carried by ctx, if any, and
// fans each entry out to the audit stream once that transaction commits.
type PostgresRecorder struct {
	db     *store.DB
	hooks  []EntryHook
	stream StreamPublisher
}

// NewPostgresRecorder returns a recorder over db with optional hooks.
func NewPostgresRecorder(db *store.DB, hooks ...EntryHook) *PostgresRecorder {
	return &PostgresRecorder{db: db, hooks: hooks, stream: NoStream{}}
}

// StreamTo sets the publisher that receives committed entries (the real-time sink fan-out).
func (r *PostgresRecorder) StreamTo(p StreamPublisher) {
	r.stream = p
}

// AddHook registers an additional validation/enrichment hook.
func (r *PostgresRecorder) AddHook(h EntryHook) {
	r.hooks = append(r.hooks, h)
}

// Record runs the hooks and inserts the row (ip and request context included).
func (r *PostgresRecorder) Record(ctx context.Context, e Entry) error {
	for _, h := range r.hooks {
		next, err := h.Before(ctx, e)
		if err != nil {
			return fmt.Errorf("audit: %s: %w", e.Action, err)
		}
		e = next
	}
	var col Column
	var id int64
	var at time.Time
	err := r.db.Q(ctx).QueryRow(ctx, insertEntrySQL,
		e.Actor.UserID, string(e.Action), string(e.Object), col.Text(e.ObjectID), col.Text(string(e.Diff)),
		col.Text(e.IP), col.Text(e.RequestID), e.TeamID, string(e.Actor.Kind), e.Sensitive, col.Text(e.ObjectLabel)).Scan(&id, &at)
	if err != nil {
		return fmt.Errorf("audit: record %s: %w", e.Action, err)
	}
	rec, stream := NewStreamRecord(id, at, e), r.stream
	store.AfterCommit(ctx, func() { stream.Publish(rec) })
	return nil
}

// insertEntrySQL is the single INSERT the write side issues (audit_log is append-only).
const insertEntrySQL = `INSERT INTO audit_log
	(actor_id, action, object_type, object_id, diff, ip, request_id, team_id, actor_kind, sensitive, object_label)
	VALUES ($1, $2, $3, $4, $5::jsonb, $6::inet, $7, $8, $9, $10, $11) RETURNING id, at`

// Column converts optional values to SQL parameters (empty → NULL).
type Column struct{}

// Text returns nil for "" so the column stores NULL.
func (Column) Text(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// HookRegistrar is implemented by recorders that accept EntryHooks (access-control plugs its catalog in).
type HookRegistrar interface {
	AddHook(h EntryHook)
}
