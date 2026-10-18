// Package pgstore implements the connections Repository and UsageResolver on Postgres.
package pgstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/secrets"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/pkg/kafka"
)

// SQL fragments of the connections repository.
const (
	selectConnection = `SELECT c.id, c.team_id, t.name, c.name, c.description, c.env_tag, c.bootstrap_servers,
       c.security_mode::text, c.config, c.secret_ref, c.sensitive, c.allow_topic_create, c.probe_enabled, c.kind,
       c.health_status::text, c.last_checked_at, c.last_check, c.row_version, c.created_at, c.updated_at, c.schema_registry
  FROM connections c JOIN teams t ON t.id = c.team_id`
	insertConnection = `INSERT INTO connections (team_id, name, description, env_tag, bootstrap_servers, security_mode, config,
       secret_ref, sensitive, allow_topic_create, probe_enabled, kind, health_status, schema_registry)
VALUES ($1, $2, $3, $4, $5, $6::connection_security_mode, $7, $8, $9, $10, $11, $12, $13::connection_health, $14) RETURNING id`
	updateSet = `name = $1, description = $2, env_tag = $3, bootstrap_servers = $4, security_mode = $5::connection_security_mode,
       config = $6, secret_ref = $7, sensitive = $8, allow_topic_create = $9, probe_enabled = $10,
       health_status = $11::connection_health, last_check = $12, schema_registry = $13`
	updateHealth = `UPDATE connections SET health_status = $2::connection_health, last_checked_at = $3, last_check = $4 WHERE id = $1`
	healthOnly   = `SELECT set_config('router.health_only', 'on', true)`
	deleteRow    = `DELETE FROM connections WHERE id = $1`
	probeWhere   = ` WHERE c.probe_enabled AND c.kind = 'kafka' AND (c.last_checked_at IS NULL OR c.last_checked_at < $1)
 ORDER BY c.last_checked_at NULLS FIRST LIMIT $2`
)

// Keyset columns of the list query.
const (
	sortColumn = "c.name"
	sortCast   = "text"
	idColumn   = "c.id"
	idCast     = "uuid"
	tableName  = "connections"
	tableID    = "id"
	likeEscape = `\`
)

// ConnectionRepository implements connections.Repository.
type ConnectionRepository struct {
	db *store.DB
	tx store.TxManager
}

// NewConnectionRepository builds the repository.
func NewConnectionRepository(db *store.DB, tx store.TxManager) *ConnectionRepository {
	return &ConnectionRepository{db: db, tx: tx}
}

// Get implements connections.Repository.
func (r *ConnectionRepository) Get(ctx context.Context, id uuid.UUID) (connections.Connection, error) {
	row := r.db.Q(ctx).QueryRow(ctx, selectConnection+` WHERE c.id = $1`, id)
	c, err := rowScanner{}.scan(row)
	if err != nil {
		return connections.Connection{}, r.db.Translate(err)
	}
	return c, nil
}

// List implements connections.Repository (keyset by name, id).
func (r *ConnectionRepository) List(ctx context.Context, q connections.ListQuery) (connections.Page, error) {
	where, args := listFilter{}.build(q)
	sql, args := store.KeysetQuery{
		Select: selectConnection, Where: where, Args: args, SortColumn: sortColumn, SortCast: sortCast,
		IDColumn: idColumn, IDCast: idCast, Direction: store.SortAsc, Limit: q.Limit, After: q.After,
	}.Build()
	items, err := r.query(ctx, sql, args...)
	if err != nil {
		return connections.Page{}, err
	}
	kept, next := store.PageSlicer[connections.Connection]{Limit: q.Limit, KeyOf: func(c connections.Connection) store.Cursor {
		return store.Cursor{SortKey: []string{c.Name}, ID: c.ID.String()}
	}}.Slice(items)
	return connections.Page{Items: kept, Next: next}, nil
}

// Create implements connections.Repository.
func (r *ConnectionRepository) Create(ctx context.Context, c connections.Connection) (connections.Connection, error) {
	cfg, err := json.Marshal(c.Config())
	if err != nil {
		return connections.Connection{}, err
	}
	registry, err := columnCodec{}.registry(c.SchemaRegistry)
	if err != nil {
		return connections.Connection{}, err
	}
	var id uuid.UUID
	err = r.db.Q(ctx).QueryRow(ctx, insertConnection, c.TeamID, c.Name, c.Description, string(c.EnvTag), c.BootstrapServers,
		string(c.Security.Mode), cfg, columnCodec{}.ref(c), c.Sensitive, c.AllowTopicCreate, c.ProbeEnabled, string(c.Kind),
		string(c.Health.Status), registry).Scan(&id)
	if err != nil {
		return connections.Connection{}, r.db.Translate(err)
	}
	return r.Get(ctx, id)
}

// Update implements connections.Repository with optimistic concurrency on row_version.
func (r *ConnectionRepository) Update(ctx context.Context, c connections.Connection, expected store.RowVersion) (connections.Connection, error) {
	cfg, err := json.Marshal(c.Config())
	if err != nil {
		return connections.Connection{}, err
	}
	failure, err := columnCodec{}.failure(c.Health.LastFailure)
	if err != nil {
		return connections.Connection{}, err
	}
	registry, err := columnCodec{}.registry(c.SchemaRegistry)
	if err != nil {
		return connections.Connection{}, err
	}
	args := []any{c.Name, c.Description, string(c.EnvTag), c.BootstrapServers, string(c.Security.Mode), cfg, columnCodec{}.ref(c),
		c.Sensitive, c.AllowTopicCreate, c.ProbeEnabled, string(c.Health.Status), failure, registry}
	if _, err := (store.VersionedUpdate{Table: tableName, IDColumn: tableID}).Exec(ctx, r.db, updateSet, args, c.ID, expected); err != nil {
		return connections.Connection{}, err
	}
	return r.Get(ctx, c.ID)
}

// Delete implements connections.Repository.
func (r *ConnectionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.db.Q(ctx).Exec(ctx, deleteRow, id)
	if err != nil {
		return r.db.Translate(err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

// UpdateHealth implements connections.Repository without bumping row_version.
func (r *ConnectionRepository) UpdateHealth(ctx context.Context, id uuid.UUID, h connections.Health) error {
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

// ListForProbe implements connections.Repository.
func (r *ConnectionRepository) ListForProbe(ctx context.Context, checkedBefore time.Time, limit int) ([]connections.Connection, error) {
	return r.query(ctx, selectConnection+probeWhere, checkedBefore, limit)
}

func (r *ConnectionRepository) query(ctx context.Context, sql string, args ...any) ([]connections.Connection, error) {
	rows, err := r.db.Q(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, r.db.Translate(err)
	}
	defer rows.Close()
	var out []connections.Connection
	for rows.Next() {
		c, err := rowScanner{}.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, r.db.Translate(rows.Err())
}

// listFilter renders the env/status/text filters.
type listFilter struct{}

func (listFilter) build(q connections.ListQuery) ([]string, []any) {
	var where []string
	var args []any
	if q.Env != "" {
		args = append(args, string(q.Env))
		where = append(where, fmt.Sprintf("c.env_tag = $%d", len(args)))
	}
	if q.Status != "" {
		args = append(args, string(q.Status))
		where = append(where, fmt.Sprintf("c.health_status = $%d::connection_health", len(args)))
	}
	if text := strings.TrimSpace(q.Text); text != "" {
		args = append(args, "%"+likePattern{}.escape(text)+"%")
		n := len(args)
		where = append(where, fmt.Sprintf("(c.name ILIKE $%d OR c.description ILIKE $%d OR array_to_string(c.bootstrap_servers, ' ') ILIKE $%d)", n, n, n))
	}
	return where, args
}

// likePattern escapes LIKE metacharacters.
type likePattern struct{}

func (likePattern) escape(s string) string {
	r := strings.NewReplacer(likeEscape, likeEscape+likeEscape, "%", likeEscape+"%", "_", likeEscape+"_")
	return r.Replace(s)
}

// columnCodec encodes nullable columns.
type columnCodec struct{}

func (columnCodec) ref(c connections.Connection) *string {
	if c.SecretRef == "" {
		return nil
	}
	ref := string(c.SecretRef)
	return &ref
}

func (columnCodec) registry(reg *kafka.SchemaRegistrySpec) ([]byte, error) {
	if reg == nil {
		return nil, nil
	}
	return json.Marshal(reg)
}

func (columnCodec) failure(f *connections.LastFailure) ([]byte, error) {
	if f == nil {
		return nil, nil
	}
	return json.Marshal(f)
}

// rowScanner maps one selectConnection row.
type rowScanner struct{}

func (rowScanner) scan(row pgx.Row) (connections.Connection, error) {
	var c connections.Connection
	var env, mode, kind, health string
	var cfg, lastCheck, registry []byte
	var ref *string
	err := row.Scan(&c.ID, &c.TeamID, &c.TeamName, &c.Name, &c.Description, &env, &c.BootstrapServers, &mode, &cfg, &ref,
		&c.Sensitive, &c.AllowTopicCreate, &c.ProbeEnabled, &kind, &health, &c.Health.CheckedAt, &lastCheck, &c.Version,
		&c.CreatedAt, &c.UpdatedAt, &registry)
	if err != nil {
		return c, err
	}
	return rowScanner{}.decode(c, decodedRow{env: env, mode: mode, kind: kind, health: health, cfg: cfg, lastCheck: lastCheck, registry: registry, ref: ref})
}

// decodedRow carries the raw columns that need decoding.
type decodedRow struct {
	env, mode, kind, health  string
	cfg, lastCheck, registry []byte
	ref                      *string
}

func (rowScanner) decode(c connections.Connection, d decodedRow) (connections.Connection, error) {
	var cfg connections.StoredConfig
	if len(d.cfg) > 0 {
		if err := json.Unmarshal(d.cfg, &cfg); err != nil {
			return c, err
		}
	}
	c.EnvTag, c.Kind, c.Health.Status = connections.EnvTag(d.env), kafka.ClusterKind(d.kind), connections.HealthStatus(d.health)
	c.Security = connections.Security{Mode: kafka.SecurityMode(d.mode), TLS: cfg.TLS, Auth: cfg.Auth}
	c.Properties, c.SecretFields = cfg.Properties, cfg.SecretFields
	if d.ref != nil {
		c.SecretRef = secrets.Ref(*d.ref)
	}
	if len(d.registry) > 0 {
		c.SchemaRegistry = &kafka.SchemaRegistrySpec{}
		if err := json.Unmarshal(d.registry, c.SchemaRegistry); err != nil {
			return c, err
		}
	}
	if len(d.lastCheck) > 0 {
		var f connections.LastFailure
		if err := json.Unmarshal(d.lastCheck, &f); err != nil {
			return c, err
		}
		c.Health.LastFailure = &f
	}
	return c, nil
}
