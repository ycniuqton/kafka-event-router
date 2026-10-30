package runner

import (
	"context"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/config"
)

func acquireAsync(l *InFlightLimiter, ctx context.Context) chan error {
	done := make(chan error, 1)
	go func() { done <- l.Acquire(ctx) }()
	return done
}

func waitQueued(t *testing.T, l *InFlightLimiter, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		l.mu.Lock()
		got := len(l.waiters)
		l.mu.Unlock()
		if got == n {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("expected %d waiters", n)
}

func TestLimiterGrowWakesWaiters(t *testing.T) {
	l := NewInFlightLimiter(1)
	_ = l.Acquire(t.Context())
	a, b := acquireAsync(l, t.Context()), acquireAsync(l, t.Context())
	waitQueued(t, l, 2)
	l.SetLimit(3)
	for _, ch := range []chan error{a, b} {
		if err := <-ch; err != nil {
			t.Fatal(err)
		}
	}
	if l.InUse() != 3 || !l.Full() {
		t.Fatalf("in use %d", l.InUse())
	}
}

func TestLimiterShrinkDrainsWithoutRevoking(t *testing.T) {
	l := NewInFlightLimiter(4)
	for range 4 {
		_ = l.Acquire(t.Context())
	}
	l.SetLimit(2)
	next := acquireAsync(l, t.Context())
	waitQueued(t, l, 1)
	l.Release()
	l.Release()
	select {
	case <-next:
		t.Fatal("acquired while still above the lowered limit")
	case <-time.After(20 * time.Millisecond):
	}
	l.Release()
	if err := <-next; err != nil {
		t.Fatal(err)
	}
	if l.InUse() != 2 {
		t.Fatalf("in use %d, want 2", l.InUse())
	}
}

func TestLimiterServesWaitersInArrivalOrder(t *testing.T) {
	l := NewInFlightLimiter(1)
	_ = l.Acquire(t.Context())
	var order []int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = l.Acquire(context.Background())
			mu.Lock()
			order = append(order, i)
			mu.Unlock()
			l.Release()
		}()
		waitQueued(t, l, i+1)
	}
	l.Release()
	wg.Wait()
	for i, v := range order {
		if v != i {
			t.Fatalf("order %v", order)
		}
	}
}

func TestLimiterCancelledWaiterLeavesQueue(t *testing.T) {
	l := NewInFlightLimiter(1)
	_ = l.Acquire(t.Context())
	ctx, cancel := context.WithCancel(t.Context())
	ch := acquireAsync(l, ctx)
	waitQueued(t, l, 1)
	cancel()
	if err := <-ch; err == nil {
		t.Fatal("cancelled acquire must fail")
	}
	l.Release()
	if l.InUse() != 0 {
		t.Fatalf("slot leaked: in use %d", l.InUse())
	}
}

func TestLimiterWindowStatistics(t *testing.T) {
	l := NewInFlightLimiter(1)
	start := time.Now()
	_ = l.Acquire(t.Context())
	ch := acquireAsync(l, t.Context())
	waitQueued(t, l, 1)
	time.Sleep(5 * time.Millisecond)
	l.Release()
	<-ch
	l.ObserveAck(10 * time.Millisecond)
	l.ObserveAck(30 * time.Millisecond)
	w := l.TakeWindow(start.Add(time.Second))
	if w.Acquired != 2 || w.Waited != 1 || w.WaitTotal < 5*time.Millisecond || w.PeakInUse != 1 || w.Limit != 1 {
		t.Fatalf("window %+v", w)
	}
	if w.AckLatency() != 20*time.Millisecond || w.Acks != 2 {
		t.Fatalf("ack latency %s", w.AckLatency())
	}
	if again := l.TakeWindow(start.Add(2 * time.Second)); again.Acquired != 0 || again.Acks != 0 || again.PeakInUse != 1 {
		t.Fatalf("window not reset: %+v", again)
	}
}

// laneRun emulates one partition lane: records are admitted one after another and released asynchronously
// after a random ack delay, while the limit is resized concurrently.
type laneRun struct {
	lane    int
	records int
	seen    []int
}

func (r *laneRun) run(l *InFlightLimiter, inflight *sync.WaitGroup, delivered *atomic.Int64) {
	rng := rand.New(rand.NewSource(int64(r.lane)))
	for seq := 0; seq < r.records; seq++ {
		if l.Acquire(context.Background()) != nil {
			return
		}
		r.seen = append(r.seen, seq)
		inflight.Add(1)
		delay := time.Duration(rng.Intn(200)) * time.Microsecond
		go func() {
			defer inflight.Done()
			time.Sleep(delay)
			delivered.Add(1)
			l.Release()
		}()
	}
}

// Per-partition order and completeness while the tuner resizes the limit from 1 to 64 under load.
func TestLimiterResizeKeepsLaneOrderAndLosesNothing(t *testing.T) {
	l := NewInFlightLimiter(8)
	stop := make(chan struct{})
	go func() {
		rng := rand.New(rand.NewSource(7))
		for {
			select {
			case <-stop:
				return
			default:
				l.SetLimit(1 + rng.Intn(64))
				time.Sleep(50 * time.Microsecond)
			}
		}
	}()
	lanes := make([]*laneRun, 12)
	var lanesDone, inflight sync.WaitGroup
	var delivered atomic.Int64
	for i := range lanes {
		lanes[i] = &laneRun{lane: i, records: 2000}
		lanesDone.Add(1)
		go func() { defer lanesDone.Done(); lanes[i].run(l, &inflight, &delivered) }()
	}
	lanesDone.Wait()
	inflight.Wait()
	close(stop)
	for _, r := range lanes {
		for i, seq := range r.seen {
			if seq != i {
				t.Fatalf("lane %d out of order at %d: %d", r.lane, i, seq)
			}
		}
	}
	if delivered.Load() != 12*2000 || l.InUse() != 0 {
		t.Fatalf("delivered %d, in use %d", delivered.Load(), l.InUse())
	}
}

type gaugeRecorder struct {
	mu     sync.Mutex
	values []int
}

func (g *gaugeRecorder) InFlightLimit(v int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.values = append(g.values, v)
}

type pressureFlag struct{ on atomic.Bool }

func (p *pressureFlag) UnderPressure() bool { return p.on.Load() }

func TestTunerStepAppliesControllerAndPressure(t *testing.T) {
	g, p := &gaugeRecorder{}, &pressureFlag{}
	b := config.InFlightBounds{Mode: config.InFlightModeAuto, Min: 10, Max: 100, Start: 80}
	tuner, err := NewInFlightTuner(InFlightTuning{Pressure: p}, b, g)
	if err != nil {
		t.Fatal(err)
	}
	if tuner.Limiter().Limit() != 80 {
		t.Fatalf("start %d", tuner.Limiter().Limit())
	}
	p.on.Store(true)
	now := time.Now()
	if got := tuner.Step(now.Add(time.Second)); got != 40 || tuner.Limiter().Limit() != 40 {
		t.Fatalf("pressure step %d", got)
	}
	if err := tuner.Rebind(config.InFlightBounds{Mode: config.InFlightModeAuto, Min: 50, Max: 60, Start: 55}, g); err != nil {
		t.Fatal(err)
	}
	if tuner.Limiter().Limit() != 50 {
		t.Fatalf("rebind must continue from the current limit clamped into new bounds, got %d", tuner.Limiter().Limit())
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.values) != 3 || g.values[0] != 80 || g.values[1] != 40 || g.values[2] != 50 {
		t.Fatalf("gauge %v", g.values)
	}
}

// BenchmarkInFlightLimiter is the per-record cost of the limiter (acquire, ack observation, release)
// with 12 lanes contending.
func BenchmarkInFlightLimiter(b *testing.B) {
	l := NewInFlightLimiter(1000)
	b.SetParallelism(12)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = l.Acquire(context.Background())
			l.ObserveAck(time.Millisecond)
			l.Release()
		}
	})
}
