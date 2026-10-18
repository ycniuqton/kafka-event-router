package sandbox

import (
	"context"
	"errors"
	"time"
)

// PrewarmReport tells route-runtime how much a pre-warm did before it switches versions.
type PrewarmReport struct {
	Sandboxes int
	Modules   int
	Instances int
	Duration  time.Duration
}

// add folds one sandbox report into r.
func (r PrewarmReport) add(o PrewarmReport) PrewarmReport {
	r.Sandboxes += o.Sandboxes
	r.Modules += o.Modules
	r.Instances += o.Instances
	return r
}

// PrewarmTarget decides how many idle instances a module of a scope keeps ready before traffic.
type PrewarmTarget struct {
	MaxIdle int
}

// For is the scope's parallelism, bounded by the configured idle cap (never below one).
func (t PrewarmTarget) For(scope RouteScope) int {
	return max(min(scope.Parallelism, t.MaxIdle), int(PoolWarmDefault))
}

// Prewarm grows every pool of this route version to target instances.
func (r *routeSandbox) Prewarm(ctx context.Context, target int) (PrewarmReport, error) {
	r.mu.Lock()
	modules := make([]*pooledModule, 0, len(r.modules))
	for _, m := range r.modules {
		modules = append(modules, m)
	}
	r.mu.Unlock()
	report := PrewarmReport{Sandboxes: 1, Modules: len(modules)}
	var errs []error
	for _, m := range modules {
		n, err := m.pool.Warm(ctx, target)
		report.Instances += n
		errs = append(errs, err)
	}
	if report.Instances > 0 {
		r.observePools()
	}
	return report, errors.Join(errs...)
}

// Prewarm warms every open sandbox of scope's route version (all limit sets), so the first events
// after a hot reload never wait for an instance to boot; call it before switching versions.
func (m *DefaultManager) Prewarm(ctx context.Context, scope RouteScope) (PrewarmReport, error) {
	start := time.Now()
	target := PrewarmTarget{MaxIdle: m.cfg.PoolMaxIdle}.For(scope)
	var report PrewarmReport
	var errs []error
	for _, rs := range m.sandboxesOf(scope) {
		r, err := rs.Prewarm(ctx, target)
		report = report.add(r)
		errs = append(errs, err)
	}
	report.Duration = time.Since(start)
	return report, errors.Join(errs...)
}

// sandboxesOf lists the open sandboxes of scope's route version and mode.
func (m *DefaultManager) sandboxesOf(scope RouteScope) []*routeSandbox {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*routeSandbox
	for rs := range m.open {
		if rs.scope.RouteID == scope.RouteID && rs.scope.VersionID == scope.VersionID && rs.scope.Mode == scope.Mode {
			out = append(out, rs)
		}
	}
	return out
}
