// Package flowmap builds the dashboard flow map: topics as nodes, deployed routes as edges between their
// source topics and each branch's destination topic, weighted by live throughput.
package flowmap

import (
	"context"
	"encoding/json"

	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/pkg/config"
)

// RouteRow is one deployed, non-archived route with its active configuration.
type RouteRow struct {
	ID       string
	Name     string
	TeamID   string
	TeamName string
	Tags     []string
	Status   routes.Status
	Config   config.RouteConfig
}

// ConnectionRow is one connection the map can label and filter by.
type ConnectionRow struct {
	ID     string
	Name   string
	EnvTag string
}

// Store reads the flow map inputs.
type Store interface {
	Routes(ctx context.Context) ([]RouteRow, error)
	Connections(ctx context.Context) (map[string]ConnectionRow, error)
}

const (
	routesSQL = `SELECT r.id::text, r.name, r.team_id::text, COALESCE(t.name, ''), r.tags, r.desired_state::text,
       COALESCE(rt.observed_state::text, ''), v.config
  FROM routes r JOIN route_versions v ON v.id = r.active_version_id
  LEFT JOIN teams t ON t.id = r.team_id
  LEFT JOIN route_runtime rt ON rt.route_id = r.id
 WHERE r.desired_state <> 'archived' ORDER BY r.name, r.id`
	connectionsSQL = `SELECT id::text, name, env_tag FROM connections`
)

// PgStore implements Store on Postgres.
type PgStore struct {
	DB *store.DB
}

// Routes implements Store; routes whose config cannot be decoded are skipped.
func (s PgStore) Routes(ctx context.Context) ([]RouteRow, error) {
	rows, err := s.DB.Q(ctx).Query(ctx, routesSQL)
	if err != nil {
		return nil, s.DB.Translate(err)
	}
	defer rows.Close()
	out := []RouteRow{}
	for rows.Next() {
		var r RouteRow
		var desired, observed string
		var raw []byte
		if err := rows.Scan(&r.ID, &r.Name, &r.TeamID, &r.TeamName, &r.Tags, &desired, &observed, &raw); err != nil {
			return nil, err
		}
		if json.Unmarshal(raw, &r.Config) != nil {
			continue
		}
		r.Status = routes.RouteStatusResolver{}.Resolve(routes.DesiredState(desired), true, routes.ObservedState(observed))
		out = append(out, r)
	}
	return out, s.DB.Translate(rows.Err())
}

// Connections implements Store.
func (s PgStore) Connections(ctx context.Context) (map[string]ConnectionRow, error) {
	rows, err := s.DB.Q(ctx).Query(ctx, connectionsSQL)
	if err != nil {
		return nil, s.DB.Translate(err)
	}
	defer rows.Close()
	out := map[string]ConnectionRow{}
	for rows.Next() {
		var c ConnectionRow
		if err := rows.Scan(&c.ID, &c.Name, &c.EnvTag); err != nil {
			return nil, err
		}
		out[c.ID] = c
	}
	return out, s.DB.Translate(rows.Err())
}
