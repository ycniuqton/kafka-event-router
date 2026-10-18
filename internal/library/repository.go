package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/store"
)

// SQL templates of the library store; %[n]s are filled from a kind's TableSchema (never from input).
// Listing args: $1 text, $2 usage jsonpath, $3 limit, $4 names, $5 include archived, $6/$7 after (name, id), $8 id.
const (
	listSQL = `SELECT o.id, o.team_id, t.name, o.name, o.description, %[6]s, o.archived_at, o.row_version,
       coalesce(v.version_no, 0), coalesce(v.size, 0), o.updated_at, coalesce(u.display_name, ''),
       (SELECT count(*) FROM routes r LEFT JOIN route_versions av ON av.id = r.active_version_id
         WHERE jsonb_path_exists(coalesce(r.draft_config, '{}'::jsonb), $2::jsonpath, jsonb_build_object('n', o.name))
            OR jsonb_path_exists(coalesce(av.config, '{}'::jsonb), $2::jsonpath, jsonb_build_object('n', o.name)))
  FROM %[1]s o JOIN teams t ON t.id = o.team_id
  LEFT JOIN LATERAL (SELECT version_no, jsonb_array_length(%[4]s) AS size, created_by FROM %[2]s
                      WHERE %[3]s = o.id ORDER BY version_no DESC LIMIT 1) v ON true
  LEFT JOIN users u ON u.id = v.created_by
 WHERE ($1 = '' OR o.name ILIKE '%%' || $1 || '%%') AND (cardinality($4::text[]) = 0 OR o.name = ANY($4::text[]))
   AND ($5 OR o.archived_at IS NULL) AND ($6 = '' OR (o.name, o.id::text) > ($6, $7)) %[5]s
 ORDER BY o.name, o.id::text LIMIT $3`
	byIDFilter     = `AND o.id = $8`
	byNameFilter   = `AND o.name = $8`
	noKeyType      = `''`
	keyTypeColumn  = `o.%s`
	insertObject   = `INSERT INTO %[1]s (team_id, name, description%[2]s) VALUES ($1, $2, $3%[3]s) RETURNING id`
	insertKeyCol   = `, %s`
	insertKeyParam = `, $4`
	lockObject     = `SELECT coalesce((SELECT max(version_no) FROM %[2]s WHERE %[3]s = o.id), 0) FROM %[1]s o WHERE o.id = $1 FOR UPDATE`
	insertVersion  = `INSERT INTO %[1]s (%[2]s, version_no, %[3]s, created_by, config_hash, change_note) VALUES ($1, $2, $3, $4, $5, $6)`
	touchObject    = `UPDATE %s SET updated_at = now() WHERE id = $1`
	versionColumns = `v.version_no, v.%[3]s, jsonb_array_length(v.%[3]s), v.config_hash, v.change_note, v.created_at, coalesce(u.display_name, '')`
	versionsSQL    = `SELECT ` + versionColumns + ` FROM %[1]s v LEFT JOIN users u ON u.id = v.created_by WHERE v.%[2]s = $1 ORDER BY v.version_no DESC`
	versionSQL     = `SELECT ` + versionColumns + ` FROM %[1]s v LEFT JOIN users u ON u.id = v.created_by WHERE v.%[2]s = $1 AND v.version_no = $2`
	metadataSQL    = `UPDATE %s SET description = coalesce($2, description) WHERE id = $1 AND row_version = $3 RETURNING row_version`
	rowVersionSQL  = `SELECT row_version FROM %s WHERE id = $1`
	archiveSQL     = `UPDATE %s SET archived_at = now() WHERE id = $1 AND archived_at IS NULL`
	teamOfSQL      = `SELECT team_id::text FROM %s WHERE id::text = $1`
	elementsSQL    = `SELECT t.elem, jsonb_array_length(v.%[3]s) FROM %[1]s v, jsonb_array_elements(v.%[3]s) WITH ORDINALITY AS t(elem, idx)
 WHERE v.%[2]s = $1 AND v.version_no = $2 AND t.idx > $3 ORDER BY t.idx LIMIT $4`
)

// VersionRecord is a validated version ready to append.
type VersionRecord struct {
	Content    json.RawMessage
	ConfigHash string
	ChangeNote string
	Author     uuid.UUID
}

// Repository stores library objects of every kind in their tables (0001_init + 2080).
type Repository struct {
	db *store.DB
}

// NewRepository builds the repository.
func NewRepository(db *store.DB) *Repository {
	return &Repository{db: db}
}

// List returns one page of objects of a kind, by name, with latest version and usage counts.
func (r *Repository) List(ctx context.Context, k ContentKind, q ListQuery) (ListPage, error) {
	afterName, afterID := "", ""
	if q.After != nil && len(q.After.SortKey) > 0 {
		afterName, afterID = q.After.SortKey[0], q.After.ID
	}
	names := q.Names
	if names == nil {
		names = []string{}
	}
	items, err := r.query(ctx, k, r.listSQL(k, ""), q.Text, k.UsagePath(), q.Limit+1, names, q.IncludeArchived, afterName, afterID)
	if err != nil {
		return ListPage{}, err
	}
	page := ListPage{Items: items}
	if len(items) > q.Limit {
		last := items[q.Limit-1]
		page.Items, page.Next = items[:q.Limit], &store.Cursor{SortKey: []string{last.Name}, ID: last.ID.String()}
	}
	return page, nil
}

// Get returns one object (archived included).
func (r *Repository) Get(ctx context.Context, k ContentKind, id uuid.UUID) (Item, error) {
	return r.one(ctx, k, byIDFilter, id)
}

// GetByName returns the object of that name in any team (names are referenced globally).
func (r *Repository) GetByName(ctx context.Context, k ContentKind, name string) (Item, error) {
	return r.one(ctx, k, byNameFilter, name)
}

// Create inserts the object; its first version is appended by the caller in the same transaction.
func (r *Repository) Create(ctx context.Context, k ContentKind, d Draft) (uuid.UUID, error) {
	s := k.Schema()
	args := []any{d.TeamID, d.Name, d.Description}
	cols, params := "", ""
	if s.KeyType != "" {
		cols, params = fmt.Sprintf(insertKeyCol, s.KeyType), insertKeyParam
		args = append(args, string(d.KeyType.OrDefault()))
	}
	var id uuid.UUID
	err := r.db.Q(ctx).QueryRow(ctx, fmt.Sprintf(insertObject, s.Objects, cols, params), args...).Scan(&id)
	return id, r.db.Translate(err)
}

// AppendVersion locks the object row and inserts version latest+1 (must run in a transaction).
func (r *Repository) AppendVersion(ctx context.Context, k ContentKind, id uuid.UUID, v VersionRecord) (int, error) {
	s := k.Schema()
	var latest int
	if err := r.db.Q(ctx).QueryRow(ctx, fmt.Sprintf(lockObject, s.Objects, s.Versions, s.ForeignKey), id).Scan(&latest); err != nil {
		return 0, r.db.Translate(err)
	}
	var by *uuid.UUID
	if v.Author != uuid.Nil {
		by = &v.Author
	}
	sql := fmt.Sprintf(insertVersion, s.Versions, s.ForeignKey, s.Content)
	if _, err := r.db.Q(ctx).Exec(ctx, sql, id, latest+1, v.Content, by, v.ConfigHash, v.ChangeNote); err != nil {
		return 0, r.db.Translate(err)
	}
	if _, err := r.db.Q(ctx).Exec(ctx, fmt.Sprintf(touchObject, s.Objects), id); err != nil {
		return 0, err
	}
	return latest + 1, nil
}

// Versions returns every version of an object, newest first.
func (r *Repository) Versions(ctx context.Context, k ContentKind, id uuid.UUID) ([]Version, error) {
	s := k.Schema()
	rows, err := r.db.Q(ctx).Query(ctx, fmt.Sprintf(versionsSQL, s.Versions, s.ForeignKey, s.Content), id)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, r.scanVersion)
}

// Version returns one version.
func (r *Repository) Version(ctx context.Context, k ContentKind, id uuid.UUID, no int) (Version, error) {
	s := k.Schema()
	rows, err := r.db.Q(ctx).Query(ctx, fmt.Sprintf(versionSQL, s.Versions, s.ForeignKey, s.Content), id, no)
	if err != nil {
		return Version{}, err
	}
	v, err := pgx.CollectExactlyOneRow(rows, r.scanVersion)
	return v, r.db.Translate(err)
}

// UpdateMetadata applies p when the row version still matches (412 otherwise).
func (r *Repository) UpdateMetadata(ctx context.Context, k ContentKind, id uuid.UUID, p MetadataPatch, expected store.RowVersion) error {
	s := k.Schema()
	var next store.RowVersion
	err := r.db.Q(ctx).QueryRow(ctx, fmt.Sprintf(metadataSQL, s.Objects), id, p.Description, expected).Scan(&next)
	if !errors.Is(err, pgx.ErrNoRows) {
		return r.db.Translate(err)
	}
	var current store.RowVersion
	if err := r.db.Q(ctx).QueryRow(ctx, fmt.Sprintf(rowVersionSQL, s.Objects), id).Scan(&current); err != nil {
		return r.db.Translate(err)
	}
	return &store.PreconditionFailedError{Current: current}
}

// Archive marks the object archived (idempotent).
func (r *Repository) Archive(ctx context.Context, k ContentKind, id uuid.UUID) error {
	_, err := r.db.Q(ctx).Exec(ctx, fmt.Sprintf(archiveSQL, k.Schema().Objects), id)
	return r.db.Translate(err)
}

// Elements pages the content elements (rows) of one version by position.
func (r *Repository) Elements(ctx context.Context, k ContentKind, id uuid.UUID, q RowsQuery) ([]json.RawMessage, int, error) {
	s := k.Schema()
	rows, err := r.db.Q(ctx).Query(ctx, fmt.Sprintf(elementsSQL, s.Versions, s.ForeignKey, s.Content), id, q.Version, q.Offset, q.Limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out, total := []json.RawMessage{}, 0
	for rows.Next() {
		var elem json.RawMessage
		if err := rows.Scan(&elem, &total); err != nil {
			return nil, 0, err
		}
		out = append(out, elem)
	}
	return out, total, rows.Err()
}

// TeamOf returns the owning team id (rbac.TeamOwner for object-scoped permission checks).
func (r *Repository) TeamOf(ctx context.Context, k ContentKind, id string) (string, error) {
	var team string
	err := r.db.Q(ctx).QueryRow(ctx, fmt.Sprintf(teamOfSQL, k.Schema().Objects), id).Scan(&team)
	return team, r.db.Translate(err)
}

func (r *Repository) one(ctx context.Context, k ContentKind, filter string, key any) (Item, error) {
	items, err := r.query(ctx, k, r.listSQL(k, filter), "", k.UsagePath(), 1, []string{}, true, "", "", key)
	if err != nil {
		return Item{}, err
	}
	if len(items) == 0 {
		return Item{}, ErrNotFound
	}
	return items[0], nil
}

func (r *Repository) listSQL(k ContentKind, filter string) string {
	s := k.Schema()
	keyType := noKeyType
	if s.KeyType != "" {
		keyType = fmt.Sprintf(keyTypeColumn, s.KeyType)
	}
	return fmt.Sprintf(listSQL, s.Objects, s.Versions, s.ForeignKey, s.Content, filter, keyType)
}

func (r *Repository) query(ctx context.Context, k ContentKind, sql string, args ...any) ([]Item, error) {
	rows, err := r.db.Q(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, r.db.Translate(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Item, error) {
		it := Item{Kind: k.Kind()}
		var keyType string
		err := row.Scan(&it.ID, &it.TeamID, &it.TeamName, &it.Name, &it.Description, &keyType, &it.ArchivedAt, &it.RowVersion,
			&it.LatestVersion, &it.Size, &it.UpdatedAt, &it.UpdatedBy, &it.UsageCount)
		it.KeyType = ValueType(keyType)
		return it, err
	})
}

func (r *Repository) scanVersion(row pgx.CollectableRow) (Version, error) {
	var v Version
	err := row.Scan(&v.No, &v.Content, &v.Size, &v.ConfigHash, &v.ChangeNote, &v.CreatedAt, &v.CreatedBy)
	return v, err
}
