package testcases

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/store"
)

// SQL of the test case store; %[1]s is the owner column from OwnerColumns (never from input).
const (
	caseColumns = `c.id, c.branch_id, coalesce(c.name, ''), c.input_event, c.expected_outcome, c.expected_output,
       coalesce(u.display_name, ''), c.created_at, c.updated_at, c.row_version`
	listCasesSQL = `SELECT ` + caseColumns + ` FROM test_cases c LEFT JOIN users u ON u.id = c.created_by
 WHERE c.%[1]s = $1 ORDER BY c.created_at, c.id`
	getCaseSQL = `SELECT ` + caseColumns + ` FROM test_cases c LEFT JOIN users u ON u.id = c.created_by
 WHERE c.%[1]s = $1 AND c.id = $2`
	countCasesSQL = `SELECT count(*) FROM test_cases WHERE %[1]s = $1`
	insertCaseSQL = `INSERT INTO test_cases (%[1]s, branch_id, name, input_event, expected_outcome, expected_output, created_by)
 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`
	updateCaseSet = `name = coalesce($1, name), expected_outcome = coalesce($2, expected_outcome),
 expected_output = CASE WHEN $3 THEN $4::jsonb ELSE expected_output END`
	deleteCaseSQL = `DELETE FROM test_cases WHERE %[1]s = $1 AND id = $2`
	routeInfoSQL  = `SELECT r.team_id::text, r.name, coalesce(r.draft_config, av.config)
  FROM routes r LEFT JOIN route_versions av ON av.id = r.active_version_id WHERE r.id = $1`
	routeTeamSQL = `SELECT team_id::text FROM routes WHERE id::text = $1`
	transformSQL = `SELECT o.team_id::text, o.name, coalesce(v.steps, '[]'::jsonb) FROM transforms o
  LEFT JOIN LATERAL (SELECT steps FROM transform_versions WHERE transform_id = o.id ORDER BY version_no DESC LIMIT 1) v ON true
 WHERE o.id = $1`
)

// TableTestCases is the table of saved cases.
const TableTestCases = "test_cases"

// OwnerColumns maps owner kinds to their foreign-key column.
var OwnerColumns = map[OwnerKind]string{OwnerRoute: "route_id", OwnerTransform: "transform_id"}

// Store persists test cases (Postgres in production, in-memory in tests).
type Store interface {
	List(ctx context.Context, owner Owner) ([]TestCase, error)
	Count(ctx context.Context, owner Owner) (int, error)
	Get(ctx context.Context, owner Owner, id uuid.UUID) (TestCase, error)
	Create(ctx context.Context, owner Owner, c NewCase) (TestCase, error)
	Update(ctx context.Context, owner Owner, id uuid.UUID, p CasePatch, expected store.RowVersion) (TestCase, error)
	Delete(ctx context.Context, owner Owner, id uuid.UUID) error
}

// RouteInfo is what a run needs about a route: owner team, name and its stored draft (or active config).
type RouteInfo struct {
	TeamID string
	Name   string
	Draft  json.RawMessage
}

// RouteSource reads routes (Postgres in production).
type RouteSource interface {
	Route(ctx context.Context, id uuid.UUID) (RouteInfo, error)
	TeamOf(ctx context.Context, id string) (string, error)
}

// Repository is the Postgres Store and RouteSource.
type Repository struct {
	db *store.DB
}

// NewRepository builds the repository.
func NewRepository(db *store.DB) *Repository {
	return &Repository{db: db}
}

// List returns the owner's cases, oldest first.
func (r *Repository) List(ctx context.Context, owner Owner) ([]TestCase, error) {
	sql, err := r.sql(listCasesSQL, owner)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.Q(ctx).Query(ctx, sql, owner.ID)
	if err != nil {
		return nil, r.db.Translate(err)
	}
	defer rows.Close()
	out := []TestCase{}
	for rows.Next() {
		c, err := r.scan(rows, owner)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, r.db.Translate(rows.Err())
}

// Count returns how many cases the owner has.
func (r *Repository) Count(ctx context.Context, owner Owner) (int, error) {
	sql, err := r.sql(countCasesSQL, owner)
	if err != nil {
		return 0, err
	}
	var n int
	if err := r.db.Q(ctx).QueryRow(ctx, sql, owner.ID).Scan(&n); err != nil {
		return 0, r.db.Translate(err)
	}
	return n, nil
}

// Get returns one case of the owner (ErrNotFound otherwise).
func (r *Repository) Get(ctx context.Context, owner Owner, id uuid.UUID) (TestCase, error) {
	sql, err := r.sql(getCaseSQL, owner)
	if err != nil {
		return TestCase{}, err
	}
	return r.scan(r.db.Q(ctx).QueryRow(ctx, sql, owner.ID, id), owner)
}

// Create inserts a case and returns it.
func (r *Repository) Create(ctx context.Context, owner Owner, c NewCase) (TestCase, error) {
	sql, err := r.sql(insertCaseSQL, owner)
	if err != nil {
		return TestCase{}, err
	}
	var author *uuid.UUID
	if c.Author != uuid.Nil {
		author = &c.Author
	}
	var id uuid.UUID
	err = r.db.Q(ctx).QueryRow(ctx, sql, owner.ID, c.BranchID, c.Name, []byte(c.InputEvent), string(c.ExpectedOutcome),
		NullableJSON{}.Of(c.ExpectedOutput), author).Scan(&id)
	if err != nil {
		return TestCase{}, r.db.Translate(err)
	}
	return r.Get(ctx, owner, id)
}

// Update applies p when the row version still matches (412 otherwise).
func (r *Repository) Update(ctx context.Context, owner Owner, id uuid.UUID, p CasePatch, expected store.RowVersion) (TestCase, error) {
	if _, err := r.Get(ctx, owner, id); err != nil {
		return TestCase{}, err
	}
	var outcome *string
	if p.ExpectedOutcome != nil {
		o := string(*p.ExpectedOutcome)
		outcome = &o
	}
	args := []any{p.Name, outcome, p.ExpectedOutput != nil, NullableJSON{}.Of(p.ExpectedOutput)}
	upd := store.VersionedUpdate{Table: TableTestCases, IDColumn: string(ColumnID)}
	if _, err := upd.Exec(ctx, r.db, updateCaseSet, args, id, expected); err != nil {
		return TestCase{}, err
	}
	return r.Get(ctx, owner, id)
}

// Delete removes one case of the owner.
func (r *Repository) Delete(ctx context.Context, owner Owner, id uuid.UUID) error {
	sql, err := r.sql(deleteCaseSQL, owner)
	if err != nil {
		return err
	}
	tag, err := r.db.Q(ctx).Exec(ctx, sql, owner.ID, id)
	if err != nil {
		return r.db.Translate(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Route returns the route's team, name and draft (the active config when no draft is stored).
func (r *Repository) Route(ctx context.Context, id uuid.UUID) (RouteInfo, error) {
	var info RouteInfo
	var draft []byte
	if err := r.db.Q(ctx).QueryRow(ctx, routeInfoSQL, id).Scan(&info.TeamID, &info.Name, &draft); err != nil {
		return RouteInfo{}, r.db.Translate(err)
	}
	info.Draft = draft
	return info, nil
}

// Transform returns a library transform's team, name and latest steps (TransformSource).
func (r *Repository) Transform(ctx context.Context, id uuid.UUID) (TransformInfo, error) {
	var info TransformInfo
	var steps []byte
	if err := r.db.Q(ctx).QueryRow(ctx, transformSQL, id).Scan(&info.TeamID, &info.Name, &steps); err != nil {
		return TransformInfo{}, r.db.Translate(err)
	}
	info.Steps = steps
	return info, nil
}

// TeamOf implements rbac.TeamOwner for /routes/{id}/test-cases.
func (r *Repository) TeamOf(ctx context.Context, id string) (string, error) {
	var team string
	if err := r.db.Q(ctx).QueryRow(ctx, routeTeamSQL, id).Scan(&team); err != nil {
		return "", r.db.Translate(err)
	}
	return team, nil
}

// sql fills the owner column into a template.
func (r *Repository) sql(template string, owner Owner) (string, error) {
	col, ok := OwnerColumns[owner.Kind]
	if !ok {
		return "", ErrUnknownOwner
	}
	return fmt.Sprintf(template, col), nil
}

// scan reads one case row.
func (r *Repository) scan(row pgx.Row, owner Owner) (TestCase, error) {
	c := TestCase{Owner: owner}
	var input, output []byte
	var outcome string
	var version int64
	err := row.Scan(&c.ID, &c.BranchID, &c.Name, &input, &outcome, &output, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &version)
	if err != nil {
		return TestCase{}, r.db.Translate(err)
	}
	c.InputEvent, c.ExpectedOutcome, c.RowVersion = input, Outcome(outcome), store.RowVersion(version)
	c.ExpectedOutput = json.RawMessage(nullJSON)
	if output != nil {
		c.ExpectedOutput = output
	}
	return c, nil
}

// Column names columns addressed by generic store helpers.
type Column string

const ColumnID Column = "id"

// NullableJSON maps JSON null (or nothing) to SQL NULL.
type NullableJSON struct{}

// Of returns nil for null/empty, else the bytes.
func (NullableJSON) Of(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == nullJSON {
		return nil
	}
	return []byte(raw)
}
