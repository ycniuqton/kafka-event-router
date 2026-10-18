package quickjs

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/determinism"
	"github.com/eventrouter/router/pkg/sandbox/host"
)

func newTestRuntime(t *testing.T, policy sandbox.ResetPolicy) *Runtime {
	t.Helper()
	rt, err := New(context.Background(), sandbox.RuntimeOptions{ResetPolicy: policy, CheckLimits: sandbox.LimitDefault,
		Prelude: determinism.DefaultPrelude()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close(context.Background()) })
	return rt
}

func newTestInstance(t *testing.T, rt *Runtime, code string, lim sandbox.Limits) *instance {
	t.Helper()
	p, err := rt.Compile(context.Background(), sandbox.Source{StepPath: "/s", Entry: sandbox.EntryTransform, Code: code}, lim)
	if err != nil {
		t.Fatal(err)
	}
	inst, err := p.NewInstance(context.Background(), host.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = inst.Close(context.Background()) })
	return inst.(*instance)
}

func guestCall(input string) sandbox.GuestCall {
	return sandbox.GuestCall{Input: []byte(input), Clock: determinism.Freeze(time.Unix(1, 0)), Logs: host.NewLogBuffer(0)}
}

func TestInstanceRecyclesAfterMemoryGrowth(t *testing.T) {
	rt := newTestRuntime(t, sandbox.ResetFreshContext)
	lim := sandbox.LimitDefault
	lim.Timeout = time.Second
	inst := newTestInstance(t, rt, "function transform(e){ return { value: new Uint8Array(e.value).length }; }", lim)
	if _, err := inst.Invoke(context.Background(), guestCall(`{"value":10}`)); err != nil || !inst.Reusable() {
		t.Fatalf("small call: %v reusable=%v", err, inst.Reusable())
	}
	if _, err := inst.Invoke(context.Background(), guestCall(`{"value":20000000}`)); err != nil {
		t.Fatalf("large call: %v", err)
	}
	if inst.Reusable() {
		t.Fatal("an instance whose memory grew past the snapshot must be recycled")
	}
}

func TestLockdownReusePolicyKeepsWorking(t *testing.T) {
	rt := newTestRuntime(t, sandbox.ResetLockdownReuse)
	inst := newTestInstance(t, rt, "function transform(e){ return { value: typeof Object.prototype.x }; }", sandbox.LimitDefault)
	for i := 0; i < 3; i++ {
		reply, err := inst.Invoke(context.Background(), guestCall(`{"value":1}`))
		if err != nil || !strings.Contains(string(reply.Output), "undefined") {
			t.Fatalf("call %d: %s %v", i, reply.Output, err)
		}
	}
	if inst.snapshot != nil {
		t.Fatal("lockdown_reuse must not snapshot")
	}
}

func TestClosedInstanceRefusesCalls(t *testing.T) {
	rt := newTestRuntime(t, sandbox.ResetFreshContext)
	inst := newTestInstance(t, rt, "function transform(e){ return e }", sandbox.LimitDefault)
	_ = inst.Close(context.Background())
	if _, err := inst.Invoke(context.Background(), guestCall(`{}`)); err == nil || inst.Reusable() || inst.MemoryBytes() != 0 {
		t.Fatalf("closed instance: err=%v", err)
	}
}

func TestRuntimeRejectsUnknownGuestExport(t *testing.T) {
	rt := newTestRuntime(t, sandbox.ResetFreshContext)
	grp, err := rt.group(context.Background(), sandbox.LimitDefault.MemoryBytes)
	if err != nil {
		t.Fatal(err)
	}
	mod, _ := grp.newModule(context.Background())
	defer mod.Close(context.Background())
	if _, err := newGuestABI(mod); err != nil {
		t.Fatalf("real guest must expose the ABI: %v", err)
	}
	if rt.Kind() != sandbox.EngineQuickJSWazero {
		t.Fatal("kind")
	}
}
