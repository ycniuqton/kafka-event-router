package scheduler

import (
	"context"
	"time"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/state"
)

// RuntimeRefresher recomputes a route's route_runtime row (controlapi aggregator).
type RuntimeRefresher interface {
	Refresh(ctx context.Context, routeID string) error
}

// AssignmentPlanner computes and applies the desired leases of every route (design 5.8).
type AssignmentPlanner struct {
	Stores    state.Stores
	Placement *PlacementManager
	Runtime   RuntimeRefresher
	Timings   domain.Timings
	Now       func() time.Time
}

// Plan runs one planning pass over all routes.
func (p *AssignmentPlanner) Plan(ctx context.Context) error {
	demands, err := p.Stores.Routes.Demands(ctx)
	if err != nil {
		return err
	}
	workers, err := p.Stores.Workers.ListWorkers(ctx)
	if err != nil {
		return err
	}
	rows, err := p.Stores.Assignments.List(ctx)
	if err != nil {
		return err
	}
	now := p.Now()
	pool := NewWorkerPool(workers, now, p.Timings.LeaseTTL)
	pool.Load(rows, demands)
	byRoute := Assignments(rows).ByRoute()
	for _, d := range demands {
		if err := p.planRoute(ctx, d, byRoute[d.RouteID], pool, now); err != nil {
			return err
		}
	}
	return nil
}

// planRoute lifts halts, removes leases of unschedulable routes and fixes the replica count.
func (p *AssignmentPlanner) planRoute(ctx context.Context, d state.RouteDemand, cur []state.AssignmentRow, pool *WorkerPool, now time.Time) error {
	if d.HaltLifted() {
		if err := p.Stores.Runtime.ClearHalt(ctx, d.RouteID); err != nil {
			return err
		}
	}
	if !d.Schedulable() {
		return p.clear(ctx, d, cur)
	}
	strategy := p.Placement.Strategy()
	want := p.want(d)
	picks := strategy.Place(d, want-pool.Healthy(cur), pool.Candidates(), cur)
	for _, workerID := range picks {
		row := state.AssignmentRow{RouteID: d.RouteID, WorkerID: workerID, LeaseExpiresAt: now.Add(p.Timings.LeaseTTL), TargetVersionID: d.ActiveVersionID}
		if err := p.Stores.Assignments.Insert(ctx, row); err != nil {
			return err
		}
		pool.Assign(workerID, domain.LoadWeights{}.Route(d.Partitions))
	}
	for _, workerID := range strategy.Surplus(d, len(cur)+len(picks)-want, pool.Candidates(), cur) {
		if err := p.Stores.Assignments.Delete(ctx, d.RouteID, workerID); err != nil {
			return err
		}
	}
	return nil
}

// clear removes every lease of a paused, archived or halted route and refreshes its observed state.
func (p *AssignmentPlanner) clear(ctx context.Context, d state.RouteDemand, cur []state.AssignmentRow) error {
	if len(cur) == 0 {
		return nil
	}
	for _, a := range cur {
		if err := p.Stores.Assignments.Delete(ctx, a.RouteID, a.WorkerID); err != nil {
			return err
		}
	}
	return p.Runtime.Refresh(ctx, d.RouteID)
}

// want is replicas (default 2), capped at the source partition count when known and by the strategy.
func (p *AssignmentPlanner) want(d state.RouteDemand) int {
	want := d.Replicas
	if want <= 0 {
		want = int(domain.DefaultReplicas)
	}
	if d.Partitions > 0 {
		want = min(want, d.Partitions)
	}
	return p.Placement.Strategy().Cap(max(want, 1))
}

// Assignments is a list of lease rows.
type Assignments []state.AssignmentRow

// ByRoute groups rows by route.
func (as Assignments) ByRoute() map[string][]state.AssignmentRow {
	out := map[string][]state.AssignmentRow{}
	for _, a := range as {
		out[a.RouteID] = append(out[a.RouteID], a)
	}
	return out
}

// WorkerPool is the planning view of live workers and their planned loads.
type WorkerPool struct {
	order []string
	loads map[string]*WorkerLoad
}

// NewWorkerPool keeps workers that heartbeated within ttl.
func NewWorkerPool(workers []state.Worker, now time.Time, ttl time.Duration) *WorkerPool {
	p := &WorkerPool{loads: map[string]*WorkerLoad{}}
	for _, w := range workers {
		if w.Alive(now, ttl) {
			p.order = append(p.order, w.ID)
			p.loads[w.ID] = &WorkerLoad{Worker: w}
		}
	}
	return p
}

// Load adds the weight of every existing lease to its worker.
func (p *WorkerPool) Load(rows []state.AssignmentRow, demands []state.RouteDemand) {
	partitions := map[string]int{}
	for _, d := range demands {
		partitions[d.RouteID] = d.Partitions
	}
	for _, a := range rows {
		p.Assign(a.WorkerID, domain.LoadWeights{}.Route(partitions[a.RouteID]))
	}
}

// Assign adds weight to a live worker.
func (p *WorkerPool) Assign(workerID string, weight float64) {
	if w, ok := p.loads[workerID]; ok {
		w.Used += weight
	}
}

// Healthy counts leases on live, non-draining workers.
func (p *WorkerPool) Healthy(cur []state.AssignmentRow) int {
	n := 0
	for _, a := range cur {
		if w, ok := p.loads[a.WorkerID]; ok && !w.Draining {
			n++
		}
	}
	return n
}

// Candidates returns the live workers in stable order.
func (p *WorkerPool) Candidates() []WorkerLoad {
	out := make([]WorkerLoad, 0, len(p.order))
	for _, id := range p.order {
		out = append(out, *p.loads[id])
	}
	return out
}
