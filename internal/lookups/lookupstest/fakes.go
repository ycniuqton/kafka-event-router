// Package lookupstest provides in-memory fakes of the lookup-source ports for unit tests.
package lookupstest

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/lookups"
	"github.com/eventrouter/router/internal/store"
)

// MemRepo is an in-memory lookups.Repository with unique (team, name) and row versions.
type MemRepo struct {
	mu    sync.Mutex
	rows  map[uuid.UUID]lookups.Source
	Teams map[uuid.UUID]string
}

// NewMemRepo builds an empty repository.
func NewMemRepo() *MemRepo {
	return &MemRepo{rows: map[uuid.UUID]lookups.Source{}, Teams: map[uuid.UUID]string{}}
}

func (m *MemRepo) Get(_ context.Context, id uuid.UUID) (lookups.Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.rows[id]
	if !ok {
		return lookups.Source{}, store.ErrNotFound
	}
	return s, nil
}

func (m *MemRepo) List(_ context.Context, q lookups.ListQuery) (lookups.Page, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []lookups.Source
	for _, s := range m.rows {
		if (q.Kind == "" || s.Kind == q.Kind) && (q.Env == "" || s.EnvTag == q.Env) {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return lookups.Page{Items: out}, nil
}

func (m *MemRepo) Create(_ context.Context, s lookups.Source) (lookups.Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rows {
		if r.TeamID == s.TeamID && r.Name == s.Name {
			return lookups.Source{}, store.ErrConflict
		}
	}
	s.ID, s.Version, s.TeamName = uuid.New(), 1, m.Teams[s.TeamID]
	s.CreatedAt, s.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	m.rows[s.ID] = s
	return s, nil
}

func (m *MemRepo) Update(_ context.Context, s lookups.Source, expected store.RowVersion) (lookups.Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.rows[s.ID]
	if !ok {
		return lookups.Source{}, store.ErrNotFound
	}
	if cur.Version != expected {
		return lookups.Source{}, &store.PreconditionFailedError{Current: cur.Version}
	}
	s.Version, s.UpdatedAt = cur.Version+1, time.Now().UTC()
	m.rows[s.ID] = s
	return s, nil
}

func (m *MemRepo) Delete(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.rows[id]; !ok {
		return store.ErrNotFound
	}
	delete(m.rows, id)
	return nil
}

func (m *MemRepo) UpdateHealth(_ context.Context, id uuid.UUID, h lookups.Health) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.rows[id]
	if !ok {
		return store.ErrNotFound
	}
	s.Health = h
	m.rows[id] = s
	return nil
}

// MemUsage answers usage from a fixed map.
type MemUsage struct {
	Routes map[uuid.UUID][]lookups.RouteRef
}

func (u *MemUsage) RoutesUsing(_ context.Context, id uuid.UUID) ([]lookups.RouteRef, error) {
	return append([]lookups.RouteRef{}, u.Routes[id]...), nil
}

func (u *MemUsage) CountsFor(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]int, error) {
	out := map[uuid.UUID]int{}
	for _, id := range ids {
		out[id] = len(u.Routes[id])
	}
	return out, nil
}
