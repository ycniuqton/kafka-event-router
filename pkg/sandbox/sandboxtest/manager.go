package sandboxtest

import (
	"context"
	"testing"

	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/engines"
)

// NewManager builds a production-configured Manager with reference services.
func NewManager(t testing.TB, mutate ...func(*sandbox.Config)) *sandbox.DefaultManager {
	t.Helper()
	cfg := sandbox.DefaultConfig()
	for _, m := range mutate {
		m(&cfg)
	}
	m, err := sandbox.NewManager(context.Background(), cfg, sandbox.ManagerDeps{
		Engines: engines.Registry(),
		Salts:   Salts{},
		Binder:  Binder{Tables: map[string]map[string]any{"countries": {"DE": "Germany", "FR": "France"}}},
	})
	if err != nil {
		t.Fatalf("sandbox manager: %v", err)
	}
	t.Cleanup(func() { _ = m.Close(context.Background()) })
	return m
}

// Scope is a preview scope for route "r1" version "v1" of team "t1".
func Scope(parallelism int) sandbox.RouteScope {
	return sandbox.RouteScope{RouteID: "r1", VersionID: "v1", TeamID: "t1", Parallelism: parallelism, Mode: sandbox.ScopePreview}
}

// LoadOne loads a single source at path "/s" and returns its module.
func LoadOne(t testing.TB, m sandbox.Manager, entry sandbox.EntryKind, code string) (sandbox.Module, sandbox.RouteSandbox) {
	t.Helper()
	rs, err := m.Load(context.Background(), Scope(2), []sandbox.Source{{StepPath: "/s", Entry: entry, Code: code}}, sandbox.RouteLimits{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	t.Cleanup(func() { _ = rs.Close(context.Background()) })
	mod, _ := rs.Module("/s")
	return mod, rs
}
