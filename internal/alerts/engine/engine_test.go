package engine

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/alerts"
	"github.com/eventrouter/router/internal/alerts/alertstest"
	"github.com/eventrouter/router/internal/alerts/evaluator"
	"github.com/eventrouter/router/internal/events"
	"github.com/eventrouter/router/internal/metrics/query"
)

type recordingPublisher struct {
	mu     sync.Mutex
	events []events.PushEvent
}

func (p *recordingPublisher) Publish(_ context.Context, e events.PushEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, e)
	return nil
}

func (p *recordingPublisher) types() []events.PushEventType {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := []events.PushEventType{}
	for _, e := range p.events {
		out = append(out, e.Type)
	}
	return out
}

type fixture struct {
	clock   *alertstest.Clock
	events  *alertstest.Events
	out     *recordingPublisher
	tracker *StateTracker
	rule    alerts.AlertRule
	route   alerts.RouteInfo
}

func newFixture() *fixture {
	clock := alertstest.NewClock(time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC))
	f := &fixture{clock: clock, events: &alertstest.Events{}, out: &recordingPublisher{}}
	f.tracker = &StateTracker{Tx: alertstest.NoTx{}, Events: f.events, Out: f.out, Factory: events.EventFactory{Now: clock.Now}, Now: clock.Now}
	f.route = alerts.RouteInfo{ID: uuid.New(), Name: "orders", TeamID: uuid.New(), Running: true}
	f.rule = alerts.AlertRule{ID: uuid.New(), RouteID: f.route.ID, Name: "lag", Kind: alerts.KindLagAbove, Threshold: 10, Window: 5}
	return f
}

func (f *fixture) observe(t *testing.T, breached bool) Transition {
	t.Helper()
	v := 42.0
	tr, err := f.tracker.Apply(context.Background(), Observation{Rule: f.rule, Route: f.route, Value: &v, Breached: breached})
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func TestStateTrackerFireDedupResolve(t *testing.T) {
	f := newFixture()
	steps := []struct {
		advance  time.Duration
		breached bool
		want     Transition
	}{
		{0, false, TransitionNone},
		{30 * time.Second, true, TransitionFired},
		{30 * time.Second, true, TransitionNone},
		{30 * time.Second, false, TransitionNone},
		{4 * time.Minute, false, TransitionNone},
		{time.Minute, false, TransitionResolved},
		{30 * time.Second, false, TransitionNone},
	}
	for i, s := range steps {
		f.clock.Advance(s.advance)
		if got := f.observe(t, s.breached); got != s.want {
			t.Fatalf("step %d: got %s want %s", i, got, s.want)
		}
	}
	if len(f.events.Rows) != 1 || f.events.Rows[0].State != alerts.StateResolved {
		t.Fatalf("history: %+v", f.events.Rows)
	}
	if got := f.out.types(); len(got) != 2 || got[0] != events.TypeAlertFired || got[1] != events.TypeAlertResolved {
		t.Fatalf("push events: %v", got)
	}
	if f.out.events[0].TeamID != f.route.TeamID.String() {
		t.Fatal("push event must carry the route team")
	}
}

func TestStateTrackerFlappingNeverResolvesEarly(t *testing.T) {
	f := newFixture()
	f.observe(t, true)
	for i := 0; i < 20; i++ {
		f.clock.Advance(2 * time.Minute)
		if got := f.observe(t, i%2 == 0); got != TransitionNone {
			t.Fatalf("flap %d: %s", i, got)
		}
	}
	if len(f.events.Rows) != 1 || f.events.Rows[0].State != alerts.StateFiring {
		t.Fatalf("flapping must keep one firing event: %+v", f.events.Rows)
	}
	f.clock.Advance(time.Minute)
	f.observe(t, false)
	f.clock.Advance(5 * time.Minute)
	if got := f.observe(t, false); got != TransitionResolved {
		t.Fatalf("clean window: %s", got)
	}
	f.clock.Advance(time.Minute)
	if got := f.observe(t, true); got != TransitionFired {
		t.Fatalf("refire: %s", got)
	}
	if len(f.events.Rows) != 2 {
		t.Fatalf("refire creates a new event: %d", len(f.events.Rows))
	}
}

type stubQuerier struct {
	vec query.Vector
	err error
}

func (stubQuerier) Kind() query.BackendKind { return query.BackendMock }
func (q stubQuerier) Instant(context.Context, query.Expr) (query.Vector, error) {
	return q.vec, q.err
}
func (stubQuerier) Range(context.Context, query.Expr, query.Window) ([]query.Series, error) {
	return nil, nil
}

type leader bool

func (l leader) IsLeader() bool { return bool(l) }

type counter struct{ n int }

func (c *counter) EvalError() { c.n++ }

type dispatcher struct{ calls int }

func (d *dispatcher) DispatchPending(context.Context) error { d.calls++; return nil }

func TestJobTickSkipsFailingQueriesAndArchivedRoutes(t *testing.T) {
	f := newFixture()
	archived := alerts.RouteInfo{ID: uuid.New(), Archived: true}
	rules := &alertstest.Rules{}
	f.rule.Enabled = true
	live, _ := rules.Insert(context.Background(), f.rule)
	_, _ = rules.Insert(context.Background(), alerts.AlertRule{RouteID: archived.ID, Kind: alerts.KindLagAbove, Enabled: true, Window: 1})
	errs, d := &counter{}, &dispatcher{}
	job := &AlertEvaluatorJob{Leader: leader(true), Rules: rules, Routes: alertstest.Routes{f.route.ID: f.route, archived.ID: archived},
		Evaluators: evaluator.NewDefaultEvaluatorManager(), Querier: stubQuerier{err: errors.New("down")}, Tracker: f.tracker,
		Dispatcher: d, Errors: errs, Interval: time.Second, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if err := job.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if errs.n != 1 || d.calls != 1 || len(f.events.Rows) != 0 {
		t.Fatalf("errors=%d dispatch=%d events=%d", errs.n, d.calls, len(f.events.Rows))
	}
	job.Querier = stubQuerier{vec: query.Vector{{Value: 99}}}
	if err := job.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.events.Rows) != 1 || f.events.Rows[0].RuleID != live.ID || *f.events.Rows[0].Value != 99 {
		t.Fatalf("lag 99 > 10 must fire once: %+v", f.events.Rows)
	}
}
