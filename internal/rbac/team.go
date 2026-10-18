package rbac

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/store"
)

// Team owns routes, connections, transforms and lookup tables; its policy decides production approvals.
type Team struct {
	ID                  TeamID           `json:"id"`
	Name                string           `json:"name"`
	RequireProdApproval bool             `json:"requireProdApproval"`
	MemberCount         int              `json:"memberCount"`
	CreatedAt           time.Time        `json:"createdAt"`
	RowVersion          store.RowVersion `json:"rowVersion"`
}

// TeamChange is a partial team update (nil members unchanged).
type TeamChange struct {
	Name                *string
	RequireProdApproval *bool
}

// TeamUsage counts the objects a team owns (a team that owns anything cannot be deleted).
type TeamUsage struct {
	Routes       int `json:"routes"`
	Connections  int `json:"connections"`
	Transforms   int `json:"transforms"`
	LookupTables int `json:"lookupTables"`
}

// Empty reports whether the team owns nothing.
func (u TeamUsage) Empty() bool {
	return u.Routes+u.Connections+u.Transforms+u.LookupTables == 0
}

// TeamNameLimit bounds team names.
type TeamNameLimit int

const MaxTeamName TeamNameLimit = 100

// TeamName validates and normalises team names.
type TeamName struct{}

// Normalise trims the name and rejects empty or overlong names.
func (TeamName) Normalise(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || utf8.RuneCountInString(name) > int(MaxTeamName) {
		return "", ErrInvalidTeamName
	}
	return name, nil
}

// TeamStore persists teams.
type TeamStore interface {
	List(ctx context.Context) ([]Team, error)
	Get(ctx context.Context, id TeamID) (Team, error)
	Insert(ctx context.Context, name string, requireProdApproval bool) (Team, error)
	Update(ctx context.Context, id TeamID, c TeamChange, expected store.RowVersion) error
	Delete(ctx context.Context, id TeamID) error
	Usage(ctx context.Context, id TeamID) (TeamUsage, error)
	NameTaken(ctx context.Context, name string, except TeamID) (bool, error)
}

// PostgresTeamStore implements TeamStore on teams.
type PostgresTeamStore struct {
	db *store.DB
}

// NewPostgresTeamStore builds the store.
func NewPostgresTeamStore(db *store.DB) *PostgresTeamStore {
	return &PostgresTeamStore{db: db}
}

// selectTeamsSQL is the projection shared by team reads.
const selectTeamsSQL = `SELECT t.id, t.name, t.require_prod_approval,
	(SELECT count(*) FROM role_bindings rb WHERE rb.team_id = t.id), t.created_at, t.row_version FROM teams t`

// List returns every team ordered by name.
func (s *PostgresTeamStore) List(ctx context.Context) ([]Team, error) {
	rows, err := s.db.Q(ctx).Query(ctx, selectTeamsSQL+` ORDER BY t.name`)
	if err != nil {
		return nil, s.db.Translate(err)
	}
	defer rows.Close()
	out := []Team{}
	for rows.Next() {
		t, err := s.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Get returns one team (store.ErrNotFound when absent).
func (s *PostgresTeamStore) Get(ctx context.Context, id TeamID) (Team, error) {
	t, err := s.scan(s.db.Q(ctx).QueryRow(ctx, selectTeamsSQL+` WHERE t.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Team{}, store.ErrNotFound
	}
	return t, s.db.Translate(err)
}

// Insert creates a team.
func (s *PostgresTeamStore) Insert(ctx context.Context, name string, requireProdApproval bool) (Team, error) {
	var id TeamID
	err := s.db.Q(ctx).QueryRow(ctx, `INSERT INTO teams (name, require_prod_approval) VALUES ($1, $2) RETURNING id`,
		name, requireProdApproval).Scan(&id)
	if err != nil {
		return Team{}, s.db.Translate(err)
	}
	return s.Get(ctx, id)
}

// Update applies the change under optimistic concurrency (412 on a stale version).
func (s *PostgresTeamStore) Update(ctx context.Context, id TeamID, c TeamChange, expected store.RowVersion) error {
	set := `name = COALESCE($1, name), require_prod_approval = COALESCE($2, require_prod_approval)`
	_, err := store.VersionedUpdate{Table: "teams", IDColumn: "id"}.Exec(ctx, s.db, set, []any{c.Name, c.RequireProdApproval}, id, expected)
	return err
}

// Delete removes a team (its bindings and grants cascade).
func (s *PostgresTeamStore) Delete(ctx context.Context, id TeamID) error {
	_, err := s.db.Q(ctx).Exec(ctx, `DELETE FROM teams WHERE id = $1`, id)
	return s.db.Translate(err)
}

// Usage counts the objects owned by the team.
func (s *PostgresTeamStore) Usage(ctx context.Context, id TeamID) (TeamUsage, error) {
	var u TeamUsage
	err := s.db.Q(ctx).QueryRow(ctx, `SELECT
		(SELECT count(*) FROM routes WHERE team_id = $1), (SELECT count(*) FROM connections WHERE team_id = $1),
		(SELECT count(*) FROM transforms WHERE team_id = $1), (SELECT count(*) FROM lookup_tables WHERE team_id = $1)`, id).
		Scan(&u.Routes, &u.Connections, &u.Transforms, &u.LookupTables)
	return u, s.db.Translate(err)
}

// NameTaken reports whether another team uses name (case-insensitive).
func (s *PostgresTeamStore) NameTaken(ctx context.Context, name string, except TeamID) (bool, error) {
	var taken bool
	err := s.db.Q(ctx).QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM teams WHERE lower(name) = lower($1) AND id <> $2)`,
		name, except).Scan(&taken)
	return taken, s.db.Translate(err)
}

func (s *PostgresTeamStore) scan(row pgx.Row) (Team, error) {
	var t Team
	var version int64
	err := row.Scan(&t.ID, &t.Name, &t.RequireProdApproval, &t.MemberCount, &t.CreatedAt, &version)
	t.RowVersion = store.RowVersion(version)
	return t, err
}

// TeamPolicyReader exposes a team's approval policy to approvals (team lookups by id).
type TeamPolicyReader interface {
	Get(ctx context.Context, id TeamID) (Team, error)
}

var _ TeamPolicyReader = (*PostgresTeamStore)(nil)

// NilTeam is the zero team id (collections, workspace-scoped objects).
var NilTeam = uuid.Nil
