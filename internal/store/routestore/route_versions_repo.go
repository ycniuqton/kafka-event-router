package routestore

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/store"
)

// versionSelect reads versions with their author and the number of the version they are based on.
const versionSelect = `SELECT v.id::text, v.route_id::text, v.version_no, v.config, v.config_hash, v.origin::text,
	v.based_on_version_id::text, bv.version_no, v.created_by::text, u.display_name, COALESCE(v.change_note, ''), v.created_at
	FROM route_versions v
	LEFT JOIN route_versions bv ON bv.id = v.based_on_version_id
	LEFT JOIN users u ON u.id = v.created_by`

// VersionRepository is the Postgres routes.VersionRepository.
type VersionRepository struct {
	db *store.DB
}

// NewVersionRepository builds the repository.
func NewVersionRepository(db *store.DB) *VersionRepository {
	return &VersionRepository{db: db}
}

// versionScanner scans versionSelect rows.
type versionScanner struct {
	v         routes.RouteVersion
	cfg       []byte
	origin    string
	basedOn   *string
	createdBy *string
	userName  *string
}

func (s *versionScanner) targets() []any {
	return []any{&s.v.ID, &s.v.RouteID, &s.v.No, &s.cfg, &s.v.ConfigHash, &s.origin, &s.basedOn, &s.v.BasedOnNo,
		&s.createdBy, &s.userName, &s.v.ChangeNote, &s.v.CreatedAt}
}

func (s *versionScanner) version() routes.RouteVersion {
	vals := Values{}
	s.v.Config, s.v.Origin, s.v.BasedOn = vals.Raw(s.cfg), routes.VersionOrigin(s.origin), vals.VersionID(s.basedOn)
	s.v.CreatedBy, s.v.Author = vals.UserID(s.createdBy), vals.UserRef(s.createdBy, s.userName)
	return s.v
}

// Create allocates version_no = max+1 under a row lock on the route and inserts the version.
func (repo *VersionRepository) Create(ctx context.Context, v routes.RouteVersion) (routes.RouteVersion, error) {
	rid, err := Params{}.UUID(string(v.RouteID))
	if err != nil {
		return routes.RouteVersion{}, err
	}
	if _, err := repo.db.Q(ctx).Exec(ctx, `SELECT 1 FROM routes WHERE id = $1 FOR UPDATE`, rid); err != nil {
		return routes.RouteVersion{}, repo.db.Translate(err)
	}
	var id string
	err = repo.db.Q(ctx).QueryRow(ctx, `INSERT INTO route_versions (route_id, version_no, config, config_hash, created_by, change_note, origin, based_on_version_id)
		SELECT $1, COALESCE(MAX(version_no), 0) + 1, $2::jsonb, $3, $4, NULLIF($5::text, ''), $6::text::route_version_origin, $7::text::uuid
		FROM route_versions WHERE route_id = $1
		RETURNING id::text`, rid, Params{}.JSON(v.Config), v.ConfigHash, v.CreatedBy, v.ChangeNote, string(v.Origin), v.BasedOn).Scan(&id)
	if err != nil {
		return routes.RouteVersion{}, repo.db.Translate(err)
	}
	return repo.GetByID(ctx, routes.VersionID(id))
}

// List returns versions newest first, keyset-paged by version_no.
func (repo *VersionRepository) List(ctx context.Context, id routes.RouteID, page routes.PageRequest) ([]routes.RouteVersion, *store.Cursor, error) {
	rid, err := Params{}.UUID(string(id))
	if err != nil {
		return nil, nil, err
	}
	sql, args := store.KeysetQuery{Select: versionSelect, Where: []string{"v.route_id = $1"}, Args: []any{rid},
		SortColumn: "v.version_no", SortCast: "int", IDColumn: "v.id", IDCast: "uuid", Limit: page.Limit, After: page.After}.Build()
	rows, err := repo.db.Q(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, nil, repo.db.Translate(err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (routes.RouteVersion, error) {
		var s versionScanner
		err := row.Scan(s.targets()...)
		return s.version(), err
	})
	if err != nil {
		return nil, nil, repo.db.Translate(err)
	}
	kept, next := store.PageSlicer[routes.RouteVersion]{Limit: page.Limit, KeyOf: func(v routes.RouteVersion) store.Cursor {
		return store.Cursor{SortKey: []string{strconv.Itoa(v.No)}, ID: string(v.ID)}
	}}.Slice(list)
	return kept, next, nil
}

// GetByNo reads version no of a route.
func (repo *VersionRepository) GetByNo(ctx context.Context, id routes.RouteID, no int) (routes.RouteVersion, error) {
	rid, err := Params{}.UUID(string(id))
	if err != nil {
		return routes.RouteVersion{}, err
	}
	return repo.one(ctx, versionSelect+` WHERE v.route_id = $1 AND v.version_no = $2`, rid, no)
}

// GetByID reads one version.
func (repo *VersionRepository) GetByID(ctx context.Context, id routes.VersionID) (routes.RouteVersion, error) {
	vid, err := Params{}.UUID(string(id))
	if err != nil {
		return routes.RouteVersion{}, err
	}
	return repo.one(ctx, versionSelect+` WHERE v.id = $1`, vid)
}

func (repo *VersionRepository) one(ctx context.Context, sql string, args ...any) (routes.RouteVersion, error) {
	var s versionScanner
	if err := repo.db.Q(ctx).QueryRow(ctx, sql, args...).Scan(s.targets()...); err != nil {
		return routes.RouteVersion{}, repo.db.Translate(err)
	}
	return s.version(), nil
}
