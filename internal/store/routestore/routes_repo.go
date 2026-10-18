package routestore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/store"
)

// routeColumns are the columns of a routes row, in scan order.
const routeColumns = `r.id::text, r.team_id::text, r.name, r.description, r.tags, r.desired_state::text,
	r.active_version_id::text, r.replicas, r.draft_config, r.row_version, r.created_by::text, r.updated_by::text,
	r.created_at, r.updated_at`

// summarySelect joins team, runtime, active version and last editor for list/detail rows.
const summarySelect = `SELECT ` + routeColumns + `, t.name, COALESCE(rr.observed_state::text, ''),
	av.id::text, av.version_no, av.config, av.config_hash, u.id::text, u.display_name
	FROM routes r
	JOIN teams t ON t.id = r.team_id
	LEFT JOIN route_runtime rr ON rr.route_id = r.id
	LEFT JOIN route_versions av ON av.id = r.active_version_id
	LEFT JOIN users u ON u.id = r.updated_by`

// RouteRepository is the Postgres routes.RouteRepository.
type RouteRepository struct {
	db *store.DB
}

// NewRouteRepository builds the repository.
func NewRouteRepository(db *store.DB) *RouteRepository {
	return &RouteRepository{db: db}
}

// rowScanner scans the routes columns into a Route.
type rowScanner struct {
	r         routes.Route
	active    *string
	createdBy *string
	updatedBy *string
	draft     []byte
	desired   string
	revision  int64
}

func (s *rowScanner) targets() []any {
	return []any{&s.r.ID, &s.r.TeamID, &s.r.Name, &s.r.Description, &s.r.Tags, &s.desired, &s.active,
		&s.r.Replicas, &s.draft, &s.revision, &s.createdBy, &s.updatedBy, &s.r.CreatedAt, &s.r.UpdatedAt}
}

func (s *rowScanner) route() routes.Route {
	v := Values{}
	s.r.DesiredState = routes.DesiredState(s.desired)
	s.r.ActiveVersionID, s.r.Draft, s.r.Revision = v.VersionID(s.active), v.Raw(s.draft), routes.Revision(s.revision)
	s.r.CreatedBy, s.r.UpdatedBy = v.UserID(s.createdBy), v.UserID(s.updatedBy)
	s.r.Tags = routes.NonNilTags(s.r.Tags)
	return s.r
}

// scanRoute reads one routes row.
func (repo *RouteRepository) scanRoute(row pgx.Row) (routes.Route, error) {
	var s rowScanner
	if err := row.Scan(s.targets()...); err != nil {
		return routes.Route{}, repo.db.Translate(err)
	}
	return s.route(), nil
}

// one runs a query returning one routes row.
func (repo *RouteRepository) one(ctx context.Context, sql string, args ...any) (routes.Route, error) {
	return repo.scanRoute(repo.db.Q(ctx).QueryRow(ctx, sql, args...))
}

// Get implements routes.RouteRepository.
func (repo *RouteRepository) Get(ctx context.Context, id routes.RouteID) (routes.Route, error) {
	uid, err := Params{}.UUID(string(id))
	if err != nil {
		return routes.Route{}, err
	}
	return repo.one(ctx, `SELECT `+routeColumns+` FROM routes r WHERE r.id = $1`, uid)
}

// GetForUpdate locks the row for the rest of the transaction.
func (repo *RouteRepository) GetForUpdate(ctx context.Context, id routes.RouteID) (routes.Route, error) {
	uid, err := Params{}.UUID(string(id))
	if err != nil {
		return routes.Route{}, err
	}
	return repo.one(ctx, `SELECT `+routeColumns+` FROM routes r WHERE r.id = $1 FOR UPDATE`, uid)
}

// Create inserts a route and returns the stored row.
func (repo *RouteRepository) Create(ctx context.Context, r routes.Route) (routes.Route, error) {
	team, err := Params{}.UUID(string(r.TeamID))
	if err != nil {
		return routes.Route{}, err
	}
	return repo.one(ctx, `INSERT INTO routes AS r (team_id, name, description, tags, desired_state, replicas, draft_config, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5::text::route_desired_state, $6, $7::jsonb, $8, $8)
		RETURNING `+routeColumns,
		team, r.Name, r.Description, routes.NonNilTags(r.Tags), string(r.DesiredState), r.Replicas, Params{}.JSON(r.Draft), r.CreatedBy)
}

// SaveDraft applies a versioned update of settings and/or the draft.
func (repo *RouteRepository) SaveDraft(ctx context.Context, w routes.DraftWrite) (routes.Route, error) {
	uid, err := Params{}.UUID(string(w.ID))
	if err != nil {
		return routes.Route{}, err
	}
	set, args := ChangeSet{}.Build(w.Change, w.UpdatedBy)
	upd := store.VersionedUpdate{Table: "routes", IDColumn: "id"}
	if _, err := upd.Exec(ctx, repo.db, set, args, uid, store.RowVersion(w.Expected)); err != nil {
		return routes.Route{}, err
	}
	return repo.Get(ctx, w.ID)
}

// SetDesiredState changes the desired state.
func (repo *RouteRepository) SetDesiredState(ctx context.Context, id routes.RouteID, s routes.DesiredState, by *uuid.UUID) (routes.Route, error) {
	uid, err := Params{}.UUID(string(id))
	if err != nil {
		return routes.Route{}, err
	}
	return repo.one(ctx, `UPDATE routes AS r SET desired_state = $1::text::route_desired_state, updated_by = COALESCE($2, r.updated_by)
		WHERE r.id = $3 RETURNING `+routeColumns, string(s), by, uid)
}

// SetActiveVersion switches the live version and desired state, optionally replacing the draft.
func (repo *RouteRepository) SetActiveVersion(ctx context.Context, w routes.ActivationWrite) (routes.Route, error) {
	uid, err := Params{}.UUID(string(w.ID))
	if err != nil {
		return routes.Route{}, err
	}
	vid, err := Params{}.UUID(string(w.VersionID))
	if err != nil {
		return routes.Route{}, err
	}
	return repo.one(ctx, `UPDATE routes AS r SET active_version_id = $1, desired_state = $2::text::route_desired_state,
		updated_by = COALESCE($3, r.updated_by), draft_config = COALESCE($4::jsonb, r.draft_config)
		WHERE r.id = $5 RETURNING `+routeColumns, vid, string(w.Desired), w.UpdatedBy, Params{}.JSON(w.Draft), uid)
}

// AddTag appends a tag unless present.
func (repo *RouteRepository) AddTag(ctx context.Context, id routes.RouteID, tag string, by *uuid.UUID) (routes.Route, error) {
	uid, err := Params{}.UUID(string(id))
	if err != nil {
		return routes.Route{}, err
	}
	return repo.one(ctx, `UPDATE routes AS r SET tags = CASE WHEN $1::text = ANY(r.tags) THEN r.tags ELSE array_append(r.tags, $1::text) END,
		updated_by = COALESCE($2, r.updated_by) WHERE r.id = $3 RETURNING `+routeColumns, tag, by, uid)
}

// NamesWithPrefix returns the team's route names starting with prefix.
func (repo *RouteRepository) NamesWithPrefix(ctx context.Context, team routes.TeamID, prefix string) (map[string]bool, error) {
	tid, err := Params{}.UUID(string(team))
	if err != nil {
		return nil, err
	}
	rows, err := repo.db.Q(ctx).Query(ctx, `SELECT name FROM routes WHERE team_id = $1 AND left(name, length($2::text)) = $2::text`, tid, prefix)
	if err != nil {
		return nil, repo.db.Translate(err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, repo.db.Translate(err)
	}
	out := map[string]bool{}
	for _, n := range names {
		out[n] = true
	}
	return out, nil
}

// NameTaken reports whether another route of the team uses name.
func (repo *RouteRepository) NameTaken(ctx context.Context, team routes.TeamID, name string, except routes.RouteID) (bool, error) {
	tid, err := Params{}.UUID(string(team))
	if err != nil {
		return false, err
	}
	var taken bool
	err = repo.db.Q(ctx).QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM routes WHERE team_id = $1 AND name = $2 AND id::text <> $3)`,
		tid, name, string(except)).Scan(&taken)
	return taken, repo.db.Translate(err)
}

// FindByName returns the team's route called name (archived ones included; names stay unique).
func (repo *RouteRepository) FindByName(ctx context.Context, team routes.TeamID, name string) (routes.Route, error) {
	tid, err := Params{}.UUID(string(team))
	if err != nil {
		return routes.Route{}, err
	}
	return repo.one(ctx, `SELECT `+routeColumns+` FROM routes AS r WHERE r.team_id = $1 AND r.name = $2`, tid, name)
}

// NameTakenByOther adapts NameTaken to validation.RouteNameIndex.
func (repo *RouteRepository) NameTakenByOther(ctx context.Context, teamID, name, exceptRouteID string) (bool, error) {
	return repo.NameTaken(ctx, routes.TeamID(teamID), name, routes.RouteID(exceptRouteID))
}

// ChangeSet renders the SET clause of a PATCH.
type ChangeSet struct{}

// Build returns "col = $1, …" and its args for the fields present in c.
func (ChangeSet) Build(c routes.RouteChange, by *uuid.UUID) (string, []any) {
	parts, args := []string{}, []any{}
	add := func(expr string, v any) {
		args = append(args, v)
		parts = append(parts, fmt.Sprintf(expr, len(args)))
	}
	if c.Name != nil {
		add("name = $%d", *c.Name)
	}
	if c.Description != nil {
		add("description = $%d", *c.Description)
	}
	if c.Tags != nil {
		add("tags = $%d", routes.NonNilTags(*c.Tags))
	}
	if c.Replicas != nil {
		add("replicas = $%d", *c.Replicas)
	}
	if c.Draft != nil {
		add("draft_config = $%d::jsonb", Params{}.JSON(c.Draft))
	}
	add("updated_by = COALESCE($%d, updated_by)", by)
	return strings.Join(parts, ", "), args
}

// CursorTime formats the list sort key.
type CursorTime struct{}

// Key renders updated_at with full precision.
func (CursorTime) Key(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
