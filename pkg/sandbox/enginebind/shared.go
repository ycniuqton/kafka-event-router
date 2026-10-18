package enginebind

import (
	"context"
	"sync"

	"github.com/eventrouter/router/pkg/engine/script"
	"github.com/eventrouter/router/pkg/sandbox"
)

// RouteScripts is the script.Runtime of one route version: every JS step and expr node of every
// branch attaches to one RouteSandbox per limit set, so identical code shares a pool. The sandbox
// closes when its last module closes.
type RouteScripts struct {
	manager   sandbox.Manager
	scope     sandbox.RouteScope
	mu        sync.Mutex
	sandboxes map[sandbox.RouteLimits]*sharedSandbox
}

// sharedSandbox counts the modules handed out from one RouteSandbox.
type sharedSandbox struct {
	rs   sandbox.RouteSandbox
	refs int
}

// NewRouteScripts returns the shared runtime of scope.
func NewRouteScripts(m sandbox.Manager, scope sandbox.RouteScope) *RouteScripts {
	return &RouteScripts{manager: m, scope: scope, sandboxes: map[sandbox.RouteLimits]*sharedSandbox{}}
}

// Compile attaches src to the route sandbox of its limits.
func (r *RouteScripts) Compile(ctx context.Context, src script.Source, lim script.Limits) (script.Module, error) {
	key := RouteLimitsOf(lim)
	r.mu.Lock()
	defer r.mu.Unlock()
	shared, err := r.sandboxFor(ctx, key)
	if err != nil {
		return nil, err
	}
	mod, err := shared.rs.Attach(ctx, sandbox.Source{StepPath: src.Origin, Entry: sandbox.EntryKind(src.Entry), Code: src.Code})
	if err != nil {
		r.dropIfUnused(ctx, key)
		return nil, ErrorMapper{}.Compile(err)
	}
	shared.refs++
	var once sync.Once
	closer := func(ctx context.Context) (err error) {
		once.Do(func() { err = r.release(ctx, key) })
		return err
	}
	return scriptModule{mod: mod, entry: src.Entry, closer: closer}, nil
}

func (r *RouteScripts) sandboxFor(ctx context.Context, key sandbox.RouteLimits) (*sharedSandbox, error) {
	if shared, ok := r.sandboxes[key]; ok {
		return shared, nil
	}
	rs, err := r.manager.Open(ctx, r.scope, key)
	if err != nil {
		return nil, ErrorMapper{}.Compile(err)
	}
	shared := &sharedSandbox{rs: rs}
	r.sandboxes[key] = shared
	return shared, nil
}

// dropIfUnused closes a sandbox that never handed out a module.
func (r *RouteScripts) dropIfUnused(ctx context.Context, key sandbox.RouteLimits) {
	if shared := r.sandboxes[key]; shared != nil && shared.refs == 0 {
		delete(r.sandboxes, key)
		_ = shared.rs.Close(ctx)
	}
}

// release drops one module reference and closes the sandbox after the last one.
func (r *RouteScripts) release(ctx context.Context, key sandbox.RouteLimits) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	shared := r.sandboxes[key]
	if shared == nil {
		return nil
	}
	shared.refs--
	if shared.refs > 0 {
		return nil
	}
	delete(r.sandboxes, key)
	return shared.rs.Close(ctx)
}

// Usage reports the footprint of this route version's sandboxes.
func (r *RouteScripts) Usage() []sandbox.RouteUsage {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]sandbox.RouteUsage, 0, len(r.sandboxes))
	for _, shared := range r.sandboxes {
		out = append(out, shared.rs.Usage())
	}
	return out
}
