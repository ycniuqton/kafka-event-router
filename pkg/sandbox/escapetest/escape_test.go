package escapetest_test

import (
	"context"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/escapetest"
	"github.com/eventrouter/router/pkg/sandbox/sandboxtest"
)

func TestEscapeSuite(t *testing.T) {
	m := sandboxtest.NewManager(t)
	runner := escapetest.Runner{Manager: m, Scope: sandboxtest.Scope(1)}
	for _, o := range runner.Run(context.Background(), escapetest.Cases()) {
		if o.Err != nil {
			t.Errorf("%s: %v", o.Case.Name, o.Err)
		}
	}
}

func TestCrossRouteStateProbe(t *testing.T) {
	m := sandboxtest.NewManager(t)
	ctx := context.Background()
	load := func(route, code string) sandbox.Module {
		scope := sandboxtest.Scope(1)
		scope.RouteID = route
		rs, err := m.Load(ctx, scope, []sandbox.Source{{StepPath: "/s", Entry: sandbox.EntryTransform, Code: code}}, sandbox.RouteLimits{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = rs.Close(ctx) })
		mod, _ := rs.Module("/s")
		return mod
	}
	writer := load("route-a", "globalThis.secret = 'a-secret';\nfunction transform(e) { globalThis.other = 1; Object.prototype.mark = 'a'; return { value: secret }; }")
	reader := load("route-b", "function transform(e) { return { value: [typeof secret, typeof other, ({}).mark === undefined] }; }")
	inv := escapetest.Event{}.At(1)
	if res, err := writer.Call(ctx, inv); err != nil || res.Outputs[0].Value != "a-secret" {
		t.Fatalf("writer: %+v %v", res, err)
	}
	res, err := reader.Call(ctx, inv)
	want := escapetest.Returns{Value: []any{"undefined", "undefined", true}}
	if verr := want.Verify(res, err); verr != nil {
		t.Fatal(verr)
	}
}

func TestTimeoutIsEnforcedPromptly(t *testing.T) {
	m := sandboxtest.NewManager(t)
	mod, _ := sandboxtest.LoadOne(t, m, sandbox.EntryTransform, "function transform(){ for(;;){} }")
	start := time.Now()
	_, err := mod.Call(context.Background(), escapetest.Event{}.At(1))
	if se, ok := sandbox.AsError(err); !ok || se.Kind != sandbox.ErrorTimeout || !se.Retryable {
		t.Fatalf("err = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 10*sandbox.LimitDefault.Timeout {
		t.Fatalf("timeout took %v", elapsed)
	}
}

func TestLookupEscapeSuite(t *testing.T) {
	m := sandboxtest.NewManager(t)
	runner := escapetest.Runner{Manager: m, Scope: sandboxtest.Scope(1)}
	for _, o := range runner.Run(context.Background(), escapetest.LookupCases()) {
		if o.Err != nil {
			t.Errorf("%s: %v", o.Case.Name, o.Err)
		}
	}
}

func TestLibrarySourcedJSIsolation(t *testing.T) {
	m := sandboxtest.NewManager(t)
	scope := func(route, version string) sandbox.RouteScope {
		s := sandboxtest.Scope(2)
		s.RouteID, s.VersionID = route, version
		return s
	}
	branches := []string{"/branches/0/transform/steps/0/code", "/branches/1/transform/steps/2/code"}
	scenarios := []escapetest.LibraryScenario{
		{Name: "two branches of one version", Scopes: []sandbox.RouteScope{scope("lib-a", "v1")}, Paths: branches},
		{Name: "two versions of one route", Scopes: []sandbox.RouteScope{scope("lib-a", "v1"), scope("lib-a", "v2")}, Paths: branches[:1]},
		{Name: "two routes", Scopes: []sandbox.RouteScope{scope("lib-a", "v1"), scope("lib-b", "v1")}, Paths: branches},
	}
	probe := escapetest.LibraryProbe{Manager: m}
	for _, s := range scenarios {
		if err := probe.Run(context.Background(), s); err != nil {
			t.Errorf("%s: %v", s.Name, err)
		}
	}
	if st := m.Programs(); st.Hits == 0 {
		t.Fatalf("library code must be served from the program cache within a route: %+v", st)
	}
}

// TestHostCallDeadlineRace repeats deadline hits during host calls; a host access to freed guest
// memory would crash the test process.
func TestHostCallDeadlineRace(t *testing.T) {
	m := sandboxtest.NewManager(t)
	rs, err := m.Load(context.Background(), sandboxtest.Scope(1), []sandbox.Source{{StepPath: "/s", Entry: sandbox.EntryTransform, Code: "function transform(e, ctx) {\n" + escapetest.HostRaceAttack + "\n}"}}, sandbox.RouteLimits{JSTimeoutMs: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close(context.Background())
	mod, _ := rs.Module("/s")
	for i := range 400 {
		if _, err := mod.Call(context.Background(), escapetest.Event{}.At(int64(i))); err == nil {
			t.Fatal("want timeout")
		}
	}
}
