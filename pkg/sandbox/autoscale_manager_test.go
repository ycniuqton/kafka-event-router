package sandbox_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/engines"
	"github.com/eventrouter/router/pkg/sandbox/sandboxtest"
)

// fastAutoscale steps every 20 ms and shrinks after 3 idle windows.
func fastAutoscale(c *sandbox.Config) {
	c.Autoscale.Interval, c.Autoscale.IdleWindows = 20*time.Millisecond, 3
}

func autoscaledManager(t *testing.T, reg *prometheus.Registry) *sandbox.DefaultManager {
	t.Helper()
	cfg := sandbox.DefaultConfig()
	fastAutoscale(&cfg)
	obs, err := sandbox.NewPromObserver(reg)
	if err != nil {
		t.Fatal(err)
	}
	m, err := sandbox.NewManager(context.Background(), cfg, sandbox.ManagerDeps{Engines: engines.Registry(),
		Salts: sandboxtest.Salts{}, Binder: sandboxtest.Binder{}, Observer: obs})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close(context.Background()) })
	return m
}

func workerScope(start int, pool sandbox.PoolRequest) sandbox.RouteScope {
	return sandbox.RouteScope{RouteID: "r1", VersionID: "9b2f-uuid", VersionNo: 7, TeamID: "t1", Parallelism: start,
		Mode: sandbox.ScopeWorker, Pool: &pool}
}

// hammer calls mod from n goroutines until stop closes and returns the number of calls.
func hammer(t *testing.T, mod sandbox.Module, n int, stop <-chan struct{}) *atomic.Int64 {
	t.Helper()
	var calls atomic.Int64
	inv := sandbox.Invocation{Event: typicalEvent(), Now: time.Unix(1700000001, 0)}
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := mod.Call(context.Background(), inv); err != nil {
					t.Error(err)
					return
				}
				calls.Add(1)
			}
		}()
	}
	t.Cleanup(wg.Wait)
	return &calls
}

func capacityOf(rs sandbox.RouteSandbox) int { return rs.Stats()[0].Capacity }

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// A queued JS step grows from 2 toward its max, then shrinks back to min once idle; metrics carry the
// route id and the version number.
func TestManagerAutoscalesWorkerPool(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := autoscaledManager(t, reg)
	rs, err := m.Load(context.Background(), workerScope(2, sandbox.PoolRequest{Min: 1, Max: 6}),
		[]sandbox.Source{{StepPath: "/s", Entry: sandbox.EntryTransform, Code: typicalTransform}}, sandbox.RouteLimits{})
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close(context.Background())
	mod, _ := rs.Module("/s")
	stop := make(chan struct{})
	calls := hammer(t, mod, 12, stop)
	waitFor(t, "pool growth above 2", func() bool { return capacityOf(rs) > 2 })
	close(stop)
	waitFor(t, "idle shrink to min", func() bool { return capacityOf(rs) == 1 })
	if calls.Load() == 0 {
		t.Fatal("no calls")
	}
	assertJSSeries(t, reg)
}

// assertJSSeries checks router_js_pool_instances and router_js_queue_wait_seconds use version "7".
func assertJSSeries(t *testing.T, reg *prometheus.Registry) {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, f := range families {
		for _, metric := range f.GetMetric() {
			labels := map[string]string{}
			for _, l := range metric.GetLabel() {
				labels[l.GetName()] = l.GetValue()
			}
			if labels["route_id"] != "r1" || labels["version"] != "7" {
				t.Fatalf("%s labels %v: version must be the version number", f.GetName(), labels)
			}
			seen[f.GetName()] = true
		}
	}
	for _, name := range []sandbox.MetricName{sandbox.MetricPoolInstances, sandbox.MetricQueueWait, sandbox.MetricCallDuration} {
		if !seen[string(name)] {
			t.Fatalf("missing %s", name)
		}
	}
}

// Two JS steps of one version are two sandboxes; the instance gauge is their sum, not the last writer.
func TestPoolGaugeSumsEveryStepOfTheVersion(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := autoscaledManager(t, reg)
	scope := workerScope(3, sandbox.PoolRequest{Min: 3, Max: 3})
	for _, path := range []string{"/a", "/b"} {
		rs, err := m.Load(context.Background(), scope, []sandbox.Source{{StepPath: path, Entry: sandbox.EntryTransform, Code: typicalTransform}}, sandbox.RouteLimits{})
		if err != nil {
			t.Fatal(err)
		}
		defer rs.Close(context.Background())
		if _, err := m.Prewarm(context.Background(), scope); err != nil {
			t.Fatal(err)
		}
	}
	m.PublishUsage()
	if got := instancesGauge(t, reg); got != 6 {
		t.Fatalf("router_js_pool_instances = %v, want 6 (3 + 3)", got)
	}
}

func instancesGauge(t *testing.T, reg *prometheus.Registry) float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() == string(sandbox.MetricPoolInstances) {
			return f.GetMetric()[0].GetGauge().GetValue()
		}
	}
	return -1
}
