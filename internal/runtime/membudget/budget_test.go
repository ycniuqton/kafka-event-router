package membudget

import (
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/twmb/franz-go/pkg/kgo"
)

// recordingPauser records the pause transitions applied to one route.
type recordingPauser struct {
	mu     sync.Mutex
	calls  []bool
	paused bool
}

func (p *recordingPauser) PauseFetch()  { p.set(true) }
func (p *recordingPauser) ResumeFetch() { p.set(false) }

func (p *recordingPauser) set(v bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, v)
	p.paused = v
}

func (p *recordingPauser) state() (bool, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.paused, len(p.calls)
}

func waitPaused(t *testing.T, p *recordingPauser, want bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got, _ := p.state(); got == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("pauser paused=%v, want %v", !want, want)
}

func TestBudgetPausesRouteOverItsShareOnlyWhenWorkerIsOverBudget(t *testing.T) {
	b := New(1000)
	pa, pb := &recordingPauser{}, &recordingPauser{}
	a, other := b.Open("a", pa), b.Open("b", pb)
	defer a.Close()
	defer other.Close()
	a.Charge(900)
	if a.Paused() {
		t.Fatal("over its share but the worker is under budget: must keep fetching")
	}
	other.Charge(100)
	if !a.Paused() || other.Paused() {
		t.Fatalf("worker at budget: a (900 ≥ share 500) must pause, b (100) must not; got a=%v b=%v", a.Paused(), other.Paused())
	}
	waitPaused(t, pa, true)
	if b.PausedRoutes() != 1 || b.Pauses() != 1 || b.Used() != 1000 {
		t.Fatalf("paused=%d pauses=%d used=%d", b.PausedRoutes(), b.Pauses(), b.Used())
	}
}

func TestBudgetResumesUnderLowWatermarkOrHalfShare(t *testing.T) {
	b := New(1000)
	pa := &recordingPauser{}
	a, other := b.Open("a", pa), b.Open("b", &recordingPauser{})
	defer other.Close()
	a.Charge(700)
	other.Charge(300)
	waitPaused(t, pa, true)
	a.Release(100)
	if !a.Paused() {
		t.Fatal("used 900 > low watermark 800 and a 600 ≥ half share: must stay paused (hysteresis)")
	}
	a.Release(100)
	if a.Paused() {
		t.Fatal("used 800 ≤ low watermark: must resume")
	}
	waitPaused(t, pa, false)
	a.Close()
	if b.Used() != 300 {
		t.Fatalf("close must return the route's usage, used=%d", b.Used())
	}
}

func TestBudgetResumesWhenARouteLeaves(t *testing.T) {
	b := New(1000)
	pa := &recordingPauser{}
	a, b2, c := b.Open("a", pa), b.Open("b", nil), b.Open("c", nil)
	defer a.Close()
	b2.Charge(400)
	c.Charge(250)
	a.Charge(350)
	b2.Charge(1)
	c.Charge(1)
	if !a.Paused() || !b2.Paused() || c.Paused() {
		t.Fatalf("share 333: a and b over it, c under; got a=%v b=%v c=%v", a.Paused(), b2.Paused(), c.Paused())
	}
	b2.Close()
	if a.Paused() {
		t.Fatal("b left: used 600 ≤ low watermark, a must resume")
	}
	waitPaused(t, pa, false)
	if b.PausedRoutes() != 0 {
		t.Fatalf("paused routes = %d", b.PausedRoutes())
	}
}

func TestBudgetSingleRouteNeverExceedsTheBudgetUnpaused(t *testing.T) {
	b := New(100)
	p := &recordingPauser{}
	a := b.Open("a", p)
	defer a.Close()
	a.Charge(100)
	waitPaused(t, p, true)
	a.Release(60)
	waitPaused(t, p, false)
	a.Charge(10)
	if a.Paused() {
		t.Fatal("50 of 100 must fetch")
	}
	if _, calls := p.state(); calls != 2 {
		t.Fatalf("pauser calls = %d, want pause then resume", calls)
	}
}

func TestBudgetUnlimitedNeverPausesAndClosedAccountIgnoresUpdates(t *testing.T) {
	b := OrUnlimited(nil)
	a := b.Open("a", &recordingPauser{})
	a.Charge(1 << 40)
	if a.Paused() || b.PausedRoutes() != 0 {
		t.Fatal("unlimited budget must not pause")
	}
	a.Close()
	a.Release(1 << 40)
	a.Charge(5)
	a.Close()
	if b.Used() != 0 {
		t.Fatalf("used after close = %d", b.Used())
	}
	if OrUnlimited(New(10)).Limit() != 10 {
		t.Fatal("OrUnlimited must keep a configured budget")
	}
}

func TestRecordCostCountsKeyValueHeadersAndOverhead(t *testing.T) {
	r := &kgo.Record{Key: []byte("k"), Value: []byte("vvv"), Headers: []kgo.RecordHeader{{Key: "h", Value: []byte("12")}}}
	if got, want := RecordCost(r), int64(1+3+1+2)+int64(RecordOverheadBytes); got != want {
		t.Fatalf("cost = %d, want %d", got, want)
	}
	if RecordsCost([]*kgo.Record{r, r}) != 2*RecordCost(r) {
		t.Fatal("batch cost must sum")
	}
}

func TestBudgetRegistersGauges(t *testing.T) {
	reg := prometheus.NewRegistry()
	b := New(1000)
	if err := b.Register(reg, "w1"); err != nil {
		t.Fatal(err)
	}
	a := b.Open("a", nil)
	defer a.Close()
	a.Charge(1000)
	if n, err := testutil.GatherAndCount(reg); err != nil || n != 4 {
		t.Fatalf("series = %d, err = %v", n, err)
	}
	if err := b.Register(reg, "w1"); err == nil {
		t.Fatal("registering twice must fail")
	}
}

func TestBudgetUnderPressureFollowsHighWatermarkAndPauses(t *testing.T) {
	b := New(1000)
	p := &recordingPauser{}
	a := b.Open("r1", p)
	defer a.Close()
	if b.UnderPressure() {
		t.Fatal("empty budget reports pressure")
	}
	a.Charge(1000)
	waitPaused(t, p, true)
	if !b.UnderPressure() {
		t.Fatal("budget at its high watermark reports no pressure")
	}
	a.Release(900)
	waitPaused(t, p, false)
	if b.UnderPressure() {
		t.Fatalf("budget under its low watermark with no paused route reports pressure (used %d)", b.Used())
	}
	if New(0).UnderPressure() {
		t.Fatal("unlimited budget reports pressure")
	}
}

func TestBudgetFetchLimitsScaleWithTheBudget(t *testing.T) {
	cases := map[int64]FetchLimits{
		256 << 20: {MaxBytes: int32(Watermark(FetchBudgetPercent).of(256 << 20)), PartitionMaxBytes: 1 << 20},
		10 << 20:  {MaxBytes: 1 << 20, PartitionMaxBytes: 1 << 20},
		0:         {MaxBytes: 50 << 20, PartitionMaxBytes: 1 << 20},
	}
	for limit, want := range cases {
		if got := New(limit).FetchLimits(); got != want {
			t.Fatalf("budget %d: %+v, want %+v", limit, got, want)
		}
	}
}

func TestBudgetDetachesOnceTenPercentIsHeld(t *testing.T) {
	b := New(1000)
	a := b.Open("r1", &recordingPauser{})
	defer a.Close()
	if a.Detaching() {
		t.Fatal("an empty budget must not detach (fast path keeps franz-go zero-copy records)")
	}
	a.Charge(100)
	if !a.Detaching() {
		t.Fatal("at 10 % of the budget polled records must be detached")
	}
	if New(0).Detaching() {
		t.Fatal("an unlimited budget never detaches")
	}
}
