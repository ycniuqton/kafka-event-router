// Package reconcile is the worker's reconciliation loop: desired leases and target versions from the
// control plane versus the runners on this worker; it compiles versions, hot-reloads and keeps running
// on cached assignments while the control plane is unreachable.
package reconcile

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/failures"
	"github.com/eventrouter/router/internal/runtime/runner"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/script"
	"github.com/eventrouter/router/pkg/kafka"
	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/enginebind"
)

// VersionCompiler turns an assignment's version document into a runnable version.
type VersionCompiler interface {
	Compile(ctx context.Context, a domain.Assignment, doc []byte) (runner.CompiledVersion, error)
}

// CompileError is a deterministic failure of a version (bad document, compile issues): never retried.
type CompileError struct {
	Cause error
}

func (e *CompileError) Error() string { return "compile: " + e.Cause.Error() }
func (e *CompileError) Unwrap() error { return e.Cause }

// SpecSource fetches connection specs with secrets (internal endpoint).
type SpecSource interface {
	ConnectionSpec(ctx context.Context, connectionID string) (kafka.ConnectionSpec, error)
}

// SpecVault keeps fetched connection specs in memory only, so runners can restart while the control
// plane is down; a fresh fetch always wins.
type SpecVault struct {
	source SpecSource
	mu     sync.Mutex
	specs  map[string]kafka.ConnectionSpec
}

// NewSpecVault builds the vault.
func NewSpecVault(source SpecSource) *SpecVault {
	return &SpecVault{source: source, specs: map[string]kafka.ConnectionSpec{}}
}

// Get fetches the spec, falling back to the in-memory copy when the fetch fails.
func (v *SpecVault) Get(ctx context.Context, id string) (kafka.ConnectionSpec, error) {
	spec, err := v.source.ConnectionSpec(ctx, id)
	v.mu.Lock()
	defer v.mu.Unlock()
	if err == nil {
		v.specs[id] = spec
		return spec, nil
	}
	if cached, ok := v.specs[id]; ok {
		return cached, nil
	}
	return kafka.ConnectionSpec{}, fmt.Errorf("connection %s: %w", id, err)
}

// ScriptProvider supplies the JS runtime of a route version.
type ScriptProvider interface {
	Runtime(scope sandbox.RouteScope) script.Runtime
	Prewarm(ctx context.Context, scope sandbox.RouteScope) error
}

// SandboxScripts runs JS steps in the shared QuickJS sandbox manager in worker mode.
type SandboxScripts struct {
	Manager sandbox.Manager
}

// Runtime implements ScriptProvider.
func (s SandboxScripts) Runtime(scope sandbox.RouteScope) script.Runtime {
	return enginebind.NewRouteScripts(s.Manager, scope)
}

// Prewarm fills the compiled version's pools before the runner switches to it (js-sandbox JS-203).
func (s SandboxScripts) Prewarm(ctx context.Context, scope sandbox.RouteScope) error {
	_, err := s.Manager.Prewarm(ctx, scope)
	return err
}

// NoScripts makes js steps compile errors (sandbox unavailable).
type NoScripts struct{}

// Runtime implements ScriptProvider.
func (NoScripts) Runtime(sandbox.RouteScope) script.Runtime { return nil }

// Prewarm implements ScriptProvider.
func (NoScripts) Prewarm(context.Context, sandbox.RouteScope) error { return nil }

// SandboxParallelism is the starting per-step sandbox pool size on a worker; pools then autoscale inside
// the route's limits.jsMinInstances / jsMaxInstances (js-sandbox 00001-pool-autoscale).
type SandboxParallelism int

const DefaultSandboxParallelism SandboxParallelism = 4

// WorkerScope builds the sandbox scope of a route version on a worker.
type WorkerScope struct{}

// Of returns the scope of a with the route's JS pool request.
func (WorkerScope) Of(a domain.Assignment, cfg *config.RouteConfig) sandbox.RouteScope {
	pool := cfg.Limits.JSPool()
	return sandbox.RouteScope{RouteID: a.RouteID, VersionID: a.VersionID, VersionNo: a.VersionNo, TeamID: a.TeamID,
		Parallelism: int(DefaultSandboxParallelism), Mode: sandbox.ScopeWorker, Formats: enginebind.ScopeFormats{}.Of(cfg),
		LookupPins: enginebind.ScopeLookups{}.Of(cfg), Pool: &sandbox.PoolRequest{Min: pool.Min, Max: pool.Max}}
}

// EngineVersionCompiler compiles with pkg/engine and resolves every connection the version uses.
type EngineVersionCompiler struct {
	Engine  engine.Compiler
	Specs   *SpecVault
	Scripts ScriptProvider
	Salts   ctxhelpers.SaltProvider
	DLQ     failures.DLQTargetResolver
	Library config.LibraryResolver
	// Sources serves lookup steps in source mode (nil = lookups fail as permanent step errors).
	Sources ctxhelpers.SourceGateway
	Codec   engine.CodecMode
}

// Compile implements VersionCompiler.
func (c EngineVersionCompiler) Compile(ctx context.Context, a domain.Assignment, doc []byte) (runner.CompiledVersion, error) {
	parsed, err := config.Parse(doc)
	if err != nil {
		return runner.CompiledVersion{}, &CompileError{Cause: err}
	}
	cfg, err := parsed.Config()
	if err != nil {
		return runner.CompiledVersion{}, &CompileError{Cause: err}
	}
	conns, err := c.connections(ctx, cfg)
	if err != nil {
		return runner.CompiledVersion{}, err
	}
	scope := WorkerScope{}.Of(a, cfg)
	library := NewLibraryProbe(c.Library)
	opts := engine.CompileOptions{Scripts: c.Scripts.Runtime(scope), Helpers: ctxhelpers.Set{Hash: ctxhelpers.NewSaltedHasher(c.Salts, a.TeamID), Sources: c.Sources}.WithDefaults(),
		Meta: engine.RouteMeta{RouteID: a.RouteID, Name: a.RouteName, Version: a.VersionNo, TeamID: a.TeamID}, Library: library.Resolver(), Codec: c.Codec}
	p, err := c.Engine.Compile(ctx, parsed, opts)
	if err != nil {
		return runner.CompiledVersion{}, library.Classify(err)
	}
	_ = c.Scripts.Prewarm(ctx, scope)
	return runner.CompiledVersion{Ref: a.Ref(), RouteName: a.RouteName, TeamID: a.TeamID, Config: cfg, Pipeline: p, Connections: conns}, nil
}

// connections resolves source, destinations and the DLQ connection.
func (c EngineVersionCompiler) connections(ctx context.Context, cfg *config.RouteConfig) (map[string]kafka.ConnectionSpec, error) {
	ids := []string{cfg.Source.ConnectionID, c.DLQ.Resolve(cfg).ConnectionID}
	for _, b := range cfg.Branches {
		ids = append(ids, b.Destination.ConnectionID)
	}
	out := map[string]kafka.ConnectionSpec{}
	for _, id := range ids {
		if _, done := out[id]; done || id == "" {
			continue
		}
		spec, err := c.Specs.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		out[id] = spec
	}
	return out, nil
}

// CompileFailures recognises compile errors.
type CompileFailures struct{}

// Match reports whether err is deterministic for the version.
func (CompileFailures) Match(err error) bool {
	var ce *CompileError
	return errors.As(err, &ce)
}

// SaltSize is the byte size of process-local team salts.
type SaltSize int

const ProcessSaltSize SaltSize = 32

// ProcessSalts generates one random salt per team for the process lifetime; a stand-in until workers
// read the team hash salt from the control plane (js-sandbox team_hash_salt).
type ProcessSalts struct {
	mu    sync.Mutex
	salts map[string][]byte
}

// NewProcessSalts returns an empty provider.
func NewProcessSalts() *ProcessSalts {
	return &ProcessSalts{salts: map[string][]byte{}}
}

// Salt implements ctxhelpers.SaltProvider.
func (p *ProcessSalts) Salt(_ context.Context, teamID string) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s, ok := p.salts[teamID]; ok {
		return s, nil
	}
	s := make([]byte, ProcessSaltSize)
	if _, err := rand.Read(s); err != nil {
		return nil, err
	}
	p.salts[teamID] = s
	return s, nil
}
