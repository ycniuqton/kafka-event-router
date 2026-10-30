package sandbox_test

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/sandbox"
)

func scaler(t *testing.T) sandbox.PoolScaler {
	t.Helper()
	s, err := sandbox.NewPoolScalers().For(sandbox.PoolBounds{Min: 1, Max: 8}, sandbox.AutoscaleDefault)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func queuedWindow(size int) sandbox.PoolWindow {
	return sandbox.PoolWindow{Calls: 1000, Waited: 400, WaitTotal: 400 * time.Millisecond, PeakInUse: size, Size: size}
}

func TestQueueWaitScalerGrowsWhileCallsWaitAndCPUAllows(t *testing.T) {
	s := scaler(t)
	if got := s.Next(sandbox.PoolSignal{Window: queuedWindow(4), CPU: 0.5}); got != 5 {
		t.Fatalf("grow from 4: %d", got)
	}
	if got := s.Next(sandbox.PoolSignal{Window: queuedWindow(8), CPU: -1}); got != 10 {
		t.Fatalf("unknown CPU must not block growth (+25%%): %d", got)
	}
	if got := s.Next(sandbox.PoolSignal{Window: queuedWindow(4), CPU: 0.9}); got != 4 {
		t.Fatalf("CPU ceiling must hold: %d", got)
	}
	few := sandbox.PoolWindow{Calls: 1000, Waited: 10, PeakInUse: 4, Size: 4}
	if got := s.Next(sandbox.PoolSignal{Window: few, CPU: 0.1}); got != 4 {
		t.Fatalf("1%% waits must not grow: %d", got)
	}
}

func TestQueueWaitScalerShrinksAfterIdleWindows(t *testing.T) {
	s := scaler(t)
	idle := sandbox.PoolWindow{Calls: 10, PeakInUse: 1, Size: 4}
	for i := 1; i < sandbox.AutoscaleDefault.IdleWindows; i++ {
		if got := s.Next(sandbox.PoolSignal{Window: idle}); got != 4 {
			t.Fatalf("window %d shrank early to %d", i, got)
		}
	}
	if got := s.Next(sandbox.PoolSignal{Window: idle}); got != 3 {
		t.Fatalf("expected shrink to 3, got %d", got)
	}
	busy := sandbox.PoolWindow{Calls: 10, PeakInUse: 3, Size: 3}
	for range sandbox.AutoscaleDefault.IdleWindows * 2 {
		if got := s.Next(sandbox.PoolSignal{Window: busy}); got != 3 {
			t.Fatalf("fully used pool must hold, got %d", got)
		}
	}
}

func TestPoolSizingResolvesAutoAndClamps(t *testing.T) {
	sz := sandbox.PoolSizing{Tuning: sandbox.AutoscaleDefault, Cores: 6}
	worker := sandbox.RouteScope{Parallelism: 4, Mode: sandbox.ScopeWorker, Pool: &sandbox.PoolRequest{Min: 1}}
	if start, b := sz.For(worker); start != 4 || b != (sandbox.PoolBounds{Min: 1, Max: 6}) {
		t.Fatalf("auto: start %d bounds %+v", start, b)
	}
	small := sandbox.PoolSizing{Tuning: sandbox.AutoscaleDefault, Cores: 1}
	if start, b := small.For(worker); start != 4 || b.Max != 4 {
		t.Fatalf("auto max never drops below the old fixed 4: start %d %+v", start, b)
	}
	worker.Pool = &sandbox.PoolRequest{Min: 2, Max: 100}
	if _, b := sz.For(worker); b.Max != sandbox.AutoscaleDefault.MaxInstances {
		t.Fatalf("operator cap: %+v", b)
	}
	worker.Pool = &sandbox.PoolRequest{Min: 2, Max: 2}
	if start, b := sz.For(worker); start != 2 || b.Autoscaled() {
		t.Fatalf("min = max is fixed: start %d %+v", start, b)
	}
	preview := sandbox.RouteScope{Parallelism: 1, Mode: sandbox.ScopePreview, Pool: &sandbox.PoolRequest{Min: 1, Max: 8}}
	if start, b := sz.For(preview); start != 1 || b.Autoscaled() {
		t.Fatalf("preview stays fixed: %d %+v", start, b)
	}
}

func scaledPool(t *testing.T, start int, b sandbox.PoolBounds, ceiling *sandbox.MemoryCeiling) (*sandbox.BoundedPool, *fakeProgram) {
	t.Helper()
	prog := &fakeProgram{}
	pool, err := sandbox.NewBoundedPool(context.Background(), prog, nil, sandbox.PoolOptions{Max: start, Warm: 1, MaxIdle: 1, Scale: b, Ceiling: ceiling})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close(context.Background()) })
	return pool, prog
}

func TestPoolGrowWakesWaiters(t *testing.T) {
	pool, _ := scaledPool(t, 1, sandbox.PoolBounds{Min: 1, Max: 4}, nil)
	ctx := context.Background()
	first, _ := pool.Acquire(ctx)
	got := make(chan sandbox.Instance, 2)
	for range 2 {
		go func() {
			inst, err := pool.Acquire(ctx)
			if err == nil {
				got <- inst
			}
		}()
	}
	time.Sleep(20 * time.Millisecond)
	if n := pool.Resize(3); n != 3 {
		t.Fatalf("resize applied %d", n)
	}
	for range 2 {
		select {
		case inst := <-got:
			pool.Release(inst)
		case <-time.After(time.Second):
			t.Fatal("waiter not woken by growth")
		}
	}
	pool.Release(first)
	w := pool.TakeWindow()
	if w.Waited != 2 || w.Calls != 3 || w.Size != 3 {
		t.Fatalf("window %+v", w)
	}
}

func TestPoolShrinkClosesSurplusInstances(t *testing.T) {
	pool, prog := scaledPool(t, 4, sandbox.PoolBounds{Min: 1, Max: 4}, nil)
	ctx := context.Background()
	insts := make([]sandbox.Instance, 4)
	for i := range insts {
		insts[i], _ = pool.Acquire(ctx)
	}
	for _, inst := range insts[:2] {
		pool.Release(inst)
	}
	if st := pool.Stats(); st.Idle != 2 {
		t.Fatalf("autoscaled pool keeps grown instances idle: %+v", st)
	}
	pool.Resize(1)
	if st := pool.Stats(); st.Size != 2 || st.Idle != 0 || prog.closed.Load() != 2 {
		t.Fatalf("idle surplus not closed: %+v closed=%d", st, prog.closed.Load())
	}
	pool.Release(insts[2])
	pool.Release(insts[3])
	if st := pool.Stats(); st.Size != 1 || st.Idle != 1 || st.Capacity != 1 || prog.closed.Load() != 3 {
		t.Fatalf("busy surplus must close on release, the last one stays idle: %+v closed=%d", st, prog.closed.Load())
	}
}

func TestPoolGrowthRespectsMemoryCeiling(t *testing.T) {
	slot := sandbox.LimitDefault.MemoryBytes
	ceiling := sandbox.NewMemoryCeiling(3 * slot)
	a, _ := scaledPool(t, 2, sandbox.PoolBounds{Min: 1, Max: 8}, ceiling)
	b, _ := scaledPool(t, 2, sandbox.PoolBounds{Min: 1, Max: 8}, ceiling)
	if ceiling.Used() != 4*slot {
		t.Fatalf("starting slots must be reserved even above the ceiling: %d", ceiling.Used()/slot)
	}
	if n := a.Resize(8); n != 2 {
		t.Fatalf("growth above the ceiling must be refused, got %d", n)
	}
	b.Resize(1)
	if n := a.Resize(8); n != 2 {
		t.Fatalf("one freed slot still leaves the worker at the ceiling (3 = 2 + 1), got %d", n)
	}
	b.Resize(1)
	_ = b.Close(context.Background())
	if n := a.Resize(8); n != 3 {
		t.Fatalf("closing b frees its slot, got %d", n)
	}
	_ = a.Close(context.Background())
	if ceiling.Used() != 0 {
		t.Fatalf("closed pools must return every reservation, used %d slots", ceiling.Used()/slot)
	}
}

// Calls keep running while the autoscaler shrinks and grows the pool; none is lost or stuck.
func TestPoolResizeUnderLoadLosesNoCall(t *testing.T) {
	pool, prog := scaledPool(t, 4, sandbox.PoolBounds{Min: 1, Max: 8}, nil)
	stop := make(chan struct{})
	go func() {
		rng := rand.New(rand.NewSource(3))
		for {
			select {
			case <-stop:
				return
			default:
				pool.Resize(1 + rng.Intn(8))
				time.Sleep(100 * time.Microsecond)
			}
		}
	}()
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 300 {
				inst, err := pool.Acquire(context.Background())
				if err != nil {
					t.Error(err)
					return
				}
				pool.Release(inst)
				mu.Lock()
				done++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	close(stop)
	st := pool.Stats()
	if done != 16*300 || st.InUse != 0 || int(prog.created.Load()-prog.closed.Load()) != st.Size || st.Size > st.Capacity {
		t.Fatalf("done %d stats %+v created %d closed %d", done, st, prog.created.Load(), prog.closed.Load())
	}
}

// fakeScalable scripts the windows of one autoscaled pool.
type fakeScalable struct {
	sandbox.Pool
	bounds  sandbox.PoolBounds
	size    int
	windows func(size int) sandbox.PoolWindow
}

func (f *fakeScalable) Bounds() sandbox.PoolBounds     { return f.bounds }
func (f *fakeScalable) TakeWindow() sandbox.PoolWindow { return f.windows(f.size) }
func (f *fakeScalable) Resize(n int) int {
	f.size = f.bounds.Clamp(n)
	return f.size
}

type fakePools struct {
	pools     []sandbox.ScopedPool
	published int
}

func (f *fakePools) ScalablePools() []sandbox.ScopedPool { return f.pools }
func (f *fakePools) PublishUsage()                       { f.published++ }

type fixedCPU float64

func (c fixedCPU) Busy(time.Time) float64 { return float64(c) }

// Simulated clock: 5 queued windows grow 4 → 8 (bounded), then an idle phase shrinks back toward min.
func TestAutoscalerStepsOnSimulatedClock(t *testing.T) {
	load := true
	pool := &fakeScalable{bounds: sandbox.PoolBounds{Min: 2, Max: 8}, size: 4, windows: func(size int) sandbox.PoolWindow {
		if load {
			return queuedWindow(size)
		}
		return sandbox.PoolWindow{Calls: 5, PeakInUse: 1, Size: size}
	}}
	src := &fakePools{pools: []sandbox.ScopedPool{{Scope: sandbox.RouteScope{RouteID: "r"}, Pool: pool}}}
	a := sandbox.NewPoolAutoscaler(sandbox.AutoscaleDefault, sandbox.NewPoolScalers(), fixedCPU(0.4), src)
	clock := time.Unix(0, 0)
	var sizes []int
	for range 5 {
		clock = clock.Add(time.Second)
		a.Step(clock)
		sizes = append(sizes, pool.size)
	}
	if sizes[0] != 5 || sizes[4] != 8 {
		t.Fatalf("growth %v", sizes)
	}
	load = false
	for range sandbox.AutoscaleDefault.IdleWindows * 10 {
		clock = clock.Add(time.Second)
		a.Step(clock)
	}
	if pool.size != 2 || src.published != 5+sandbox.AutoscaleDefault.IdleWindows*10 {
		t.Fatalf("idle shrink to min: size %d published %d", pool.size, src.published)
	}
}
