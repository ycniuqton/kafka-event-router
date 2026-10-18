package sandboxtest

import (
	"context"
	"testing"

	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/script"
	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/enginebind"
	"github.com/eventrouter/router/pkg/sandbox/engines"
)

// WorkerScriptRuntime is the engine script.Runtime a worker builds: real QuickJS sandbox,
// production ctx helpers and a worker-mode scope; it is closed with the test.
type WorkerScriptRuntime struct {
	Parallelism int
}

// Runtime starts the sandbox manager and returns the bound runtime.
func (w WorkerScriptRuntime) Runtime(t testing.TB) script.Runtime {
	t.Helper()
	m, err := sandbox.NewManager(context.Background(), sandbox.DefaultConfig(), sandbox.ManagerDeps{
		Engines: engines.Registry(),
		Salts:   Salts{},
		Binder:  enginebind.Binder{Base: ctxhelpers.Set{}},
	})
	if err != nil {
		t.Fatalf("sandbox manager: %v", err)
	}
	t.Cleanup(func() { _ = m.Close(context.Background()) })
	scope := sandbox.RouteScope{RouteID: "r1", VersionID: "v1", TeamID: "t1", Parallelism: w.Parallelism, Mode: sandbox.ScopeWorker}
	return enginebind.ScriptRuntime{Manager: m, Scope: scope}
}
