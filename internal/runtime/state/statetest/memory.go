// Package statetest is an in-memory implementation of the route-runtime state ports for tests.
package statetest

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/state"
	"github.com/eventrouter/router/internal/store"
)

// Memory holds every table the ports touch.
type Memory struct {
	mu          sync.Mutex
	Workers     map[string]state.Worker
	Leases      map[[2]string]state.AssignmentRow
	Demands     map[string]state.RouteDemand
	Configs     map[string][]byte
	VersionNos  map[string]int
	Runtime     map[string]state.RuntimeRow
	Deployments map[string]*Deployment
	RouteNames  map[string]string
	Replays     map[string]state.Replay
}

// Deployment is a deployments row.
type Deployment struct {
	state.Rollout
	Status string
}

// DeploymentStatus names the statuses the fake tracks.
type DeploymentStatus string

const (
	StatusRolling    DeploymentStatus = "rolling_out"
	StatusActive     DeploymentStatus = "active"
	StatusSuperseded DeploymentStatus = "superseded"
	StatusFailed     DeploymentStatus = "failed"
)

// New returns empty tables.
func New() *Memory {
	return &Memory{Workers: map[string]state.Worker{}, Leases: map[[2]string]state.AssignmentRow{}, Demands: map[string]state.RouteDemand{},
		Configs: map[string][]byte{}, VersionNos: map[string]int{}, Runtime: map[string]state.RuntimeRow{}, Deployments: map[string]*Deployment{},
		RouteNames: map[string]string{}, Replays: map[string]state.Replay{}}
}

// Stores exposes the memory as ports.
func (m *Memory) Stores() state.Stores {
	return state.Stores{Workers: workers{m}, Assignments: assignments{m}, Routes: routes{m}, Runtime: runtime{m}, Rollouts: rollouts{m}, Replays: replays{m}, Tx: tx{}}
}

// Lease returns one lease row.
func (m *Memory) Lease(routeID, workerID string) (state.AssignmentRow, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.Leases[[2]string{routeID, workerID}]
	return a, ok
}

// SetLease writes one lease row.
func (m *Memory) SetLease(a state.AssignmentRow) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Leases[[2]string{a.RouteID, a.WorkerID}] = a
}

type tx struct{}

func (tx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error { return fn(ctx) }

type workers struct{ m *Memory }

func (w workers) UpsertHeartbeat(_ context.Context, id string, hb domain.Heartbeat, now time.Time) error {
	w.m.mu.Lock()
	defer w.m.mu.Unlock()
	cur, ok := w.m.Workers[id]
	if !ok {
		cur.StartedAt = now
	}
	cur.ID, cur.Host, cur.Zone, cur.Version, cur.Capacity = id, hb.Host, hb.Zone, hb.Version, hb.Capacity
	cur.CapacityUsed, cur.Draining, cur.LastHeartbeatAt = hb.CapacityUsed, hb.Draining, now
	w.m.Workers[id] = cur
	return nil
}

func (w workers) ExtendLeases(_ context.Context, workerID string, until time.Time) error {
	w.m.mu.Lock()
	defer w.m.mu.Unlock()
	for k, a := range w.m.Leases {
		if a.WorkerID == workerID {
			a.LeaseExpiresAt = until
			w.m.Leases[k] = a
		}
	}
	return nil
}

func (w workers) ListWorkers(context.Context) ([]state.Worker, error) {
	w.m.mu.Lock()
	defer w.m.mu.Unlock()
	out := []state.Worker{}
	for _, wk := range w.m.Workers {
		out = append(out, wk)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (w workers) PageWorkers(ctx context.Context, limit int, after *store.Cursor) ([]state.Worker, *store.Cursor, error) {
	all, _ := w.ListWorkers(ctx)
	out := []state.Worker{}
	for _, wk := range all {
		if after == nil || wk.ID > after.ID {
			out = append(out, wk)
		}
	}
	if len(out) <= limit {
		return out, nil, nil
	}
	out = out[:limit]
	return out, &store.Cursor{SortKey: []string{out[limit-1].ID}, ID: out[limit-1].ID}, nil
}
