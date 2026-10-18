package scheduler

import (
	"fmt"
	"sort"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/state"
)

// WorkerLoad is a placement candidate with its planned load.
type WorkerLoad struct {
	state.Worker
	Used float64
}

// Score is the load score (used ÷ capacity).
func (w WorkerLoad) Score() float64 { return domain.LoadWeights{}.Score(w.Used, w.Capacity) }

// Fits reports whether one more replica of weight fits the worker's capacity.
func (w WorkerLoad) Fits(weight float64) bool { return w.Used+weight <= float64(w.Capacity) }

// PlacementStrategy is the base of every placement policy (selected by ROUTER_SCHEDULER_PLACEMENT).
type PlacementStrategy interface {
	Kind() domain.PlacementKind
	Place(route state.RouteDemand, need int, pool []WorkerLoad, current []state.AssignmentRow) []string
	Surplus(route state.RouteDemand, surplus int, pool []WorkerLoad, current []state.AssignmentRow) []string
	Cap(want int) int
}

// LeastLoadedStrategy picks the lowest load score, spreading replicas across zones and hosts.
type LeastLoadedStrategy struct{}

func (LeastLoadedStrategy) Kind() domain.PlacementKind { return domain.PlacementLeastLoaded }

// Place returns up to need workers not already running the route.
func (LeastLoadedStrategy) Place(route state.RouteDemand, need int, pool []WorkerLoad, current []state.AssignmentRow) []string {
	spread := NewSpread(pool, current)
	weight := domain.LoadWeights{}.Route(route.Partitions)
	candidates := []WorkerLoad{}
	for _, w := range pool {
		if !spread.Has(w.ID) && !w.Draining && w.Fits(weight) {
			candidates = append(candidates, w)
		}
	}
	picked := []string{}
	for len(picked) < need && len(candidates) > 0 {
		sort.SliceStable(candidates, func(i, j int) bool { return spread.Less(candidates[i], candidates[j]) })
		best := candidates[0]
		candidates = candidates[1:]
		picked = append(picked, best.ID)
		spread.Add(best)
	}
	return picked
}

// Cap keeps the requested replicas.
func (LeastLoadedStrategy) Cap(want int) int { return want }

// Surplus drops replicas on draining workers first, then on the most loaded ones.
func (LeastLoadedStrategy) Surplus(_ state.RouteDemand, surplus int, pool []WorkerLoad, current []state.AssignmentRow) []string {
	return SurplusOrder{}.Pick(surplus, pool, current)
}

// SingleWorkerStrategy gives every running route one lease on the first live worker (milestone 2).
type SingleWorkerStrategy struct{}

func (SingleWorkerStrategy) Kind() domain.PlacementKind { return domain.PlacementSingleWorker }

// Place assigns at most one replica.
func (SingleWorkerStrategy) Place(_ state.RouteDemand, need int, pool []WorkerLoad, current []state.AssignmentRow) []string {
	if need <= 0 {
		return nil
	}
	spread := NewSpread(pool, current)
	for _, w := range pool {
		if !w.Draining && !spread.Has(w.ID) {
			return []string{w.ID}
		}
	}
	return nil
}

// Surplus keeps one replica.
func (SingleWorkerStrategy) Surplus(_ state.RouteDemand, surplus int, pool []WorkerLoad, current []state.AssignmentRow) []string {
	return SurplusOrder{}.Pick(max(surplus, len(current)-1), pool, current)
}

// Cap keeps one replica per route.
func (SingleWorkerStrategy) Cap(want int) int { return min(want, 1) }

// PlacementManager selects the configured strategy.
type PlacementManager struct {
	active PlacementStrategy
}

// NewPlacementManager registers strategies and selects kind.
func NewPlacementManager(kind domain.PlacementKind, strategies ...PlacementStrategy) (*PlacementManager, error) {
	for _, s := range strategies {
		if s.Kind() == kind {
			return &PlacementManager{active: s}, nil
		}
	}
	return nil, fmt.Errorf("scheduler: unknown placement %q", kind)
}

// NewDefaultPlacementManager registers least_loaded and single_worker.
func NewDefaultPlacementManager(kind domain.PlacementKind) (*PlacementManager, error) {
	return NewPlacementManager(kind, LeastLoadedStrategy{}, SingleWorkerStrategy{})
}

// Strategy returns the active strategy.
func (m *PlacementManager) Strategy() PlacementStrategy { return m.active }
