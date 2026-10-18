package scheduler

import (
	"sort"

	"github.com/eventrouter/router/internal/runtime/state"
)

// Spread tracks which zones, hosts and workers already run a route's replicas.
type Spread struct {
	workers map[string]bool
	zones   map[string]int
	hosts   map[string]int
}

// NewSpread indexes the current replicas of one route.
func NewSpread(pool []WorkerLoad, current []state.AssignmentRow) *Spread {
	byID := map[string]WorkerLoad{}
	for _, w := range pool {
		byID[w.ID] = w
	}
	s := &Spread{workers: map[string]bool{}, zones: map[string]int{}, hosts: map[string]int{}}
	for _, a := range current {
		s.workers[a.WorkerID] = true
		if w, ok := byID[a.WorkerID]; ok {
			s.zones[w.Zone]++
			s.hosts[w.Host]++
		}
	}
	return s
}

// Has reports whether the worker already runs the route.
func (s *Spread) Has(workerID string) bool { return s.workers[workerID] }

// Add records a new replica.
func (s *Spread) Add(w WorkerLoad) {
	s.workers[w.ID] = true
	s.zones[w.Zone]++
	s.hosts[w.Host]++
}

// Less orders candidates: fewer replicas in the zone, then on the host, then lower score, then id.
func (s *Spread) Less(a, b WorkerLoad) bool {
	if s.zones[a.Zone] != s.zones[b.Zone] {
		return s.zones[a.Zone] < s.zones[b.Zone]
	}
	if s.hosts[a.Host] != s.hosts[b.Host] {
		return s.hosts[a.Host] < s.hosts[b.Host]
	}
	if a.Score() != b.Score() {
		return a.Score() < b.Score()
	}
	return a.ID < b.ID
}

// SurplusOrder chooses which replicas to remove.
type SurplusOrder struct{}

// Pick returns n worker ids: replicas on draining or unknown workers first, then the highest load.
func (SurplusOrder) Pick(n int, pool []WorkerLoad, current []state.AssignmentRow) []string {
	byID := map[string]WorkerLoad{}
	for _, w := range pool {
		byID[w.ID] = w
	}
	ids := make([]string, 0, len(current))
	for _, a := range current {
		ids = append(ids, a.WorkerID)
	}
	rank := func(id string) (bool, float64) {
		w, ok := byID[id]
		return !ok || w.Draining, w.Score()
	}
	sort.SliceStable(ids, func(i, j int) bool {
		di, si := rank(ids[i])
		dj, sj := rank(ids[j])
		if di != dj {
			return di
		}
		return si > sj
	})
	return ids[:min(max(n, 0), len(ids))]
}
