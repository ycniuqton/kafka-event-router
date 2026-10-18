package sandbox_test

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/boundary"
	"github.com/eventrouter/router/pkg/sandbox/sandboxtest"
)

const reloadCode = `function transform(e, ctx) {
  const items = (e.value.items || []).map(i => ({ sku: i.sku, total: i.qty * i.price }));
  return { value: { id: e.value.id, items, sum: items.reduce((a, i) => a + i.total, 0) } };
}`

func reloadEvent(i int) boundary.GuestEvent {
	return boundary.GuestEvent{Topic: "orders", Offset: int64(i), Value: map[string]any{
		"id": i, "items": []any{map[string]any{"sku": "a", "qty": 2, "price": 3.5}}}}
}

func versionScope(version string, parallelism int) sandbox.RouteScope {
	s := sandboxtest.Scope(parallelism)
	s.VersionID, s.Mode = version, sandbox.ScopeWorker
	return s
}

func loadVersion(t *testing.T, m sandbox.Manager, scope sandbox.RouteScope) (sandbox.RouteSandbox, sandbox.Module) {
	t.Helper()
	rs, err := m.Load(context.Background(), scope, []sandbox.Source{{StepPath: "/s", Entry: sandbox.EntryTransform, Code: reloadCode}}, sandbox.RouteLimits{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rs.Close(context.Background()) })
	mod, _ := rs.Module("/s")
	return rs, mod
}

func TestPrewarmFillsPoolsOfOneVersion(t *testing.T) {
	m := sandboxtest.NewManager(t)
	ctx := context.Background()
	v1, _ := loadVersion(t, m, versionScope("v1", 4))
	v2, _ := loadVersion(t, m, versionScope("v2", 4))
	report, err := m.Prewarm(ctx, versionScope("v2", 4))
	if err != nil {
		t.Fatal(err)
	}
	if report.Sandboxes != 1 || report.Modules != 1 || report.Instances != 3 {
		t.Fatalf("report %+v, want 3 new instances in one module", report)
	}
	if got := v2.Usage().Instances; got != 4 {
		t.Fatalf("v2 instances = %d, want 4", got)
	}
	if got := v1.Usage().Instances; got != 1 {
		t.Fatalf("v1 must stay untouched, has %d", got)
	}
	again, _ := m.Prewarm(ctx, versionScope("v2", 4))
	if again.Instances != 0 {
		t.Fatalf("second prewarm must be a no-op: %+v", again)
	}
	_ = v2.Close(ctx)
	if r, _ := m.Prewarm(ctx, versionScope("v2", 4)); r.Sandboxes != 0 {
		t.Fatalf("closed versions are not warmed: %+v", r)
	}
}

func TestPrewarmTargetIsBoundedByIdleCap(t *testing.T) {
	target := sandbox.PrewarmTarget{MaxIdle: 4}
	cases := map[int]int{0: 1, 1: 1, 3: 3, 16: 4}
	for parallelism, want := range cases {
		if got := target.For(sandbox.RouteScope{Parallelism: parallelism}); got != want {
			t.Fatalf("parallelism %d: target %d, want %d", parallelism, got, want)
		}
	}
}

// TestHotReloadUnderLoadHasNoColdStart (JS-205): v1 serves under load while v2 compiles and is
// pre-warmed; switching every worker to v2 at once boots no instance on the hot path.
func TestHotReloadUnderLoadHasNoColdStart(t *testing.T) {
	const workers = 4
	m := sandboxtest.NewManager(t)
	ctx := context.Background()
	_, v1 := loadVersion(t, m, versionScope("v1", workers))
	var current atomic.Value
	current.Store(v1)
	steady := runLoad(t, &current, workers, 200*time.Millisecond, func() {
		rs, v2 := loadVersion(t, m, versionScope("v2", workers))
		if _, err := m.Prewarm(ctx, versionScope("v2", workers)); err != nil {
			t.Error(err)
		}
		before := rs.Usage().ColdStarts
		current.Store(v2)
		t.Cleanup(func() {
			if after := rs.Usage().ColdStarts; after != before {
				t.Errorf("v2 booted %d instances on the hot path after pre-warm", after-before)
			}
		})
	})
	t.Logf("steady p50 %v p99 %v, first calls on v2 max %v", steady.p50, steady.p99, steady.firstMax)
	if limit := steady.p99*3 + 2*time.Millisecond; steady.firstMax > limit {
		t.Fatalf("first-event latency %v after reload exceeds %v", steady.firstMax, limit)
	}
}

// loadReport summarises latencies seen by the load loop.
type loadReport struct {
	p50, p99, firstMax time.Duration
}

// runLoad calls the current module from n goroutines, runs reload halfway and reports latencies.
func runLoad(t *testing.T, current *atomic.Value, n int, d time.Duration, reload func()) loadReport {
	t.Helper()
	var (
		mu       sync.Mutex
		lat      []time.Duration
		firstMax time.Duration
		wg       sync.WaitGroup
	)
	stop := make(chan struct{})
	for w := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var last sandbox.Module
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				mod := current.Load().(sandbox.Module)
				start := time.Now()
				if _, err := mod.Call(context.Background(), sandbox.Invocation{Event: reloadEvent(w*1_000_000 + i), Now: time.Unix(1, 0)}); err != nil {
					t.Error(err)
					return
				}
				took := time.Since(start)
				mu.Lock()
				if last != nil && last != mod {
					firstMax = max(firstMax, took)
				} else {
					lat = append(lat, took)
				}
				mu.Unlock()
				last = mod
			}
		}()
	}
	time.Sleep(d / 2)
	reload()
	time.Sleep(d / 2)
	close(stop)
	wg.Wait()
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	return loadReport{p50: lat[len(lat)/2], p99: lat[len(lat)*99/100], firstMax: firstMax}
}
