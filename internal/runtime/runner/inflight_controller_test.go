package runner_test

import (
	"math"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/runtime/runner"
	"github.com/eventrouter/router/pkg/config"
)

// pipeModel is a deterministic model of one route: lanes can push min(cpu, demand) ev/s; each record is
// acked after base latency unless the broker saturates, where Little's law makes latency = in-flight / broker.
type pipeModel struct {
	cpu      float64
	broker   float64
	demand   float64
	base     time.Duration
	queueing bool
	seed     uint64
}

// window simulates one second at limit and returns what the limiter would have observed.
func (m *pipeModel) window(limit int) runner.InFlightWindow {
	want := m.cpu
	if m.demand > 0 {
		want = math.Min(want, m.demand)
	}
	lat, rate, binding := m.base, math.Min(want, float64(limit)/m.base.Seconds()), float64(limit)/m.base.Seconds() < want
	if m.queueing {
		lat = m.base + time.Duration(float64(limit)/m.cpu*float64(time.Second))
		rate, binding = float64(limit)/lat.Seconds(), true
	}
	if m.broker > 0 && rate > m.broker {
		rate, binding = m.broker, true
		lat = time.Duration(float64(limit) / m.broker * float64(time.Second))
	}
	rate *= m.jitter()
	acks := int64(rate)
	w := runner.InFlightWindow{Elapsed: time.Second, Acquired: acks, Acks: acks, AckTotal: lat * time.Duration(acks), Limit: limit,
		PeakInUse: int(math.Min(float64(limit), rate*lat.Seconds()))}
	if binding {
		w.Waited, w.WaitTotal, w.PeakInUse = acks/2, time.Duration(acks/2)*time.Millisecond, limit
	}
	return w
}

// jitter is a deterministic ±1 % noise (LCG) so throughput comparisons see realistic variance.
func (m *pipeModel) jitter() float64 {
	m.seed = m.seed*6364136223846793005 + 1442695040888963407
	return 0.99 + 0.02*float64(m.seed>>40)/float64(1<<24)
}

// run steps controller c for n windows on m and returns the limits and the last throughput.
func run(c runner.InFlightController, m *pipeModel, n int, pressure func(i int) bool) ([]int, float64) {
	limits := make([]int, 0, n)
	limit, rate := c.Current(), 0.0
	for i := 0; i < n; i++ {
		w := m.window(limit)
		rate = w.Throughput()
		limit = c.Next(runner.InFlightSignal{Window: w, Pressure: pressure != nil && pressure(i)})
		limits = append(limits, limit)
	}
	return limits, rate
}

func autoBounds(minV, maxV int) config.InFlightBounds {
	return config.InFlightBounds{Mode: config.InFlightModeAuto, Min: minV, Max: maxV, Start: min(max(1000, minV), maxV)}
}

func gradient(b config.InFlightBounds) runner.InFlightController {
	return runner.NewGradientInFlight(b, 0, runner.DefaultGradientTuning)
}

func assertWithin(t *testing.T, limits []int, b config.InFlightBounds) {
	t.Helper()
	for i, l := range limits {
		if l < b.Min || l > b.Max {
			t.Fatalf("window %d: limit %d outside [%d, %d]", i, l, b.Min, b.Max)
		}
	}
}

// S2 p12 shape: 1000 in flight at ~15 ms caps the route at ~66k while CPU could do 80k.
func TestGradientGrowsUntilCPUIsTheLimit(t *testing.T) {
	b := autoBounds(64, 10000)
	m := &pipeModel{cpu: 80000, base: 15 * time.Millisecond}
	limits, rate := run(gradient(b), m, 30, nil)
	assertWithin(t, limits, b)
	if rate < 0.95*m.cpu {
		t.Fatalf("throughput %.0f below 95%% of capacity %.0f (limits %v)", rate, m.cpu, limits)
	}
	if last := limits[len(limits)-1]; last > 2*int(m.cpu*m.base.Seconds()) {
		t.Fatalf("limit %d overshoots the knee %d", last, int(m.cpu*m.base.Seconds()))
	}
	t.Logf("limits %v → %.0f ev/s", limits, rate)
}

// Latency that rises with load (lat = base + inflight / capacity): the S2 shape where 1000 in flight gave
// ~60k and 5000 gave ~78k. The fast phase stops at 2× the long-term latency, then probing climbs while
// each step still pays off.
func TestGradientClimbsWithLoadDependentLatency(t *testing.T) {
	b := autoBounds(64, 10000)
	m := &pipeModel{cpu: 85000, base: 5 * time.Millisecond, queueing: true}
	first := m.window(1000).Throughput()
	limits, rate := run(gradient(b), m, 600, nil)
	assertWithin(t, limits, b)
	if rate < 1.2*first {
		t.Fatalf("throughput %.0f did not rise 20%% above the start %.0f (last limit %d)", rate, first, limits[len(limits)-1])
	}
	t.Logf("start %.0f ev/s at 1000 → %.0f ev/s at %d (after 60 s: %d, 300 s: %d)", first, rate, limits[len(limits)-1], limits[59], limits[299])
}

// When the broker is the bottleneck more in-flight records only add latency: probes are reverted.
func TestGradientDoesNotCreepWhenBrokerBound(t *testing.T) {
	b := autoBounds(64, 10000)
	m := &pipeModel{cpu: 200000, broker: 60000, base: 15 * time.Millisecond}
	limits, rate := run(gradient(b), m, 300, nil)
	assertWithin(t, limits, b)
	peak := 0
	for _, l := range limits[100:] {
		peak = max(peak, l)
	}
	if peak > 1600 {
		t.Fatalf("limit crept to %d although throughput is broker-bound", peak)
	}
	if rate < 0.95*m.broker {
		t.Fatalf("throughput %.0f fell below the broker capacity %.0f", rate, m.broker)
	}
}

func TestGradientHoldsWhenTheLimitDoesNotBind(t *testing.T) {
	b := autoBounds(64, 10000)
	m := &pipeModel{cpu: 80000, demand: 10000, base: 15 * time.Millisecond}
	limits, _ := run(gradient(b), m, 60, nil)
	for _, l := range limits {
		if l != 1000 {
			t.Fatalf("limit moved to %d without queueing", l)
		}
	}
}

func TestGradientShrinksOnLatencyRise(t *testing.T) {
	b := autoBounds(64, 10000)
	c := runner.NewGradientInFlight(b, 4000, runner.DefaultGradientTuning)
	m := &pipeModel{cpu: 80000, demand: 100000, base: 10 * time.Millisecond}
	run(c, m, 20, nil)
	grown := c.Current()
	m.base = 60 * time.Millisecond
	limits, _ := run(c, m, 3, nil)
	if limits[0] >= grown || limits[0] < grown/2 || limits[0] < b.Start {
		t.Fatalf("first window after a 6x latency rise should back off by at most half from %d, not below the start: %v", grown, limits)
	}
}

// Wave-2 S2 death spiral: latency blips while the limit binds must not throttle the route below the start
// limit (the old fixed default); only memory pressure goes to the floor.
func TestGradientLatencyNeverDropsBelowTheStart(t *testing.T) {
	b := autoBounds(64, 10000)
	c := gradient(b)
	m := &pipeModel{cpu: 80000, base: 15 * time.Millisecond}
	for i := 0; i < 200; i++ {
		m.base = 15 * time.Millisecond
		if i%4 == 3 {
			m.base = 90 * time.Millisecond
		}
		if l := c.Next(runner.InFlightSignal{Window: m.window(c.Current())}); l < b.Start {
			t.Fatalf("window %d: latency back-off went below the start limit: %d", i, l)
		}
	}
}

func TestGradientPressureAlwaysWins(t *testing.T) {
	b := autoBounds(100, 10000)
	m := &pipeModel{cpu: 200000, base: 15 * time.Millisecond}
	limits, _ := run(gradient(b), m, 12, func(i int) bool { return i >= 4 })
	for i := 5; i < len(limits); i++ {
		if limits[i] > limits[i-1] {
			t.Fatalf("limit grew under pressure: %v", limits)
		}
	}
	if limits[len(limits)-1] != b.Min {
		t.Fatalf("pressure should drive the limit to the floor %d: %v", b.Min, limits)
	}
}

func TestGradientRespectsCap(t *testing.T) {
	b := autoBounds(64, 2000)
	m := &pipeModel{cpu: 500000, base: 15 * time.Millisecond}
	limits, _ := run(gradient(b), m, 40, nil)
	assertWithin(t, limits, b)
	if limits[len(limits)-1] != 2000 {
		t.Fatalf("limit should reach the cap: %v", limits)
	}
}

func TestGradientHoldsOnIdleWindow(t *testing.T) {
	c := gradient(autoBounds(64, 10000))
	if got := c.Next(runner.InFlightSignal{Window: runner.InFlightWindow{Elapsed: time.Second, Acks: 3, Waited: 3, Acquired: 3}}); got != 1000 {
		t.Fatalf("too few acks must hold, got %d", got)
	}
}

func TestFixedControllerKeepsSize(t *testing.T) {
	b := (&config.Limits{InFlightMode: ptrMode(config.InFlightModeFixed)}).InFlight()
	c, err := runner.NewInFlightControllers(runner.DefaultGradientTuning).For(b, 0)
	if err != nil {
		t.Fatal(err)
	}
	limits, _ := run(c, &pipeModel{cpu: 200000, base: 15 * time.Millisecond}, 10, func(int) bool { return true })
	for _, l := range limits {
		if l != 1000 {
			t.Fatalf("fixed controller moved to %d", l)
		}
	}
}

func TestControllersRejectUnknownMode(t *testing.T) {
	if _, err := runner.NewInFlightControllers(runner.DefaultGradientTuning).For(config.InFlightBounds{Mode: "turbo"}, 0); err == nil {
		t.Fatal("unknown mode must fail")
	}
}

func ptrMode(m config.InFlightMode) *config.InFlightMode { return &m }

// S2 p1 shape (wave 2): a lightly used limit (demand far below it) sees short broker latency spikes. They are
// not caused by this route's in-flight records, so the limit must not ratchet down; when demand then steps up
// the route must reach its capacity within a few windows instead of being throttled at the floor.
func TestGradientIgnoresLatencySpikesOfAnUnusedLimit(t *testing.T) {
	b := autoBounds(64, 10000)
	c := gradient(b)
	m := &pipeModel{cpu: 80000, demand: 20000, base: 2500 * time.Microsecond}
	for i := 0; i < 120; i++ {
		m.base = 2500 * time.Microsecond
		if i%15 == 14 {
			m.base = 15 * time.Millisecond
		}
		if l := c.Next(runner.InFlightSignal{Window: m.window(c.Current())}); l < b.Start {
			t.Fatalf("window %d: an unused limit shrank to %d on a latency spike", i, l)
		}
	}
	m.base, m.demand = 2500*time.Microsecond, 60000
	_, rate := run(c, m, 5, nil)
	if rate < 0.95*m.demand {
		t.Fatalf("after the demand step the route ran at %.0f ev/s, want ≥ 95%% of %.0f", rate, m.demand)
	}
}
