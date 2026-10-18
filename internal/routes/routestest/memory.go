// Package routestest provides in-memory route repositories, a snapshotting TxManager and a
// recording event publisher so routes managers and handlers can be tested without Postgres.
package routestest

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/store"
)

// State is the whole in-memory database.
type State struct {
	Routes      map[routes.RouteID]routes.Route
	Versions    map[routes.VersionID]routes.RouteVersion
	Deployments map[routes.DeploymentID]routes.Deployment
	Observed    map[routes.RouteID]routes.ObservedState
	Teams       map[routes.TeamID]string
}

// clone copies the maps (values are replaced, never mutated in place).
func (s State) clone() State {
	c := State{Routes: map[routes.RouteID]routes.Route{}, Versions: map[routes.VersionID]routes.RouteVersion{},
		Deployments: map[routes.DeploymentID]routes.Deployment{}, Observed: map[routes.RouteID]routes.ObservedState{}, Teams: map[routes.TeamID]string{}}
	for k, v := range s.Routes {
		v.Tags = append([]string{}, v.Tags...)
		c.Routes[k] = v
	}
	for k, v := range s.Versions {
		c.Versions[k] = v
	}
	for k, v := range s.Deployments {
		c.Deployments[k] = v
	}
	for k, v := range s.Observed {
		c.Observed[k] = v
	}
	for k, v := range s.Teams {
		c.Teams[k] = v
	}
	return c
}

// DB is the shared store behind the three repositories.
type DB struct {
	mu    sync.Mutex
	state State
	clock time.Time
}

// NewDB returns an empty database with one team.
func NewDB(team routes.TeamID, teamName string) *DB {
	db := &DB{state: State{}.clone(), clock: time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)}
	db.state.Teams[team] = teamName
	return db
}

// now advances a deterministic clock by one millisecond per call.
func (db *DB) now() time.Time {
	db.clock = db.clock.Add(time.Millisecond)
	return db.clock
}

// SetObserved simulates route-runtime reporting a runtime state.
func (db *DB) SetObserved(id routes.RouteID, s routes.ObservedState) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.state.Observed[id] = s
}

// Repositories returns the three repositories over db.
func (db *DB) Repositories() routes.Repositories {
	return routes.Repositories{Routes: &RouteRepo{db: db}, Versions: &VersionRepo{db: db}, Deployments: &DeploymentRepo{db: db}}
}

// Tx is a TxManager that restores the state when the unit of work fails.
type Tx struct {
	DB *DB
}

type txKey struct{}

// WithinTx snapshots the state and restores it on error; nested calls join.
func (t Tx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if InTx(ctx) {
		return fn(ctx)
	}
	t.DB.mu.Lock()
	snapshot := t.DB.state.clone()
	t.DB.mu.Unlock()
	if err := fn(context.WithValue(ctx, txKey{}, true)); err != nil {
		t.DB.mu.Lock()
		t.DB.state = snapshot
		t.DB.mu.Unlock()
		return err
	}
	return nil
}

// InTx reports whether ctx is inside a Tx unit of work.
func InTx(ctx context.Context) bool {
	v, _ := ctx.Value(txKey{}).(bool)
	return v
}

// Publisher records events and whether they were published inside a transaction.
type Publisher struct {
	mu       sync.Mutex
	Events   []routes.RouteChanged
	DuringTx int
}

// Publish implements routes.RouteEventPublisher.
func (p *Publisher) Publish(ctx context.Context, ev routes.RouteChanged) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if InTx(ctx) {
		p.DuringTx++
	}
	p.Events = append(p.Events, ev)
	return nil
}

// RouteRepo is the in-memory routes.RouteRepository.
type RouteRepo struct {
	db *DB
}

func (r *RouteRepo) Get(_ context.Context, id routes.RouteID) (routes.Route, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	route, ok := r.db.state.Routes[id]
	if !ok {
		return routes.Route{}, store.ErrNotFound
	}
	return route, nil
}

func (r *RouteRepo) GetForUpdate(ctx context.Context, id routes.RouteID) (routes.Route, error) {
	return r.Get(ctx, id)
}

func (r *RouteRepo) GetSummary(_ context.Context, id routes.RouteID) (routes.SummaryRow, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	route, ok := r.db.state.Routes[id]
	if !ok {
		return routes.SummaryRow{}, store.ErrNotFound
	}
	return r.summary(route), nil
}

func (r *RouteRepo) summary(route routes.Route) routes.SummaryRow {
	row := routes.SummaryRow{Route: route, TeamName: r.db.state.Teams[route.TeamID], ObservedState: r.db.state.Observed[route.ID]}
	if route.ActiveVersionID != nil {
		v := r.db.state.Versions[*route.ActiveVersionID]
		row.Active = &routes.ActiveVersionRef{ID: v.ID, No: v.No, Config: v.Config, ConfigHash: v.ConfigHash}
	}
	return row
}

// ListSummaries filters by search, status, tag and team; ordered by updated_at desc, id desc.
func (r *RouteRepo) ListSummaries(_ context.Context, q routes.ListQuery, page routes.PageRequest) ([]routes.SummaryRow, *store.Cursor, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	rows := []routes.SummaryRow{}
	for _, route := range r.db.state.Routes {
		row := r.summary(route)
		if (Matcher{}).Matches(row, q) {
			rows = append(rows, row)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].Route.UpdatedAt.Equal(rows[j].Route.UpdatedAt) {
			return rows[i].Route.UpdatedAt.After(rows[j].Route.UpdatedAt)
		}
		return rows[i].Route.ID > rows[j].Route.ID
	})
	rows = Pager[routes.SummaryRow]{Key: func(s routes.SummaryRow) (string, string) {
		return s.Route.UpdatedAt.Format(time.RFC3339Nano), string(s.Route.ID)
	}}.After(rows, page)
	kept, next := store.PageSlicer[routes.SummaryRow]{Limit: page.Limit, KeyOf: func(s routes.SummaryRow) store.Cursor {
		return store.Cursor{SortKey: []string{s.Route.UpdatedAt.Format(time.RFC3339Nano)}, ID: string(s.Route.ID)}
	}}.Slice(rows)
	return kept, next, nil
}

func (r *RouteRepo) Create(_ context.Context, route routes.Route) (routes.Route, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	route.ID = routes.RouteID(uuid.NewString())
	route.Revision = 1
	route.CreatedAt = r.db.now()
	route.UpdatedAt = route.CreatedAt
	route.Tags = routes.NonNilTags(route.Tags)
	r.db.state.Routes[route.ID] = route
	return route, nil
}

func (r *RouteRepo) SaveDraft(_ context.Context, w routes.DraftWrite) (routes.Route, error) {
	return r.update(w.ID, &w.Expected, func(route *routes.Route) {
		c := w.Change
		if c.Name != nil {
			route.Name = *c.Name
		}
		if c.Description != nil {
			route.Description = *c.Description
		}
		if c.Tags != nil {
			route.Tags = routes.NonNilTags(*c.Tags)
		}
		if c.Replicas != nil {
			route.Replicas = *c.Replicas
		}
		if c.Draft != nil {
			route.Draft = c.Draft
		}
		route.UpdatedBy = w.UpdatedBy
	})
}

func (r *RouteRepo) SetDesiredState(_ context.Context, id routes.RouteID, s routes.DesiredState, by *uuid.UUID) (routes.Route, error) {
	return r.update(id, nil, func(route *routes.Route) { route.DesiredState = s })
}

func (r *RouteRepo) SetActiveVersion(_ context.Context, w routes.ActivationWrite) (routes.Route, error) {
	return r.update(w.ID, nil, func(route *routes.Route) {
		v := w.VersionID
		route.ActiveVersionID, route.DesiredState = &v, w.Desired
		if w.Draft != nil {
			route.Draft = w.Draft
		}
	})
}

func (r *RouteRepo) AddTag(_ context.Context, id routes.RouteID, tag string, _ *uuid.UUID) (routes.Route, error) {
	return r.update(id, nil, func(route *routes.Route) {
		for _, t := range route.Tags {
			if t == tag {
				return
			}
		}
		route.Tags = append(route.Tags, tag)
	})
}

// update applies fn, bumping the revision like the bump_row_version trigger.
func (r *RouteRepo) update(id routes.RouteID, expected *routes.Revision, fn func(*routes.Route)) (routes.Route, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	route, ok := r.db.state.Routes[id]
	if !ok {
		return routes.Route{}, store.ErrNotFound
	}
	if expected != nil && *expected != route.Revision {
		return routes.Route{}, &store.PreconditionFailedError{Current: store.RowVersion(route.Revision)}
	}
	fn(&route)
	route.Revision++
	route.UpdatedAt = r.db.now()
	r.db.state.Routes[id] = route
	return route, nil
}

func (r *RouteRepo) NamesWithPrefix(_ context.Context, team routes.TeamID, prefix string) (map[string]bool, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	out := map[string]bool{}
	for _, route := range r.db.state.Routes {
		if route.TeamID == team && strings.HasPrefix(route.Name, prefix) {
			out[route.Name] = true
		}
	}
	return out, nil
}

func (r *RouteRepo) NameTaken(_ context.Context, team routes.TeamID, name string, except routes.RouteID) (bool, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	for _, route := range r.db.state.Routes {
		if route.TeamID == team && route.Name == name && route.ID != except {
			return true, nil
		}
	}
	return false, nil
}

// FindByName returns the team's route called name.
func (r *RouteRepo) FindByName(_ context.Context, team routes.TeamID, name string) (routes.Route, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	for _, route := range r.db.state.Routes {
		if route.TeamID == team && route.Name == name {
			return route, nil
		}
	}
	return routes.Route{}, store.ErrNotFound
}

// NameTakenByOther implements validation.RouteNameIndex.
func (r *RouteRepo) NameTakenByOther(ctx context.Context, teamID, name, except string) (bool, error) {
	return r.NameTaken(ctx, routes.TeamID(teamID), name, routes.RouteID(except))
}

// Matcher applies a ListQuery in memory.
type Matcher struct{}

// Matches mirrors the SQL list filter.
func (Matcher) Matches(row routes.SummaryRow, q routes.ListQuery) bool {
	r := row.Route
	status := routes.RouteStatusResolver{}.Resolve(r.DesiredState, r.HasActiveVersion(), row.ObservedState)
	if len(q.Statuses) == 0 && status == routes.StatusArchived {
		return false
	}
	if len(q.Statuses) > 0 && !StatusSet(q.Statuses).Has(status) {
		return false
	}
	if q.Search != "" && !strings.Contains(strings.ToLower(r.Name), strings.ToLower(q.Search)) {
		return false
	}
	if q.Tag != "" && !StatusSet(nil).HasTag(r.Tags, q.Tag) {
		return false
	}
	return len(q.TeamIDs) == 0 || TeamSet(q.TeamIDs).Has(r.TeamID)
}

// TeamSet is a list of team ids.
type TeamSet []routes.TeamID

// Has reports membership.
func (s TeamSet) Has(x routes.TeamID) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}

// StatusSet is a list of statuses.
type StatusSet []routes.Status

// Has reports membership.
func (s StatusSet) Has(x routes.Status) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}

// HasTag reports whether tags contains tag.
func (StatusSet) HasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// Pager drops rows up to and including the cursor position.
type Pager[T any] struct {
	Key func(T) (string, string)
}

// After returns the rows after page.After.
func (p Pager[T]) After(rows []T, page routes.PageRequest) []T {
	if page.After == nil {
		return rows
	}
	for i, row := range rows {
		sortKey, id := p.Key(row)
		if len(page.After.SortKey) > 0 && sortKey == page.After.SortKey[0] && id == page.After.ID {
			return rows[i+1:]
		}
	}
	return rows
}

// IntKey renders numeric sort keys.
func IntKey(n int) string {
	return strconv.Itoa(n)
}
