// Package connectionstest provides in-memory fakes of the connections ports for unit and HTTP tests.
package connectionstest

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/secrets"
	"github.com/eventrouter/router/internal/store"
)

// MemRepo is an in-memory connections.Repository.
type MemRepo struct {
	mu    sync.Mutex
	rows  map[uuid.UUID]connections.Connection
	Teams map[uuid.UUID]string
}

// NewMemRepo returns an empty repository.
func NewMemRepo() *MemRepo {
	return &MemRepo{rows: map[uuid.UUID]connections.Connection{}, Teams: map[uuid.UUID]string{}}
}

// Get implements connections.Repository.
func (m *MemRepo) Get(_ context.Context, id uuid.UUID) (connections.Connection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.rows[id]
	if !ok {
		return c, store.ErrNotFound
	}
	return c, nil
}

// List implements connections.Repository (name order, no keyset beyond limit).
func (m *MemRepo) List(_ context.Context, q connections.ListQuery) (connections.Page, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []connections.Connection
	for _, c := range m.rows {
		if (q.Env == "" || c.EnvTag == q.Env) && (q.Status == "" || c.Health.Status == q.Status) {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return connections.Page{Items: out}, nil
}

// Create implements connections.Repository; duplicate (team, name) is a conflict at /name.
func (m *MemRepo) Create(_ context.Context, c connections.Connection) (connections.Connection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rows {
		if r.TeamID == c.TeamID && r.Name == c.Name {
			return c, store.ErrConflict
		}
	}
	c.ID, c.Version, c.TeamName = uuid.New(), 1, m.Teams[c.TeamID]
	c.CreatedAt, c.UpdatedAt = time.Now(), time.Now()
	m.rows[c.ID] = c
	return c, nil
}

// Update implements connections.Repository with version checking.
func (m *MemRepo) Update(_ context.Context, c connections.Connection, expected store.RowVersion) (connections.Connection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.rows[c.ID]
	if !ok {
		return c, store.ErrNotFound
	}
	if cur.Version != expected {
		return c, &store.PreconditionFailedError{Current: cur.Version}
	}
	c.Version = cur.Version + 1
	c.UpdatedAt = time.Now()
	m.rows[c.ID] = c
	return c, nil
}

// Delete implements connections.Repository.
func (m *MemRepo) Delete(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.rows[id]; !ok {
		return store.ErrNotFound
	}
	delete(m.rows, id)
	return nil
}

// UpdateHealth implements connections.Repository (no version bump).
func (m *MemRepo) UpdateHealth(_ context.Context, id uuid.UUID, h connections.Health) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.rows[id]
	if !ok {
		return store.ErrNotFound
	}
	c.Health = h
	m.rows[id] = c
	return nil
}

// ListForProbe implements connections.Repository.
func (m *MemRepo) ListForProbe(_ context.Context, before time.Time, limit int) ([]connections.Connection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []connections.Connection
	for _, c := range m.rows {
		due := c.Health.CheckedAt == nil || c.Health.CheckedAt.Before(before)
		if c.ProbeEnabled && due && len(out) < limit {
			out = append(out, c)
		}
	}
	return out, nil
}

// MemUsage is an in-memory connections.UsageResolver.
type MemUsage struct {
	Routes map[uuid.UUID][]connections.RouteRef
}

// RoutesUsing implements connections.UsageResolver.
func (u *MemUsage) RoutesUsing(_ context.Context, id uuid.UUID) ([]connections.RouteRef, error) {
	return append([]connections.RouteRef{}, u.Routes[id]...), nil
}

// CountsFor implements connections.UsageResolver.
func (u *MemUsage) CountsFor(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]int, error) {
	out := map[uuid.UUID]int{}
	for _, id := range ids {
		out[id] = len(u.Routes[id])
	}
	return out, nil
}

// MemSecrets is an in-memory secrets.Manager.
type MemSecrets struct {
	mu   sync.Mutex
	Data map[secrets.Ref][]byte
}

// NewMemSecrets returns an empty store.
func NewMemSecrets() *MemSecrets { return &MemSecrets{Data: map[secrets.Ref][]byte{}} }

// Put implements secrets.Manager.
func (s *MemSecrets) Put(_ context.Context, plain []byte) (secrets.Ref, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ref := secrets.NewRef()
	s.Data[ref] = append([]byte{}, plain...)
	return ref, nil
}

// Get implements secrets.Manager.
func (s *MemSecrets) Get(_ context.Context, ref secrets.Ref) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.Data[ref]
	if !ok {
		return nil, secrets.ErrNotFound
	}
	return append([]byte{}, v...), nil
}

// Replace implements secrets.Manager.
func (s *MemSecrets) Replace(_ context.Context, ref secrets.Ref, plain []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Data[ref] = append([]byte{}, plain...)
	return nil
}

// Delete implements secrets.Manager.
func (s *MemSecrets) Delete(_ context.Context, ref secrets.Ref) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.Data, ref)
	return nil
}

// DirectTx runs units of work without a transaction.
type DirectTx struct{}

// WithinTx implements store.TxManager.
func (DirectTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// MemRates is a RateEstimator returning fixed rates.
type MemRates struct {
	Fixed map[string]float64
}

// Rates implements connections.RateEstimator.
func (r MemRates) Rates(context.Context, uuid.UUID, map[string]int64) (map[string]*float64, error) {
	out := map[string]*float64{}
	for k, v := range r.Fixed {
		val := v
		out[k] = &val
	}
	return out, nil
}
