package sandbox_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/host"
	"github.com/eventrouter/router/pkg/sandbox/sandboxtest"
)

type countingCompiler struct {
	calls atomic.Int64
	fail  bool
}

func (c *countingCompiler) Compile(_ context.Context, src sandbox.Source, lim sandbox.Limits) (sandbox.Program, error) {
	c.calls.Add(1)
	if c.fail {
		return nil, errors.New("boom")
	}
	return stubProgram{src: src, lim: lim}, nil
}

type stubProgram struct {
	src sandbox.Source
	lim sandbox.Limits
}

func (p stubProgram) Source() sandbox.Source { return p.src }
func (p stubProgram) Limits() sandbox.Limits { return p.lim }
func (stubProgram) NewInstance(context.Context, *host.Registry) (sandbox.Instance, error) {
	return nil, errors.New("stub")
}

func TestRouteProgramCacheCompilesOncePerKey(t *testing.T) {
	cc := &countingCompiler{}
	cache := sandbox.NewRouteProgramCache(cc)
	src := sandbox.Source{StepPath: "/a", Entry: sandbox.EntryTransform, Code: "function transform(e){return e}"}
	key := sandbox.ProgramKeyOf(sandbox.RouteScope{RouteID: "r1"}, src, sandbox.LimitDefault)
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := cache.Acquire(context.Background(), key, src); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if st := cache.Stats(); cc.calls.Load() != 1 || st.Compiles != 1 || st.Hits != 15 || st.Entries != 1 {
		t.Fatalf("compiles=%d stats=%+v", cc.calls.Load(), st)
	}
	for range 16 {
		cache.Release(key)
	}
	if st := cache.Stats(); st.Entries != 0 {
		t.Fatalf("entry must be evicted after the last release: %+v", st)
	}
}

func TestRouteProgramCacheKeysByRouteAndLimits(t *testing.T) {
	src := sandbox.Source{Entry: sandbox.EntryTransform, Code: "x"}
	a := sandbox.ProgramKeyOf(sandbox.RouteScope{RouteID: "r1", VersionID: "v1"}, src, sandbox.LimitDefault)
	b := sandbox.ProgramKeyOf(sandbox.RouteScope{RouteID: "r1", VersionID: "v2"}, sandbox.Source{StepPath: "/other", Entry: src.Entry, Code: src.Code}, sandbox.LimitDefault)
	other := sandbox.ProgramKeyOf(sandbox.RouteScope{RouteID: "r2"}, src, sandbox.LimitDefault)
	lim := sandbox.LimitDefault
	lim.Timeout *= 2
	slower := sandbox.ProgramKeyOf(sandbox.RouteScope{RouteID: "r1"}, src, lim)
	if a != b || a == other || a == slower {
		t.Fatal("key must ignore version and step path but separate routes and limits")
	}
}

func TestRouteProgramCacheDoesNotKeepFailures(t *testing.T) {
	cc := &countingCompiler{fail: true}
	cache := sandbox.NewRouteProgramCache(cc)
	key := sandbox.ProgramKey{RouteID: "r1", Module: "k"}
	for range 2 {
		if _, err := cache.Acquire(context.Background(), key, sandbox.Source{}); err == nil {
			t.Fatal("want compile error")
		}
	}
	if cc.calls.Load() != 2 || cache.Stats().Entries != 0 {
		t.Fatalf("failed compiles must not be cached: calls=%d %+v", cc.calls.Load(), cache.Stats())
	}
}

func TestProgramCacheRegistry(t *testing.T) {
	reg := sandbox.NewProgramCacheRegistry()
	if _, err := reg.Build("nope", &countingCompiler{}); err == nil {
		t.Fatal("unknown kind must fail")
	}
	cc := &countingCompiler{}
	off, err := reg.Build(sandbox.ProgramCacheOff, cc)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		_, _ = off.Acquire(context.Background(), sandbox.ProgramKey{}, sandbox.Source{})
	}
	if cc.calls.Load() != 3 {
		t.Fatalf("off cache compiles every time, got %d", cc.calls.Load())
	}
}

func TestManagerCompilesLibraryCodeOncePerRoute(t *testing.T) {
	m := sandboxtest.NewManager(t)
	ctx := context.Background()
	library := "function transform(e, ctx) { return { value: { lib: true, v: e.value } }; }"
	srcs := []sandbox.Source{
		{StepPath: "/branches/0/transform/steps/0/steps/0/code", Entry: sandbox.EntryTransform, Code: library},
		{StepPath: "/branches/1/transform/steps/3/steps/0/code", Entry: sandbox.EntryTransform, Code: library},
	}
	v1 := sandboxtest.Scope(2)
	rs1, err := m.Load(ctx, v1, srcs, sandbox.RouteLimits{})
	if err != nil {
		t.Fatal(err)
	}
	v2 := v1
	v2.VersionID = "v2"
	rs2, err := m.Load(ctx, v2, srcs, sandbox.RouteLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if st := m.Programs(); st.Compiles != 1 || st.Hits != 1 || st.Entries != 1 {
		t.Fatalf("library code of two versions must compile once: %+v", st)
	}
	other := v1
	other.RouteID = "r2"
	rs3, err := m.Load(ctx, other, srcs, sandbox.RouteLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if st := m.Programs(); st.Compiles != 2 || st.Entries != 2 {
		t.Fatalf("another route compiles its own program: %+v", st)
	}
	for _, rs := range []sandbox.RouteSandbox{rs1, rs2, rs3} {
		_ = rs.Close(ctx)
	}
	if st := m.Programs(); st.Entries != 0 {
		t.Fatalf("closing every version must evict the programs: %+v", st)
	}
}
