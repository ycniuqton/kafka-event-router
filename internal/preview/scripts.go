package preview

import (
	"context"
	"crypto/rand"
	"fmt"
	"github.com/eventrouter/router/pkg/lookup"
	"sync"
	"time"

	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/script"
	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/enginebind"
	"github.com/eventrouter/router/pkg/sandbox/engines"
	"github.com/eventrouter/router/pkg/sandbox/host"
)

// RunScope identifies who a run executes for (hash salt team, sandbox log retention).
type RunScope struct {
	TeamID      string
	Mode        RunMode
	LookupPins  []host.LookupPin
	LookupMocks map[string]lookup.MockAnswer
}

// ScopeRoute names the pseudo route id preview sandboxes run under.
type ScopeRoute string

const PreviewRouteID ScopeRoute = "preview"

// ScriptRuntimeProvider supplies the JS runtime of a run (nil = js steps fail to compile).
type ScriptRuntimeProvider interface {
	Kind() ScriptsKind
	Runtime(scope RunScope) script.Runtime
	Close(ctx context.Context) error
}

// NoScripts disables js steps and expr filters.
type NoScripts struct{}

func (NoScripts) Kind() ScriptsKind               { return ScriptsNone }
func (NoScripts) Runtime(RunScope) script.Runtime { return nil }
func (NoScripts) Close(context.Context) error     { return nil }

// SandboxScripts runs js steps in the QuickJS sandbox with preview/test scope modes.
type SandboxScripts struct {
	Manager sandbox.Manager
}

// sandboxModes maps run modes to sandbox scope modes (both keep logs).
var sandboxModes = map[RunMode]sandbox.ScopeMode{RunPreview: sandbox.ScopePreview, RunTest: sandbox.ScopeTest}

func (SandboxScripts) Kind() ScriptsKind { return ScriptsSandbox }

// Runtime returns an engine script runtime scoped to the run's team.
func (s SandboxScripts) Runtime(scope RunScope) script.Runtime {
	return enginebind.NewRouteScripts(s.Manager, sandbox.RouteScope{
		RouteID: string(PreviewRouteID), TeamID: scope.TeamID, Parallelism: 1, Mode: sandboxModes[scope.Mode],
		LookupPins: scope.LookupPins,
	})
}

// Close shuts the sandbox manager down.
func (s SandboxScripts) Close(ctx context.Context) error { return s.Manager.Close(ctx) }

// NewSandboxScripts builds the sandbox manager with the default config and a salt provider.
func NewSandboxScripts(ctx context.Context, salts host.SaltProvider) (SandboxScripts, error) {
	return NewConfiguredSandboxScripts(ctx, sandbox.DefaultConfig(), salts)
}

// NewConfiguredSandboxScripts builds the sandbox manager with the operator config (ROUTER_SANDBOX_*).
func NewConfiguredSandboxScripts(ctx context.Context, cfg sandbox.Config, salts host.SaltProvider) (SandboxScripts, error) {
	return NewLibrarySandboxScripts(ctx, cfg, salts, nil)
}

// NewLibrarySandboxScripts also lets ctx.lookup read the library tables a run pins (nil: none).
func NewLibrarySandboxScripts(ctx context.Context, cfg sandbox.Config, salts host.SaltProvider, library host.LibraryTables) (SandboxScripts, error) {
	m, err := sandbox.NewManager(ctx, cfg, sandbox.ManagerDeps{
		Engines: engines.Registry(),
		Salts:   salts,
		Binder:  enginebind.Binder{Base: ctxhelpers.Set{}},
		Library: library,
	})
	if err != nil {
		return SandboxScripts{}, fmt.Errorf("preview: sandbox: %w", err)
	}
	return SandboxScripts{Manager: m}, nil
}

// ProcessSaltProvider generates one random salt per team for the process lifetime.
// Stand-in until preview reads team salts from the control plane (hash parity with workers).
type ProcessSaltProvider struct {
	mu    sync.Mutex
	salts map[string][]byte
}

// NewProcessSaltProvider returns an empty provider.
func NewProcessSaltProvider() *ProcessSaltProvider {
	return &ProcessSaltProvider{salts: map[string][]byte{}}
}

// Salt returns the team's process-local salt.
func (p *ProcessSaltProvider) Salt(_ context.Context, teamID string) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s, ok := p.salts[teamID]; ok {
		return s, nil
	}
	s := make([]byte, host.SaltSizeDefault)
	if _, err := rand.Read(s); err != nil {
		return nil, err
	}
	p.salts[teamID] = s
	return s, nil
}

// SampleClock is the frozen per-event clock of a run: the engine reads it once per event.
type SampleClock struct {
	mu sync.Mutex
	t  time.Time
}

// Now implements ctxhelpers.Clock.
func (c *SampleClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

// Set freezes the clock at t for the next event.
func (c *SampleClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = t
}

// ScriptRuntimeFactory builds a script runtime provider.
type ScriptRuntimeFactory func(ctx context.Context) (ScriptRuntimeProvider, error)

// ScriptRuntimeRegistry selects the configured runtime (ROUTER_PREVIEW_SCRIPTS).
type ScriptRuntimeRegistry struct {
	factories map[ScriptsKind]ScriptRuntimeFactory
	library   host.LibraryTables
}

// NewScriptRuntimeRegistry registers "none" and "sandbox" (configured by cfg, salted with salts).
func NewScriptRuntimeRegistry(salts host.SaltProvider, cfg sandbox.Config) *ScriptRuntimeRegistry {
	r := &ScriptRuntimeRegistry{factories: map[ScriptsKind]ScriptRuntimeFactory{}}
	r.Register(ScriptsNone, func(context.Context) (ScriptRuntimeProvider, error) { return NoScripts{}, nil })
	r.Register(ScriptsSandbox, func(ctx context.Context) (ScriptRuntimeProvider, error) {
		return NewLibrarySandboxScripts(ctx, cfg, salts, r.library)
	})
	return r
}

// WithLibrary lets sandbox runs read pinned library lookup tables through ctx.lookup.
func (r *ScriptRuntimeRegistry) WithLibrary(library host.LibraryTables) *ScriptRuntimeRegistry {
	r.library = library
	return r
}

// Register adds or replaces a factory.
func (r *ScriptRuntimeRegistry) Register(kind ScriptsKind, f ScriptRuntimeFactory) {
	r.factories[kind] = f
}

// Build instantiates the runtime of kind.
func (r *ScriptRuntimeRegistry) Build(ctx context.Context, kind ScriptsKind) (ScriptRuntimeProvider, error) {
	f, ok := r.factories[kind]
	if !ok {
		return nil, fmt.Errorf("preview: no script runtime registered for %q", kind)
	}
	return f(ctx)
}
