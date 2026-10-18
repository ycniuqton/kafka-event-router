package sandbox_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/sandboxtest"
)

func TestCheckDiagnostics(t *testing.T) {
	m := sandboxtest.NewManager(t)
	cases := []struct {
		name  string
		entry sandbox.EntryKind
		code  string
		msg   string
		line  int
	}{
		{"ok function", sandbox.EntryTransform, "function transform(e, ctx) { return e; }", "", 0},
		{"ok const arrow", sandbox.EntryTransform, "const transform = (e) => e;", "", 0},
		{"syntax error", sandbox.EntryTransform, "function transform(e) {\n  let x = ;\n}", "SyntaxError", 2},
		{"missing transform", sandbox.EntryTransform, "function nope() {}", "transform(event, ctx) is not defined", 0},
		{"top-level throw", sandbox.EntryTransform, "\nthrow new Error('top')", "Error: top", 2},
		{"top-level infinite loop", sandbox.EntryTransform, "for(;;){}\nfunction transform(e){return e}", "time limit", 0},
		{"too large", sandbox.EntryTransform, "//" + strings.Repeat("x", 70*1024), "maximum", 0},
		{"predicate ok", sandbox.EntryPredicate, `event.value.amount > 100 && event.value.currency === "USD"`, "", 0},
		{"predicate dangling", sandbox.EntryPredicate, "event.value.x > 0 &&", "SyntaxError", 1},
		{"predicate empty", sandbox.EntryPredicate, "  ", "expression is empty", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := "/branches/0/transform/steps/1/code"
			diags := m.Check(context.Background(), sandbox.Source{StepPath: path, Entry: tc.entry, Code: tc.code})
			if tc.msg == "" {
				if len(diags) != 0 {
					t.Fatalf("unexpected diagnostics %+v", diags)
				}
				return
			}
			if len(diags) != 1 || !strings.Contains(diags[0].Message, tc.msg) || diags[0].Path != path || diags[0].Line != tc.line {
				t.Fatalf("diagnostics = %+v, want %q at line %d", diags, tc.msg, tc.line)
			}
		})
	}
}

func TestLoadFailsOnBadSource(t *testing.T) {
	m := sandboxtest.NewManager(t)
	srcs := []sandbox.Source{
		{StepPath: "/ok", Entry: sandbox.EntryTransform, Code: "function transform(e){return e}"},
		{StepPath: "/bad", Entry: sandbox.EntryTransform, Code: "function transform(e){"},
	}
	_, err := m.Load(context.Background(), sandboxtest.Scope(1), srcs, sandbox.RouteLimits{})
	se, ok := sandbox.AsError(err)
	if !ok || se.Kind != sandbox.ErrorCompile || !strings.HasPrefix(err.Error(), "/bad") {
		t.Fatalf("err = %v", err)
	}
}

func TestWorkerModeDropsLogs(t *testing.T) {
	m := sandboxtest.NewManager(t)
	scope := sandboxtest.Scope(1)
	scope.Mode = sandbox.ScopeWorker
	rs, err := m.Load(context.Background(), scope, []sandbox.Source{{StepPath: "/s", Entry: sandbox.EntryTransform,
		Code: "function transform(e, ctx){ ctx.log('secret payload'); console.log('x'); return e }"}}, sandbox.RouteLimits{})
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close(context.Background())
	mod, _ := rs.Module("/s")
	res, err := mod.Call(context.Background(), sandbox.Invocation{Event: typicalEvent(), Now: time.Unix(1, 0)})
	if err != nil || len(res.Logs) != 0 {
		t.Fatalf("worker logs = %v, %v", res.Logs, err)
	}
}

func TestPreviewLogsAreCapped(t *testing.T) {
	m := sandboxtest.NewManager(t)
	mod, _ := sandboxtest.LoadOne(t, m, sandbox.EntryTransform,
		"function transform(e, ctx){ for (let i = 0; i < 200; i++) ctx.log('x'.repeat(200) + i); return e }")
	res, err := mod.Call(context.Background(), sandbox.Invocation{Event: typicalEvent(), Now: time.Unix(1, 0)})
	total := 0
	for _, l := range res.Logs {
		total += len(l)
	}
	if err != nil || !res.LogsTruncated || int64(total) > sandbox.LimitDefault.MaxLogBytes {
		t.Fatalf("logs=%d bytes truncated=%v err=%v", total, res.LogsTruncated, err)
	}
}

func TestRouteSandboxCloseAndStats(t *testing.T) {
	m := sandboxtest.NewManager(t)
	mod, rs := sandboxtest.LoadOne(t, m, sandbox.EntryTransform, "function transform(e){return e}")
	st := rs.Stats()
	if len(st) != 1 || st[0].Size != 1 || st[0].Idle != 1 || st[0].MemoryBytes <= 0 {
		t.Fatalf("stats = %+v", st)
	}
	_ = rs.Close(context.Background())
	_, err := mod.Call(context.Background(), sandbox.Invocation{Event: typicalEvent(), Now: time.Unix(1, 0)})
	if se, ok := sandbox.AsError(err); !ok || se.Kind != sandbox.ErrorInternal {
		t.Fatalf("call after close = %v", err)
	}
	if _, ok := rs.Module("/missing"); ok {
		t.Fatal("unknown step must not resolve")
	}
}

func TestResolveLimitsWarnsOnClamp(t *testing.T) {
	m := sandboxtest.NewManager(t)
	lim, diags := m.ResolveLimits(sandbox.RouteLimits{JSTimeoutMs: 10_000})
	if lim.Timeout != sandbox.LimitMaximum.Timeout || len(diags) != 1 || diags[0].Path != string(sandbox.PathJSTimeoutMs) {
		t.Fatalf("limits=%+v diags=%+v", lim, diags)
	}
	if m.Info().Engine != sandbox.EngineQuickJSWazero {
		t.Fatal("info engine")
	}
}
