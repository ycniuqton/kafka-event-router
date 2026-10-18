package reconcile

import (
	"context"

	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/runner"
)

// ensure brings one assignment to its target: start, upgrade, or nothing.
func (r *Reconciler) ensure(ctx context.Context, a domain.Assignment) {
	cur, ok := r.d.Runners.Current(a.RouteID)
	switch {
	case !ok:
		r.start(ctx, a)
	case cur.Ref.VersionID == a.VersionID:
		return
	case cur.Failed:
		r.d.Runners.Remove(ctx, a.RouteID, domain.StopRestart)
		r.start(ctx, a)
	default:
		r.upgrade(ctx, cur, a)
	}
}

// start compiles and starts a missing runner, unless the worker is over capacity.
func (r *Reconciler) start(ctx context.Context, a domain.Assignment) {
	if r.refuse(a) {
		return
	}
	v, err := r.compile(ctx, a)
	if err != nil {
		r.compileFailed(a, err, r.d.Runners.RecordFailure)
		return
	}
	if err := r.d.Runners.Start(ctx, v); err != nil {
		r.d.Log.Warn("runtime: runner start failed", string(logging.AttrRouteID), a.RouteID, string(logging.AttrError), err)
	}
}

// upgrade compiles the target in the background of the running version and swaps or restarts.
func (r *Reconciler) upgrade(ctx context.Context, cur runner.Current, a domain.Assignment) {
	next, err := r.compile(ctx, a)
	if err != nil {
		r.compileFailed(a, err, r.d.Runners.RecordUpgradeFailure)
		return
	}
	kind := r.d.Reload.Classify(cur.Version.Config, next.Config)
	r.d.Metrics.Reload(kind)
	if kind == domain.ReloadFullRestart {
		err = r.d.Runners.Restart(ctx, next)
	} else {
		err = r.d.Runners.Swap(ctx, next)
	}
	if err != nil {
		r.d.Log.Warn("runtime: reload failed", string(logging.AttrRouteID), a.RouteID, string(logging.AttrError), err)
	}
}

// compileFailed records deterministic failures; transient ones (secrets, config fetch) retry next pass.
func (r *Reconciler) compileFailed(a domain.Assignment, err error, record func(domain.VersionRef, domain.ErrorCategory, string)) {
	if (CompileFailures{}).Match(err) {
		record(a.Ref(), domain.CategoryCompile, err.Error())
		return
	}
	r.d.Log.Warn("runtime: version not ready", string(logging.AttrRouteID), a.RouteID, string(logging.AttrError), err)
}

// compile loads the version document (cache first) and compiles it.
func (r *Reconciler) compile(ctx context.Context, a domain.Assignment) (runner.CompiledVersion, error) {
	doc, err := r.d.Cache.LoadVersion(a.Ref())
	if err != nil {
		if doc, err = r.d.ControlPlane.VersionConfig(ctx, a.VersionID); err != nil {
			return runner.CompiledVersion{}, err
		}
		if err := r.d.Cache.SaveVersion(a.Ref(), doc); err != nil {
			r.d.Log.Warn("runtime: version cache write failed", string(logging.AttrError), err)
		}
	}
	return r.d.Compiler.Compile(ctx, a, doc)
}

// refuse marks a new route refused when one more replica would exceed this worker's capacity.
func (r *Reconciler) refuse(a domain.Assignment) bool {
	over := r.d.Runners.CapacityUsed()+domain.LoadWeights{}.Route(0) > float64(r.d.Identity.Capacity)
	r.state.Lock()
	defer r.state.Unlock()
	if over {
		r.refused[a.RouteID] = true
	} else {
		delete(r.refused, a.RouteID)
	}
	return over
}
