// Package dashboard builds the dashboard: health summary, needs-attention rows, recent activity and
// the first-run checklist, from Postgres reads plus metrics-store queries.
package dashboard

import (
	"context"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/store"
)

// RouteRow is one non-archived route with its derived status.
type RouteRow struct {
	ID           routes.RouteID
	Name         string
	TeamID       routes.TeamID
	Status       routes.Status
	ErrorMessage string
}

// ConnectionRow is one connection failing its health check, with the routes it affects.
type ConnectionRow struct {
	ID             string
	Name           string
	Health         connections.HealthStatus
	AffectedRoutes int
}

// OnboardingCounts are the facts the first-run checklist derives from.
type OnboardingCounts struct {
	Connections       int
	Routes            int
	ActiveDeployments int
}

// RouteSnapshotReader lists live routes (Postgres routes ⨝ route_runtime).
type RouteSnapshotReader interface {
	LiveRoutes(ctx context.Context) ([]RouteRow, error)
}

// ConnectionHealthReader lists connections whose last check failed.
type ConnectionHealthReader interface {
	Unhealthy(ctx context.Context) ([]ConnectionRow, error)
}

// OnboardingCounter counts connections, routes and active deployments.
type OnboardingCounter interface {
	OnboardingCounts(ctx context.Context) (OnboardingCounts, error)
}

// ConnectionUsage counts the routes using each connection (connections' pgstore.UsageResolver).
type ConnectionUsage interface {
	CountsFor(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int, error)
}

// UnhealthyStatuses are the health values that put a connection on the attention list.
var UnhealthyStatuses = []connections.HealthStatus{connections.HealthDegraded, connections.HealthUnreachable, connections.HealthAuthFailed}

// SQL of the dashboard reads (reads are open across teams).
const (
	liveRoutesSQL = `SELECT r.id, r.name, r.team_id, r.desired_state, r.active_version_id IS NOT NULL,
       COALESCE(rt.observed_state::text, ''), COALESCE(rt.error_message, '')
  FROM routes r LEFT JOIN route_runtime rt ON rt.route_id = r.id
 WHERE r.desired_state <> 'archived'
 ORDER BY r.name, r.id`
	unhealthySQL = `SELECT c.id, c.name, c.health_status::text FROM connections c
 WHERE c.health_status::text = ANY($1) AND c.kind <> 'demo' ORDER BY c.name, c.id`
	onboardingSQL = `SELECT (SELECT count(*) FROM connections WHERE kind <> 'demo'),
       (SELECT count(*) FROM routes WHERE desired_state <> 'archived'),
       (SELECT count(*) FROM deployments WHERE status = 'active')`
)

// PgDashboardStore implements the dashboard reads on Postgres.
type PgDashboardStore struct {
	db    *store.DB
	usage ConnectionUsage
}

// NewPgDashboardStore builds the store; usage may be nil (affected counts stay 0).
func NewPgDashboardStore(db *store.DB, usage ConnectionUsage) *PgDashboardStore {
	return &PgDashboardStore{db: db, usage: usage}
}

// LiveRoutes implements RouteSnapshotReader.
func (s *PgDashboardStore) LiveRoutes(ctx context.Context) ([]RouteRow, error) {
	rows, err := s.db.Q(ctx).Query(ctx, liveRoutesSQL)
	if err != nil {
		return nil, s.db.Translate(err)
	}
	defer rows.Close()
	out := []RouteRow{}
	for rows.Next() {
		var r RouteRow
		var desired, observed string
		var hasActive bool
		if err := rows.Scan(&r.ID, &r.Name, &r.TeamID, &desired, &hasActive, &observed, &r.ErrorMessage); err != nil {
			return nil, err
		}
		r.Status = routes.RouteStatusResolver{}.Resolve(routes.DesiredState(desired), hasActive, routes.ObservedState(observed))
		out = append(out, r)
	}
	return out, s.db.Translate(rows.Err())
}

// Unhealthy implements ConnectionHealthReader.
func (s *PgDashboardStore) Unhealthy(ctx context.Context) ([]ConnectionRow, error) {
	statuses := make([]string, len(UnhealthyStatuses))
	for i, h := range UnhealthyStatuses {
		statuses[i] = string(h)
	}
	rows, err := s.db.Q(ctx).Query(ctx, unhealthySQL, statuses)
	if err != nil {
		return nil, s.db.Translate(err)
	}
	defer rows.Close()
	out := []ConnectionRow{}
	for rows.Next() {
		var c ConnectionRow
		if err := rows.Scan(&c.ID, &c.Name, &c.Health); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, s.db.Translate(err)
	}
	return out, ConnectionUsageFiller{Usage: s.usage}.Fill(ctx, out)
}

// OnboardingCounts implements OnboardingCounter.
func (s *PgDashboardStore) OnboardingCounts(ctx context.Context) (OnboardingCounts, error) {
	var c OnboardingCounts
	err := s.db.Q(ctx).QueryRow(ctx, onboardingSQL).Scan(&c.Connections, &c.Routes, &c.ActiveDeployments)
	return c, s.db.Translate(err)
}

// ConnectionUsageFiller sets AffectedRoutes from the usage resolver.
type ConnectionUsageFiller struct {
	Usage ConnectionUsage
}

// Fill updates rows in place.
func (f ConnectionUsageFiller) Fill(ctx context.Context, rows []ConnectionRow) error {
	if f.Usage == nil || len(rows) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		if id, err := uuid.Parse(r.ID); err == nil {
			ids = append(ids, id)
		}
	}
	counts, err := f.Usage.CountsFor(ctx, ids)
	if err != nil {
		return err
	}
	for i := range rows {
		if id, err := uuid.Parse(rows[i].ID); err == nil {
			rows[i].AffectedRoutes = counts[id]
		}
	}
	return nil
}
