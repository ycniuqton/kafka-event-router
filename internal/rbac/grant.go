package rbac

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/store"
)

// ConnectionGrant lets a team use a connection owned by another team (routes validation enforces it).
type ConnectionGrant struct {
	TeamID         TeamID     `json:"teamId"`
	ConnectionID   uuid.UUID  `json:"connectionId"`
	ConnectionName string     `json:"connectionName"`
	OwnerTeamID    TeamID     `json:"ownerTeamId"`
	EnvTag         EnvTag     `json:"envTag"`
	GrantedBy      *uuid.UUID `json:"grantedBy"`
	CreatedAt      time.Time  `json:"createdAt"`
}

// ConnectionInfo is what access control reads about a connection (owned by the connections feature).
type ConnectionInfo struct {
	ID        uuid.UUID
	TeamID    TeamID
	Name      string
	EnvTag    EnvTag
	Sensitive bool
}

// Resource returns the connection as an authorization target.
func (c ConnectionInfo) Resource() Resource {
	return Resource{Type: ResourceConnection, ID: c.ID.String(), TeamID: c.TeamID, EnvTag: c.EnvTag, Sensitive: c.Sensitive}
}

// ConnectionDirectory reads connection ownership, environment and sensitivity.
type ConnectionDirectory interface {
	Find(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]ConnectionInfo, error)
}

// GrantStore persists team connection grants.
type GrantStore interface {
	List(ctx context.Context, team TeamID) ([]ConnectionGrant, error)
	Exists(ctx context.Context, team TeamID, conn uuid.UUID) (bool, error)
	Insert(ctx context.Context, team TeamID, conn uuid.UUID, by *uuid.UUID) (bool, error)
	Delete(ctx context.Context, team TeamID, conn uuid.UUID) (bool, error)
}

// PostgresConnectionDirectory implements ConnectionDirectory on connections.
type PostgresConnectionDirectory struct {
	db *store.DB
}

// NewPostgresConnectionDirectory builds the directory.
func NewPostgresConnectionDirectory(db *store.DB) *PostgresConnectionDirectory {
	return &PostgresConnectionDirectory{db: db}
}

// Find returns the known connections among ids.
func (d *PostgresConnectionDirectory) Find(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]ConnectionInfo, error) {
	out := map[uuid.UUID]ConnectionInfo{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := d.db.Q(ctx).Query(ctx, `SELECT id, team_id, name, env_tag, sensitive FROM connections WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, d.db.Translate(err)
	}
	defer rows.Close()
	for rows.Next() {
		var c ConnectionInfo
		var env string
		if err := rows.Scan(&c.ID, &c.TeamID, &c.Name, &env, &c.Sensitive); err != nil {
			return nil, err
		}
		c.EnvTag = EnvTag(env)
		out[c.ID] = c
	}
	return out, rows.Err()
}

// PostgresGrantStore implements GrantStore on team_connection_grants.
type PostgresGrantStore struct {
	db *store.DB
}

// NewPostgresGrantStore builds the store.
func NewPostgresGrantStore(db *store.DB) *PostgresGrantStore {
	return &PostgresGrantStore{db: db}
}

// List returns the team's grants with connection names, ordered by connection name.
func (s *PostgresGrantStore) List(ctx context.Context, team TeamID) ([]ConnectionGrant, error) {
	rows, err := s.db.Q(ctx).Query(ctx, `SELECT g.team_id, g.connection_id, c.name, c.team_id, c.env_tag, g.granted_by, g.created_at
		FROM team_connection_grants g JOIN connections c ON c.id = g.connection_id WHERE g.team_id = $1 ORDER BY c.name`, team)
	if err != nil {
		return nil, s.db.Translate(err)
	}
	defer rows.Close()
	out := []ConnectionGrant{}
	for rows.Next() {
		g, err := s.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// Exists reports whether the grant exists.
func (s *PostgresGrantStore) Exists(ctx context.Context, team TeamID, conn uuid.UUID) (bool, error) {
	var ok bool
	err := s.db.Q(ctx).QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM team_connection_grants WHERE team_id = $1 AND connection_id = $2)`,
		team, conn).Scan(&ok)
	return ok, s.db.Translate(err)
}

// Insert adds the grant; it reports false when it already existed.
func (s *PostgresGrantStore) Insert(ctx context.Context, team TeamID, conn uuid.UUID, by *uuid.UUID) (bool, error) {
	tag, err := s.db.Q(ctx).Exec(ctx, `INSERT INTO team_connection_grants (team_id, connection_id, granted_by) VALUES ($1, $2, $3)
		ON CONFLICT (team_id, connection_id) DO NOTHING`, team, conn, by)
	if err != nil {
		return false, s.db.Translate(err)
	}
	return tag.RowsAffected() > 0, nil
}

// Delete removes the grant; it reports false when there was none.
func (s *PostgresGrantStore) Delete(ctx context.Context, team TeamID, conn uuid.UUID) (bool, error) {
	tag, err := s.db.Q(ctx).Exec(ctx, `DELETE FROM team_connection_grants WHERE team_id = $1 AND connection_id = $2`, team, conn)
	if err != nil {
		return false, s.db.Translate(err)
	}
	return tag.RowsAffected() > 0, nil
}

func (s *PostgresGrantStore) scan(row pgx.Row) (ConnectionGrant, error) {
	var g ConnectionGrant
	var env string
	err := row.Scan(&g.TeamID, &g.ConnectionID, &g.ConnectionName, &g.OwnerTeamID, &env, &g.GrantedBy, &g.CreatedAt)
	g.EnvTag = EnvTag(env)
	return g, err
}
