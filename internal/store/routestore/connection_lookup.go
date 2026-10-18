package routestore

import (
	"context"

	"github.com/eventrouter/router/internal/routes/validation"
	"github.com/eventrouter/router/internal/store"
)

// ConnectionLookup reads connection identity from the connections table (read-only; connections owns it).
type ConnectionLookup struct {
	db *store.DB
}

// NewConnectionLookup builds the lookup.
func NewConnectionLookup(db *store.DB) *ConnectionLookup {
	return &ConnectionLookup{db: db}
}

// Find implements validation.ConnectionLookup; ids that are not uuids simply do not match.
func (l *ConnectionLookup) Find(ctx context.Context, ids []string) (map[string]validation.ConnectionInfo, error) {
	out := map[string]validation.ConnectionInfo{}
	if len(ids) == 0 {
		return out, nil
	}
	return l.query(ctx, func(c validation.ConnectionInfo) string { return c.ID }, `SELECT `+connectionColumns+`
		FROM connections WHERE id::text = ANY($1::text[])`, ids)
}

// FindByNames implements portability.ConnectionDirectory: per name the team's connection, else the oldest
// connection of that name in the workspace (use by another team is checked by the grant validator).
func (l *ConnectionLookup) FindByNames(ctx context.Context, teamID string, names []string) (map[string]validation.ConnectionInfo, error) {
	if len(names) == 0 {
		return map[string]validation.ConnectionInfo{}, nil
	}
	return l.query(ctx, func(c validation.ConnectionInfo) string { return c.Name }, `SELECT DISTINCT ON (name) `+connectionColumns+`
		FROM connections WHERE name = ANY($1::text[]) ORDER BY name, (team_id::text = $2::text) DESC, created_at, id`, names, teamID)
}

// connectionColumns are scanned by query in this order.
const connectionColumns = `id::text, team_id::text, name, env_tag, COALESCE(schema_registry->>'url', '') <> ''`

// query scans connection rows keyed by keyOf.
func (l *ConnectionLookup) query(ctx context.Context, keyOf func(validation.ConnectionInfo) string, sql string, args ...any) (map[string]validation.ConnectionInfo, error) {
	out := map[string]validation.ConnectionInfo{}
	rows, err := l.db.Q(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, l.db.Translate(err)
	}
	defer rows.Close()
	for rows.Next() {
		var c validation.ConnectionInfo
		if err := rows.Scan(&c.ID, &c.TeamID, &c.Name, &c.EnvTag, &c.HasSchemaRegistry); err != nil {
			return nil, l.db.Translate(err)
		}
		out[keyOf(c)] = c
	}
	return out, l.db.Translate(rows.Err())
}
