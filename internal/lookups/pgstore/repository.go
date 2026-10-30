// Package pgstore implements the lookup-source Repository and UsageResolver on Postgres.
package pgstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/lookups"
	"github.com/eventrouter/router/internal/secrets"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/pkg/lookup"
)

// SQL of the repository.
const (
	selectSource = `SELECT s.id, s.team_id, t.name, s.name, s.description, s.kind, s.env_tag, s.sensitive, s.config, s.limits,
       s.secret_ref, s.secret_fields, s.health_status, s.last_checked_at, s.last_check, s.row_version, s.created_at, s.updated_at
  FROM lookup_sources s JOIN teams t ON t.id = s.team_id`
	insertSource = `INSERT INTO lookup_sources (team_id, name, description, kind, env_tag, sensitive, config, limits, secret_ref,
       secret_fields, health_status) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id`
	updateSet = `name = $1, description = $2, env_tag = $3, sensitive = $4, config = $5, limits = $6, secret_ref = $7,
       secret_fields = $8, health_status = $9, last_check = $10`
	updateHealth = `UPDATE lookup_sources SET health_status = $2, last_checked_at = $3, last_check = $4 WHERE id = $1`
	healthOnly   = `SELECT set_config('router.health_only', 'on', true)`
	deleteRow    = `DELETE FROM lookup_sources WHERE id = $1`
)

// Keyset columns of the list query.
const (
	sortColumn = "s.name"
	sortCast   = "text"
	idColumn   = "s.id"
	idCast     = "uuid"
	tableName  = "lookup_sources"
	tableID    = "id"
	likeEscape = `\`
)

// Repository implements lookups.Repository.
type Repository struct {
	db *store.DB
	tx store.TxManager
}

// NewRepository builds the repository.
func NewRepository(db *store.DB, tx store.TxManager) *Repository { return &Repository{db: db, tx: tx} }

// Get implements lookups.Repository.
func (r *Repository) Get(ctx context.Context, id uuid.UUID) (lookups.Source, error) {
	s, err := rowScanner{}.scan(r.db.Q(ctx).QueryRow(ctx, selectSource+` WHERE s.id = $1`, id))
	if err != nil {
		return lookups.Source{}, r.db.Translate(err)
	}
	return s, nil
}

// List implements lookups.Repository (keyset by name, id).
func (r *Repository) List(ctx context.Context, q lookups.ListQuery) (lookups.Page, error) {
	where, args := listFilter{}.build(q)
	sql, args := store.KeysetQuery{Select: selectSource, Where: where, Args: args, SortColumn: sortColumn, SortCast: sortCast,
		IDColumn: idColumn, IDCast: idCast, Direction: store.SortAsc, Limit: q.Limit, After: q.After}.Build()
	rows, err := r.db.Q(ctx).Query(ctx, sql, args...)
	if err != nil {
		return lookups.Page{}, r.db.Translate(err)
	}
	defer rows.Close()
	var items []lookups.Source
	for rows.Next() {
		s, err := rowScanner{}.scan(rows)
		if err != nil {
			return lookups.Page{}, err
		}
		items = append(items, s)
	}
	kept, next := store.PageSlicer[lookups.Source]{Limit: q.Limit, KeyOf: func(s lookups.Source) store.Cursor {
		return store.Cursor{SortKey: []string{s.Name}, ID: s.ID.String()}
	}}.Slice(items)
	return lookups.Page{Items: kept, Next: next}, r.db.Translate(rows.Err())
}

// Create implements lookups.Repository.
func (r *Repository) Create(ctx context.Context, s lookups.Source) (lookups.Source, error) {
	cols, err := columnCodec{}.encode(s)
	if err != nil {
		return lookups.Source{}, err
	}
	var id uuid.UUID
	err = r.db.Q(ctx).QueryRow(ctx, insertSource, s.TeamID, s.Name, s.Description, string(s.Kind), string(s.EnvTag), s.Sensitive,
		cols.config, cols.limits, cols.ref, cols.fields, string(s.Health.Status)).Scan(&id)
	if err != nil {
		return lookups.Source{}, r.db.Translate(err)
	}
	return r.Get(ctx, id)
}

// Update implements lookups.Repository with optimistic concurrency on row_version.
func (r *Repository) Update(ctx context.Context, s lookups.Source, expected store.RowVersion) (lookups.Source, error) {
	cols, err := columnCodec{}.encode(s)
	if err != nil {
		return lookups.Source{}, err
	}
	failure, err := columnCodec{}.failure(s.Health.LastFailure)
	if err != nil {
		return lookups.Source{}, err
	}
	args := []any{s.Name, s.Description, string(s.EnvTag), s.Sensitive, cols.config, cols.limits, cols.ref, cols.fields, string(s.Health.Status), failure}
	if _, err := (store.VersionedUpdate{Table: tableName, IDColumn: tableID}).Exec(ctx, r.db, updateSet, args, s.ID, expected); err != nil {
		return lookups.Source{}, err
	}
	return r.Get(ctx, s.ID)
}

// Delete implements lookups.Repository.
func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.db.Q(ctx).Exec(ctx, deleteRow, id)
	if err != nil {
		return r.db.Translate(err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

// UpdateHealth implements lookups.Repository without bumping row_version.
func (r *Repository) UpdateHealth(ctx context.Context, id uuid.UUID, h lookups.Health) error {
	failure, err := columnCodec{}.failure(h.LastFailure)
	if err != nil {
		return err
	}
	return r.tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := r.db.Q(ctx).Exec(ctx, healthOnly); err != nil {
			return r.db.Translate(err)
		}
		_, err := r.db.Q(ctx).Exec(ctx, updateHealth, id, string(h.Status), h.CheckedAt, failure)
		return r.db.Translate(err)
	})
}

// listFilter renders the kind/env/text filters.
type listFilter struct{}

func (listFilter) build(q lookups.ListQuery) ([]string, []any) {
	var where []string
	var args []any
	if q.Kind != "" {
		args = append(args, string(q.Kind))
		where = append(where, fmt.Sprintf("s.kind = $%d", len(args)))
	}
	if q.Env != "" {
		args = append(args, string(q.Env))
		where = append(where, fmt.Sprintf("s.env_tag = $%d", len(args)))
	}
	if text := strings.TrimSpace(q.Text); text != "" {
		esc := strings.NewReplacer(likeEscape, likeEscape+likeEscape, "%", likeEscape+"%", "_", likeEscape+"_").Replace(text)
		args = append(args, "%"+esc+"%")
		where = append(where, fmt.Sprintf("(s.name ILIKE $%d OR s.description ILIKE $%d)", len(args), len(args)))
	}
	return where, args
}

// encodedColumns are the JSON / nullable columns of a source.
type encodedColumns struct {
	config, limits []byte
	ref            *string
	fields         []string
}

// columnCodec encodes and decodes JSON and nullable columns.
type columnCodec struct{}

func (columnCodec) encode(s lookups.Source) (encodedColumns, error) {
	cfg, err := json.Marshal(s.Settings)
	if err != nil {
		return encodedColumns{}, err
	}
	limits, err := json.Marshal(s.Limits)
	if err != nil {
		return encodedColumns{}, err
	}
	out := encodedColumns{config: cfg, limits: limits, fields: []string{}}
	if s.SecretRef != "" {
		ref := string(s.SecretRef)
		out.ref = &ref
	}
	for _, f := range s.SecretFields {
		out.fields = append(out.fields, string(f))
	}
	return out, nil
}

func (columnCodec) failure(f *lookups.LastFailure) ([]byte, error) {
	if f == nil {
		return nil, nil
	}
	return json.Marshal(f)
}

// rowScanner maps one selectSource row.
type rowScanner struct{}

// rawRow carries the columns that need decoding.
type rawRow struct {
	kind, env, health         string
	config, limits, lastCheck []byte
	ref                       *string
	fields                    []string
}

func (rowScanner) scan(row pgx.Row) (lookups.Source, error) {
	var s lookups.Source
	var raw rawRow
	err := row.Scan(&s.ID, &s.TeamID, &s.TeamName, &s.Name, &s.Description, &raw.kind, &raw.env, &s.Sensitive, &raw.config, &raw.limits,
		&raw.ref, &raw.fields, &raw.health, &s.Health.CheckedAt, &raw.lastCheck, &s.Version, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return s, err
	}
	return rowScanner{}.decode(s, raw)
}

func (rowScanner) decode(s lookups.Source, raw rawRow) (lookups.Source, error) {
	s.Kind, s.EnvTag, s.Health.Status = lookup.SourceKind(raw.kind), lookups.EnvTag(raw.env), lookups.HealthStatus(raw.health)
	if err := json.Unmarshal(raw.config, &s.Settings); err != nil {
		return s, err
	}
	if err := json.Unmarshal(raw.limits, &s.Limits); err != nil {
		return s, err
	}
	if raw.ref != nil {
		s.SecretRef = secrets.Ref(*raw.ref)
	}
	for _, f := range raw.fields {
		s.SecretFields = append(s.SecretFields, lookup.SecretField(f))
	}
	if len(raw.lastCheck) > 0 {
		var f lookups.LastFailure
		if err := json.Unmarshal(raw.lastCheck, &f); err != nil {
			return s, err
		}
		s.Health.LastFailure = &f
	}
	return s, nil
}
