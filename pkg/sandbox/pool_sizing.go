package sandbox

import "runtime"

// PoolRequest is the JS pool size a worker scope asks for (limits jsMinInstances / jsMaxInstances). Max 0
// is auto: the worker's cores, never below the old fixed size (StartInstances). A nil request keeps a
// fixed pool of the scope's Parallelism.
type PoolRequest struct {
	Min int
	Max int
}

// PoolSizing resolves a scope's pool request against the operator cap and the worker's cores.
type PoolSizing struct {
	Tuning AutoscaleTuning
	Cores  int
}

// NewPoolSizing sizes pools for this process (cores = GOMAXPROCS).
func NewPoolSizing(t AutoscaleTuning) PoolSizing {
	return PoolSizing{Tuning: t, Cores: runtime.GOMAXPROCS(0)}
}

// For returns the starting capacity and the autoscaling bounds of the pools of scope. Only worker scopes
// with a request autoscale; the start is the scope's Parallelism clamped into the bounds.
func (s PoolSizing) For(scope RouteScope) (int, PoolBounds) {
	if scope.Pool == nil || scope.Mode != ScopeWorker {
		return scope.Parallelism, PoolBounds{}
	}
	maxV := scope.Pool.Max
	if maxV <= 0 {
		maxV = max(s.Cores, s.Tuning.StartInstances)
	}
	maxV = max(min(maxV, s.Tuning.MaxInstances), 1)
	minV := min(max(scope.Pool.Min, 1), maxV)
	return min(max(scope.Parallelism, minV), maxV), PoolBounds{Min: minV, Max: maxV}
}
