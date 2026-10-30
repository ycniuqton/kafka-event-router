package lookup_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/lookup"
)

func TestCircuitBreakerCycle(t *testing.T) {
	now := time.Unix(0, 0)
	var states []lookup.BreakerState
	b := lookup.NewCircuitBreaker(2, time.Second, func() time.Time { return now }, func(s lookup.BreakerState) { states = append(states, s) })
	b.Record(true)
	if !b.Allow() || b.State() != lookup.BreakerClosed {
		t.Fatal("one failure keeps the breaker closed")
	}
	b.Record(true)
	if b.Allow() || b.State() != lookup.BreakerOpen {
		t.Fatal("threshold reached: open")
	}
	now = now.Add(2 * time.Second)
	if !b.Allow() || b.Allow() || b.State() != lookup.BreakerHalfOpen {
		t.Fatal("after openFor exactly one probe passes")
	}
	b.Record(true)
	if b.State() != lookup.BreakerOpen {
		t.Fatal("failed probe re-opens")
	}
	now = now.Add(2 * time.Second)
	_ = b.Allow()
	b.Record(false)
	if b.State() != lookup.BreakerClosed || len(states) != 5 {
		t.Fatalf("successful probe closes; states %v", states)
	}
	off := lookup.NewCircuitBreaker(0, time.Second, nil, nil)
	off.Record(true)
	off.Record(true)
	if !off.Allow() {
		t.Fatal("threshold 0 disables the breaker")
	}
}

func TestLimiterConcurrencyAndRate(t *testing.T) {
	l := lookup.NewLimiter(lookup.Limits{MaxConcurrency: 1})
	release, err := l.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err = l.Acquire(ctx)
	var se *ctxhelpers.SourceError
	if !errors.As(err, &se) || !se.Transient || !errors.Is(err, lookup.ErrConcurrency) {
		t.Fatalf("full limiter: %v", err)
	}
	release()
	if r, err := l.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	} else {
		r()
	}
	rl := lookup.NewLimiter(lookup.Limits{RatePerSec: 1, Burst: 1})
	_, _ = rl.Acquire(context.Background())
	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel2()
	if _, err := rl.Acquire(ctx2); !errors.As(err, &se) || !se.Transient {
		t.Fatalf("rate limited: %v", err)
	}
}

// scriptedClient answers with a fixed outcome and counts calls.
type scriptedClient struct {
	mu     sync.Mutex
	err    error
	calls  int
	closed bool
}

func (c *scriptedClient) Fetch(context.Context, ctxhelpers.SourceRequest) (ctxhelpers.SourceAnswer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.err != nil {
		return ctxhelpers.SourceAnswer{}, c.err
	}
	return ctxhelpers.SourceAnswer{Found: true, Value: "v"}, nil
}
func (c *scriptedClient) Probe(context.Context) error { return c.err }
func (c *scriptedClient) Close()                      { c.closed = true }

type scriptedFactory struct{ clients []*scriptedClient }

func (f *scriptedFactory) Kind() lookup.SourceKind { return lookup.KindHTTP }
func (f *scriptedFactory) New(lookup.SourceSpec) (lookup.Client, error) {
	c := &scriptedClient{}
	f.clients = append(f.clients, c)
	return c, nil
}

type specMap struct {
	mu    sync.Mutex
	specs map[string]lookup.SourceSpec
	err   error
	calls int
}

func (p *specMap) Spec(_ context.Context, id string) (lookup.SourceSpec, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.err != nil {
		return lookup.SourceSpec{}, p.err
	}
	s, ok := p.specs[id]
	if !ok {
		return lookup.SourceSpec{}, lookup.ErrSourceNotFound
	}
	return s, nil
}

func TestGatewayResolvesCachesAndRefreshes(t *testing.T) {
	now := time.Unix(0, 0)
	specs := &specMap{specs: map[string]lookup.SourceSpec{"s1": {ID: "s1", Kind: lookup.KindHTTP, Version: 1, Limits: lookup.Limits{BreakerFailures: 1, BreakerOpenMs: 60000}}}}
	factory := &scriptedFactory{}
	reg := prometheus.NewRegistry()
	gw := lookup.NewGateway(lookup.GatewayDeps{Specs: specs, Factories: lookup.NewFactoryRegistry(factory), Observer: lookup.NewPromObserver(reg), Now: func() time.Time { return now }})
	defer gw.Close()
	call := ctxhelpers.SourceCall{SourceID: "s1", Kind: ctxhelpers.RequestHTTP}
	for range 3 {
		if a, err := gw.Fetch(context.Background(), call); err != nil || !a.Found || a.Mode != ctxhelpers.AnswerLive {
			t.Fatalf("fetch: %+v %v", a, err)
		}
	}
	if specs.calls != 1 || len(factory.clients) != 1 {
		t.Fatalf("spec calls=%d clients=%d", specs.calls, len(factory.clients))
	}
	now = now.Add(2 * time.Minute)
	specs.err = errors.New("control plane down")
	if _, err := gw.Fetch(context.Background(), call); err != nil {
		t.Fatalf("stale client must be kept when refresh fails: %v", err)
	}
	specs.err = nil
	specs.specs["s1"] = lookup.SourceSpec{ID: "s1", Kind: lookup.KindHTTP, Version: 2}
	_, _ = gw.Fetch(context.Background(), call)
	if len(factory.clients) != 2 {
		t.Fatal("a new spec version builds a new client")
	}
	if n, err := testutil.GatherAndCount(reg, "router_lookup_requests_total"); err != nil || n != 1 {
		t.Fatalf("requests series=%d err=%v", n, err)
	}
}

func TestGatewayErrorsAndMetrics(t *testing.T) {
	specs := &specMap{specs: map[string]lookup.SourceSpec{"s1": {ID: "s1", Kind: lookup.KindHTTP, Version: 1, Limits: lookup.Limits{BreakerFailures: 1, BreakerOpenMs: 60000}}}}
	factory := &scriptedFactory{}
	reg := prometheus.NewRegistry()
	obs := lookup.NewPromObserver(reg)
	gw := lookup.NewGateway(lookup.GatewayDeps{Specs: specs, Factories: lookup.NewFactoryRegistry(factory), Observer: obs})
	var se *ctxhelpers.SourceError
	if _, err := gw.Fetch(context.Background(), ctxhelpers.SourceCall{SourceID: "s1", Kind: ctxhelpers.RequestRedis}); !errors.As(err, &se) || se.Transient {
		t.Fatalf("kind mismatch must be permanent: %v", err)
	}
	specs.specs["owned"] = lookup.SourceSpec{ID: "owned", TeamID: "team-a", Kind: lookup.KindHTTP, Version: 1}
	if _, err := gw.Fetch(context.Background(), ctxhelpers.SourceCall{SourceID: "owned", TeamID: "team-b", Kind: ctxhelpers.RequestHTTP}); !errors.As(err, &se) || se.Transient || se.Reason != string(lookup.ReasonForbidden) {
		t.Fatalf("another team's source must be refused: %v", err)
	}
	if _, err := gw.Fetch(context.Background(), ctxhelpers.SourceCall{SourceID: "nope", Kind: ctxhelpers.RequestHTTP}); !errors.As(err, &se) || se.Transient {
		t.Fatalf("unknown source must be permanent: %v", err)
	}
	call := ctxhelpers.SourceCall{SourceID: "s1", Kind: ctxhelpers.RequestHTTP}
	_, _ = gw.Fetch(context.Background(), call)
	factory.clients[0].err = &ctxhelpers.SourceError{Transient: true, Reason: "503"}
	_, _ = gw.Fetch(context.Background(), call)
	if _, err := gw.Fetch(context.Background(), call); !errors.Is(err, lookup.ErrCircuitOpen) {
		t.Fatalf("breaker must open after 1 transient failure: %v", err)
	}
	if factory.clients[0].calls != 2 {
		t.Fatalf("open breaker must not call the source, calls=%d", factory.clients[0].calls)
	}
	gw.Observe("s1", ctxhelpers.EventCacheHit)
	if n, err := testutil.GatherAndCount(reg, "router_lookup_requests_total", "router_lookup_cache_total", "router_lookup_breaker_state"); err != nil || n != 5 {
		t.Fatalf("series=%d err=%v (hit, error, circuit_open, cache hit, breaker)", n, err)
	}
}

func TestPrometheusMetricNames(t *testing.T) {
	reg := prometheus.NewRegistry()
	obs := lookup.NewPromObserver(reg)
	obs.Request("s1", lookup.ResultHit, time.Millisecond)
	obs.Cache("s1", ctxhelpers.EventCacheHit)
	obs.Cache("s1", ctxhelpers.EventCacheMiss)
	obs.Breaker("s1", lookup.BreakerOpen)
	n, err := testutil.GatherAndCount(reg, "router_lookup_requests_total", "router_lookup_duration_seconds", "router_lookup_cache_total", "router_lookup_breaker_state")
	if err != nil || n != 5 {
		t.Fatalf("series=%d err=%v", n, err)
	}
}

func TestPreviewGatewayMocksAndCap(t *testing.T) {
	live := &slowGateway{delay: 50 * time.Millisecond}
	gw := lookup.NewPreviewGateway(live, map[string]lookup.MockAnswer{"crm": {Found: true, Value: map[string]any{"tier": "gold"}}})
	gw.Cap = 5 * time.Millisecond
	a, err := gw.Fetch(context.Background(), ctxhelpers.SourceCall{StepID: "crm"})
	if err != nil || a.Mode != ctxhelpers.AnswerMock || !a.Found || live.calls != 0 {
		t.Fatalf("mock: %+v %v", a, err)
	}
	if _, err := gw.Fetch(context.Background(), ctxhelpers.SourceCall{StepID: "other"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("live call must be capped: %v", err)
	}
}

type slowGateway struct {
	delay time.Duration
	calls int
}

func (g *slowGateway) Fetch(ctx context.Context, _ ctxhelpers.SourceCall) (ctxhelpers.SourceAnswer, error) {
	g.calls++
	select {
	case <-time.After(g.delay):
		return ctxhelpers.SourceAnswer{Found: true}, nil
	case <-ctx.Done():
		return ctxhelpers.SourceAnswer{}, ctx.Err()
	}
}
func (g *slowGateway) Observe(string, ctxhelpers.SourceEvent) {}

func TestTesterChecklist(t *testing.T) {
	factory := &scriptedFactory{}
	tester := lookup.Tester{Factories: lookup.NewFactoryRegistry(&probeFactory{err: &lookup.ProbeError{Stage: lookup.StageAuthenticated, Hint: "h", Cause: errors.New("401")}})}
	r := tester.Run(context.Background(), lookup.SourceSpec{Kind: lookup.KindHTTP})
	states := []lookup.TestState{r.Steps[0].State, r.Steps[1].State, r.Steps[2].State}
	if r.OK || states[0] != lookup.TestPassed || states[1] != lookup.TestFailed || states[2] != lookup.TestSkipped || r.Steps[1].Hint != "h" || r.FailedStage() != lookup.StageAuthenticated {
		t.Fatalf("report %+v", r)
	}
	ok := lookup.Tester{Factories: lookup.NewFactoryRegistry(factory)}.Run(context.Background(), lookup.SourceSpec{Kind: lookup.KindHTTP})
	if !ok.OK || len(ok.Steps) != 3 || !factory.clients[0].closed {
		t.Fatalf("ok report %+v", ok)
	}
	unknown := lookup.Tester{Factories: lookup.NewFactoryRegistry()}.Run(context.Background(), lookup.SourceSpec{Kind: "ftp"})
	if unknown.OK || unknown.FailedStage() != lookup.StageReachable {
		t.Fatalf("unknown kind %+v", unknown)
	}
}

type probeFactory struct{ err error }

func (f *probeFactory) Kind() lookup.SourceKind { return lookup.KindHTTP }
func (f *probeFactory) New(lookup.SourceSpec) (lookup.Client, error) {
	return &scriptedClient{err: f.err}, nil
}
