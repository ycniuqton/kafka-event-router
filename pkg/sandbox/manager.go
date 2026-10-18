package sandbox

import (
	"context"
	"fmt"
	"sync"

	"github.com/eventrouter/router/pkg/sandbox/boundary"
	"github.com/eventrouter/router/pkg/sandbox/determinism"
	"github.com/eventrouter/router/pkg/sandbox/host"
)

// Manager is the only entry point business logic uses for the sandbox.
type Manager interface {
	Check(ctx context.Context, src Source) []Diagnostic
	ResolveLimits(req RouteLimits) (Limits, []Diagnostic)
	Open(ctx context.Context, scope RouteScope, req RouteLimits) (RouteSandbox, error)
	Load(ctx context.Context, scope RouteScope, srcs []Source, req RouteLimits) (RouteSandbox, error)
	Prewarm(ctx context.Context, scope RouteScope) (PrewarmReport, error)
	Usage() []RouteUsage
	Programs() ProgramCacheStats
	Info() RuntimeInfo
	Close(ctx context.Context) error
}

// ManagerDeps are the ports a Manager needs; nil Observer, Marshallers and Library get defaults
// (without a Library, scopes that pin lookup tables fail to open).
// Marshaller, when set, forces one boundary for every scope instead of selecting by format.
type ManagerDeps struct {
	Engines     *EngineRegistry
	Salts       host.SaltProvider
	Binder      host.ServicesBinder
	Observer    Observer
	Marshallers *boundary.Registry
	Marshaller  boundary.Marshaller
	Library     host.LibraryTables
}

// DefaultManager compiles sources once per route version and pools instances per step.
type DefaultManager struct {
	cfg        Config
	runtime    Runtime
	programs   ProgramCache
	policy     LimitPolicy
	deps       ManagerDeps
	classifier ErrorClassifier
	info       RuntimeInfo
	mu         sync.Mutex
	open       map[*routeSandbox]struct{}
}

// NewManager builds the configured engine and the runtime-info catalogue.
func NewManager(ctx context.Context, cfg Config, deps ManagerDeps) (*DefaultManager, error) {
	deps = deps.withDefaults()
	policy := NewLimitPolicy(cfg.Maxima)
	rt, err := deps.Engines.Build(ctx, cfg.Engine, RuntimeOptions{
		ResetPolicy: cfg.ResetPolicy,
		CacheDir:    cfg.CacheDir,
		CheckLimits: policy.Defaults,
		Prelude:     determinism.DefaultPrelude(),
	})
	if err != nil {
		return nil, err
	}
	programs, err := NewProgramCacheRegistry().Build(cfg.Programs, rt)
	if err != nil {
		_ = rt.Close(ctx)
		return nil, err
	}
	m := &DefaultManager{cfg: cfg, runtime: rt, programs: programs, policy: policy, deps: deps,
		classifier: NewErrorClassifier(), open: map[*routeSandbox]struct{}{}}
	m.info = InfoBuilder{}.Build(cfg.Engine, policy)
	return m, nil
}

func (d ManagerDeps) withDefaults() ManagerDeps {
	if d.Observer == nil {
		d.Observer = NopObserver{}
	}
	if d.Marshallers == nil {
		d.Marshallers = boundary.NewRegistry()
	}
	if d.Library == nil {
		d.Library = host.NoLibraryTables{}
	}
	return d
}

// Check validates one source: size, syntax and the entry contract.
func (m *DefaultManager) Check(ctx context.Context, src Source) []Diagnostic {
	if size := int64(len(src.Code)); size > m.policy.Defaults.MaxSourceBytes {
		return []Diagnostic{{Severity: SeverityError, Path: src.StepPath,
			Message: MsgSourceTooLarge.Format(size, m.policy.Defaults.MaxSourceBytes)}}
	}
	return m.runtime.Check(ctx, src)
}

// ResolveLimits clamps route limits to operator maxima (warnings feed validation).
func (m *DefaultManager) ResolveLimits(req RouteLimits) (Limits, []Diagnostic) {
	return m.policy.Resolve(req)
}

// Info returns the runtime-info catalogue.
func (m *DefaultManager) Info() RuntimeInfo { return m.info }

// Close releases the engine.
func (m *DefaultManager) Close(ctx context.Context) error { return m.runtime.Close(ctx) }

// Open resolves limits, the team salt and the boundary of scope and returns an empty route
// sandbox; sources are added with Attach (identical steps of different branches share a pool).
func (m *DefaultManager) Open(ctx context.Context, scope RouteScope, req RouteLimits) (RouteSandbox, error) {
	limits, _ := m.policy.Resolve(req)
	registry, err := m.registryFor(ctx, scope)
	if err != nil {
		return nil, err
	}
	builder := poolBuilder{m: m, limits: limits, registry: registry, marshaller: m.marshallerFor(scope)}
	rs := newRouteSandbox(scope, builder, m.deps.Observer, m.forget)
	m.mu.Lock()
	m.open[rs] = struct{}{}
	m.mu.Unlock()
	return rs, nil
}

// Load opens a route sandbox and attaches every source (a load error fails the version).
func (m *DefaultManager) Load(ctx context.Context, scope RouteScope, srcs []Source, req RouteLimits) (RouteSandbox, error) {
	rs, err := m.Open(ctx, scope, req)
	if err != nil {
		return nil, err
	}
	for _, src := range srcs {
		if _, err := rs.Attach(ctx, src); err != nil {
			_ = rs.Close(ctx)
			return nil, fmt.Errorf("%s: %w", src.StepPath, err)
		}
	}
	return rs, nil
}

// Usage lists the footprint of every open route sandbox (worker capacity scoring).
func (m *DefaultManager) Usage() []RouteUsage {
	m.mu.Lock()
	open := make([]*routeSandbox, 0, len(m.open))
	for rs := range m.open {
		open = append(open, rs)
	}
	m.mu.Unlock()
	out := make([]RouteUsage, 0, len(open))
	for _, rs := range open {
		out = append(out, rs.Usage())
	}
	return out
}

// Programs reports the compiled-program cache (compiles saved by shared code and hot reloads).
func (m *DefaultManager) Programs() ProgramCacheStats { return m.programs.Stats() }

func (m *DefaultManager) forget(rs *routeSandbox) {
	m.mu.Lock()
	delete(m.open, rs)
	m.mu.Unlock()
}

// marshallerFor returns the forced boundary or the one selected by the scope's formats.
func (m *DefaultManager) marshallerFor(scope RouteScope) boundary.Marshaller {
	if m.deps.Marshaller != nil {
		return m.deps.Marshaller
	}
	return m.deps.Marshallers.Select(scope.Formats)
}

// registryFor binds host services to the team salt and the pinned lookup tables of scope.
func (m *DefaultManager) registryFor(ctx context.Context, scope RouteScope) (*host.Registry, error) {
	salt, err := m.deps.Salts.Salt(ctx, scope.TeamID)
	if err != nil {
		return nil, err
	}
	services, err := m.deps.Binder.Bind(ctx, scope.TeamID, salt)
	if err != nil {
		return nil, err
	}
	tables, err := host.PinnedTablesLoader{Library: m.deps.Library}.Load(ctx, scope.LookupPins)
	if err != nil {
		return nil, m.classifier.New(ErrorCompile, err.Error())
	}
	return host.NewStandardRegistry(host.PinnedServices{HostServices: services, Tables: tables}), nil
}

// poolBuilder compiles one source and warms its pool under a route version's limits.
type poolBuilder struct {
	m          *DefaultManager
	limits     Limits
	registry   *host.Registry
	marshaller boundary.Marshaller
}

func (b poolBuilder) build(ctx context.Context, scope RouteScope, src Source) (*pooledModule, error) {
	m := b.m
	if size := int64(len(src.Code)); size > b.limits.MaxSourceBytes {
		return nil, m.classifier.New(ErrorCompile, MsgSourceTooLarge.Format(size, b.limits.MaxSourceBytes))
	}
	key := ProgramKeyOf(scope, src, b.limits)
	program, err := m.programs.Acquire(ctx, key, src)
	if err != nil {
		return nil, err
	}
	pool, err := NewBoundedPool(ctx, program, b.registry, PoolOptions{
		Max:     scope.Parallelism,
		Warm:    int(PoolWarmDefault),
		MaxIdle: max(m.cfg.PoolMaxIdle, int(PoolWarmDefault)),
	})
	if err != nil {
		m.programs.Release(key)
		return nil, err
	}
	return &pooledModule{scope: scope, program: program, pool: pool, marshaller: b.marshaller,
		classifier: m.classifier, observer: m.deps.Observer, lease: programLease{cache: m.programs, key: key},
		policy: determinism.ProcessingPolicy{}}, nil
}
