package testcases

import (
	"context"
	"fmt"
	"sync"

	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/script"
	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/enginebind"
	"github.com/eventrouter/router/pkg/sandbox/engines"
	"github.com/eventrouter/router/pkg/sandbox/host"
)

// ScopeTag names the sandbox scope of test runs.
type ScopeTag string

const (
	ScopeVersionTestRun ScopeTag = "test-run"
	scopeParallelism             = 1
)

// SandboxScriptRuntimes runs js steps of test cases in the QuickJS sandbox (test scope).
type SandboxScriptRuntimes struct {
	Manager sandbox.Manager
}

// NewSandboxScriptRuntimes starts a sandbox manager with the given team salts.
func NewSandboxScriptRuntimes(ctx context.Context, salts host.SaltProvider) (*SandboxScriptRuntimes, error) {
	m, err := sandbox.NewManager(ctx, sandbox.DefaultConfig(), sandbox.ManagerDeps{
		Engines: engines.Registry(), Salts: salts, Binder: enginebind.Binder{Base: ctxhelpers.Set{}},
	})
	if err != nil {
		return nil, fmt.Errorf("testcases: sandbox: %w", err)
	}
	return &SandboxScriptRuntimes{Manager: m}, nil
}

// Runtime implements ScriptRuntimes.
func (s *SandboxScriptRuntimes) Runtime(target RunTarget) script.Runtime {
	return enginebind.ScriptRuntime{Manager: s.Manager, Scope: sandbox.RouteScope{
		RouteID: target.RouteID, VersionID: string(ScopeVersionTestRun), TeamID: target.TeamID,
		Parallelism: scopeParallelism, Mode: sandbox.ScopeTest,
	}}
}

// LazySandboxScripts starts the sandbox on the first run with a js step source; a start failure
// leaves js steps without a runtime (they then fail to compile with a clear message).
type LazySandboxScripts struct {
	Salts host.SaltProvider
	once  sync.Once
	rt    *SandboxScriptRuntimes
}

// Runtime implements ScriptRuntimes.
func (l *LazySandboxScripts) Runtime(target RunTarget) script.Runtime {
	l.once.Do(func() {
		rt, err := NewSandboxScriptRuntimes(context.Background(), l.Salts)
		if err == nil {
			l.rt = rt
		}
	})
	if l.rt == nil {
		return nil
	}
	return l.rt.Runtime(target)
}
