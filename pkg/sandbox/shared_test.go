package sandbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/boundary"
	"github.com/eventrouter/router/pkg/sandbox/determinism"
	"github.com/eventrouter/router/pkg/sandbox/sandboxtest"
)

const echo = "function transform(e, ctx) { return { value: { r: Math.random(), now: ctx.now(), v: e.value } }; }"

func TestBranchesShareOnePoolPerCode(t *testing.T) {
	m := sandboxtest.NewManager(t)
	ctx := context.Background()
	srcs := []sandbox.Source{
		{StepPath: "/branches/0/transform/steps/0/code", Entry: sandbox.EntryTransform, Code: echo},
		{StepPath: "/branches/1/transform/steps/0/code", Entry: sandbox.EntryTransform, Code: echo},
		{StepPath: "/branches/1/filter/expr", Entry: sandbox.EntryPredicate, Code: "true"},
	}
	rs, err := m.Load(ctx, sandboxtest.Scope(2), srcs, sandbox.RouteLimits{})
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close(ctx)
	if n := len(rs.Stats()); n != 2 {
		t.Fatalf("pools = %d, want 2 (identical code shares one)", n)
	}
	a, _ := rs.Module(srcs[0].StepPath)
	b, _ := rs.Module(srcs[1].StepPath)
	if a != b {
		t.Fatal("identical steps of two branches must resolve to the same module")
	}
	_ = rs.Close(ctx)
	if _, err := rs.Attach(ctx, srcs[2]); err == nil {
		t.Fatal("attach after close must fail")
	}
}

func TestRouteUsageAccounting(t *testing.T) {
	m := sandboxtest.NewManager(t)
	ctx := context.Background()
	mod, rs := sandboxtest.LoadOne(t, m, sandbox.EntryTransform, echo)
	for i := range 5 {
		res, err := mod.Call(ctx, sandbox.Invocation{Event: boundary.GuestEvent{Value: i, Offset: int64(i)}, Now: time.Unix(1, 0)})
		if err != nil || res.CPUTime <= 0 || res.CPUTime > res.Duration {
			t.Fatalf("call %d: cpu %v duration %v err %v", i, res.CPUTime, res.Duration, err)
		}
	}
	u := rs.Usage()
	if u.Calls != 5 || u.CPUTime <= 0 || u.MemoryBytes <= 0 || u.Instances < 1 || u.Modules != 1 || u.RouteID != "r1" {
		t.Fatalf("usage %+v", u)
	}
	if !usageListed(m.Usage(), u.CPUTime) {
		t.Fatalf("manager usage %+v does not list the route", m.Usage())
	}
	_ = rs.Close(ctx)
	if usageListed(m.Usage(), u.CPUTime) {
		t.Fatal("closed route sandboxes must leave the manager usage")
	}
}

func usageListed(list []sandbox.RouteUsage, cpu time.Duration) bool {
	for _, u := range list {
		if u.RouteID == "r1" && u.CPUTime == cpu {
			return true
		}
	}
	return false
}

type fixedUsage []sandbox.RouteUsage

func (f *fixedUsage) Usage() []sandbox.RouteUsage { return *f }

func TestUsageSamplerCPUShare(t *testing.T) {
	start := time.Unix(100, 0)
	src := &fixedUsage{{RouteID: "a", VersionID: "1", CPUTime: 200 * time.Millisecond, MemoryBytes: 10},
		{RouteID: "a", VersionID: "1", CPUTime: 300 * time.Millisecond, MemoryBytes: 5}}
	s := sandbox.NewUsageSampler(src, start)
	loads := s.Sample(start.Add(time.Second))
	if len(loads) != 1 || loads[0].CPUShare != 0.5 || loads[0].MemoryBytes != 15 {
		t.Fatalf("first sample %+v", loads)
	}
	*src = fixedUsage{{RouteID: "a", VersionID: "1", CPUTime: 1500 * time.Millisecond}}
	loads = s.Sample(start.Add(3 * time.Second))
	if loads[0].CPUShare != 0.5 {
		t.Fatalf("second sample %+v", loads)
	}
	*src = fixedUsage{{RouteID: "a", VersionID: "1"}}
	if loads = s.Sample(start.Add(4 * time.Second)); loads[0].CPUShare != 0 {
		t.Fatalf("closed sandbox must not report negative load: %+v", loads)
	}
}

func testScope() sandbox.RouteScope {
	s := sandboxtest.Scope(1)
	s.Mode = sandbox.ScopeTest
	return s
}

func loadEcho(t *testing.T, m sandbox.Manager, scope sandbox.RouteScope) sandbox.Module {
	t.Helper()
	rs, err := m.Load(context.Background(), scope, []sandbox.Source{{StepPath: "/s", Entry: sandbox.EntryTransform, Code: echo}}, sandbox.RouteLimits{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rs.Close(context.Background()) })
	mod, _ := rs.Module("/s")
	return mod
}

func TestTestModePinReplaysSavedCase(t *testing.T) {
	m := sandboxtest.NewManager(t)
	ctx := context.Background()
	ev := boundary.GuestEvent{Topic: "in", Offset: 42, Value: "x"}
	recorded, err := loadEcho(t, m, sandboxtest.Scope(1)).Call(ctx, sandbox.Invocation{Event: ev, Now: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil || recorded.Determinism != determinism.VersionCurrent {
		t.Fatalf("record: %+v %v", recorded, err)
	}
	pin := determinism.Current(recorded.Seed, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	test := loadEcho(t, m, testScope())
	other := boundary.GuestEvent{Topic: "elsewhere", Offset: 7, Value: "x"}
	viaInv, err := test.Call(ctx, sandbox.Invocation{Event: other, Now: time.Now(), Pin: &pin})
	if err != nil || !sameValue(viaInv, recorded) {
		t.Fatalf("pinned call %+v differs from recorded %+v (%v)", viaInv.Outputs, recorded.Outputs, err)
	}
	viaCtx, err := test.Call(determinism.PinContext{}.With(ctx, pin), sandbox.Invocation{Event: other, Now: time.Now()})
	if err != nil || !sameValue(viaCtx, recorded) {
		t.Fatalf("context-pinned call %+v differs (%v)", viaCtx.Outputs, err)
	}
	stale := pin
	stale.Version = determinism.VersionCurrent + 1
	_, err = test.Call(ctx, sandbox.Invocation{Event: ev, Pin: &stale})
	if se, ok := sandbox.AsError(err); !ok || se.Kind != sandbox.ErrorDeterminism || se.Retryable {
		t.Fatalf("stale pin: %v", err)
	}
	worker := sandboxtest.Scope(1)
	worker.Mode = sandbox.ScopeWorker
	ignored, err := loadEcho(t, m, worker).Call(ctx, sandbox.Invocation{Event: other, Now: time.Unix(5, 0), Pin: &stale})
	if err != nil || ignored.Seed == pin.Seed {
		t.Fatalf("worker mode must ignore pins: %+v %v", ignored, err)
	}
}

func sameValue(a, b sandbox.Result) bool {
	av, _ := a.Outputs[0].Value.(map[string]any)
	bv, _ := b.Outputs[0].Value.(map[string]any)
	return av != nil && bv != nil && av["r"] == bv["r"] && av["now"] == bv["now"]
}

func TestScopeFormatsSelectTaggedBoundary(t *testing.T) {
	m := sandboxtest.NewManager(t)
	scope := sandboxtest.Scope(1)
	scope.Formats = []boundary.Format{boundary.FormatAvro}
	rs, err := m.Load(context.Background(), scope, []sandbox.Source{{StepPath: "/s", Entry: sandbox.EntryTransform,
		Code: "function transform(e) { return { value: { id: e.value.id, kind: typeof e.value.id, next: { $long: (BigInt(e.value.id.$long) + 1n).toString() } } }; }"}}, sandbox.RouteLimits{})
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close(context.Background())
	mod, _ := rs.Module("/s")
	res, err := mod.Call(context.Background(), sandbox.Invocation{Event: boundary.GuestEvent{Value: map[string]any{"id": int64(1<<60 + 1)}}})
	if err != nil {
		t.Fatal(err)
	}
	v := res.Outputs[0].Value.(map[string]any)
	if v["id"] != int64(1<<60+1) || v["kind"] != "object" || v["next"] != int64(1<<60+2) {
		t.Fatalf("tagged boundary value %#v", v)
	}
}
