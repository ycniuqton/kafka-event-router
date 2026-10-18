// Package resolver implements pipeline-engine's config.LibraryResolver over the library tables, so
// compiles (test cases, publish checks, routes validation, preview) load pinned library versions.
package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/pkg/config"
)

// SQL loading version $2 of the object named $1 (oldest object first when teams share a name).
const (
	transformSQL = `SELECT v.steps FROM transforms o JOIN transform_versions v ON v.transform_id = o.id
 WHERE o.name = $1 AND v.version_no = $2 ORDER BY o.created_at, o.id LIMIT 1`
	lookupSQL = `SELECT v.rows FROM lookup_tables o JOIN lookup_table_versions v ON v.lookup_table_id = o.id
 WHERE o.name = $1 AND v.version_no = $2 ORDER BY o.created_at, o.id LIMIT 1`
)

// StoreLibraryResolver resolves `library` step and named `lookup` refs from Postgres.
type StoreLibraryResolver struct {
	db *store.DB
}

// New builds the resolver.
func New(db *store.DB) *StoreLibraryResolver {
	return &StoreLibraryResolver{db: db}
}

// Transform implements config.LibraryResolver.
func (r *StoreLibraryResolver) Transform(ctx context.Context, ref string, version int) (config.Steps, error) {
	var steps config.Steps
	return steps, r.load(ctx, transformSQL, ref, version, &steps)
}

// LookupTable implements config.LibraryResolver.
func (r *StoreLibraryResolver) LookupTable(ctx context.Context, ref string, version int) (config.LookupRows, error) {
	var rows config.LookupRows
	return rows, r.load(ctx, lookupSQL, ref, version, &rows)
}

// load decodes the pinned content into out; a missing pin wraps config.ErrLibraryRefMissing.
func (r *StoreLibraryResolver) load(ctx context.Context, sql, ref string, version int, out any) error {
	var raw json.RawMessage
	err := r.db.Q(ctx).QueryRow(ctx, sql, ref, version).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s@%d", config.ErrLibraryRefMissing, ref, version)
	}
	if err != nil {
		return r.db.Translate(err)
	}
	return json.Unmarshal(raw, out)
}

var _ config.LibraryResolver = (*StoreLibraryResolver)(nil)
