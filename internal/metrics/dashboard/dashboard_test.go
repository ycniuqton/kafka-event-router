package dashboard

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/metrics"
	"github.com/eventrouter/router/internal/metrics/query"
	"github.com/eventrouter/router/internal/routes"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

type fakeRoutes struct {
	rows []RouteRow
	err  error
}

func (f fakeRoutes) LiveRoutes(context.Context) ([]RouteRow, error) { return f.rows, f.err }

type fakeConnections struct {
	rows []ConnectionRow
	err  error
}

func (f fakeConnections) Unhealthy(context.Context) ([]ConnectionRow, error) { return f.rows, f.err }

// fakeQuerier answers instants by expression; unknown expressions return an empty vector.
type fakeQuerier struct {
	instants map[query.Expr]query.Vector
	ranges   map[query.Expr][]query.Series
	err      error
}

func (f fakeQuerier) Kind() query.BackendKind { return query.BackendMock }
func (f fakeQuerier) Instant(_ context.Context, e query.Expr) (query.Vector, error) {
	return f.instants[e], f.err
}
func (f fakeQuerier) Range(_ context.Context, e query.Expr, _ query.Window) ([]query.Series, error) {
	return f.ranges[e], f.err
}

func byRoute(values map[string]float64) query.Vector {
	var v query.Vector
	for id, n := range values {
		v = append(v, query.Sample{Labels: query.Labels{"route_id": id}, Value: n})
	}
	return v
}

var liveRoutes = []RouteRow{
	{ID: "r1", Name: "orders", Status: routes.StatusRunning},
	{ID: "r2", Name: "payments", Status: routes.StatusError, ErrorMessage: "boom"},
	{ID: "r3", Name: "audit", Status: routes.StatusPaused},
	{ID: "r4", Name: "draft", Status: routes.StatusDraft},
	{ID: "r5", Name: "quiet", Status: routes.StatusRunning},
}

func TestRouteErrorDetector(t *testing.T) {
	items, _ := RouteErrorDetector{}.Detect(context.Background(), Snapshot{Routes: liveRoutes})
	if len(items) != 1 || items[0].Action.Intent.RouteID != "r2" || items[0].Tone != metrics.ToneErr ||
		items[0].Action.Intent.Type != IntentRouteDetail || items[0].Action.Intent.Tab != TabDeadLetters {
		t.Fatalf("unexpected %+v", items)
	}
}

func TestConnectionHealthDetector(t *testing.T) {
	d := ConnectionHealthDetector{Connections: fakeConnections{rows: []ConnectionRow{{ID: "c1", Name: "prod", Health: connections.HealthUnreachable, AffectedRoutes: 3}}}}
	items, err := d.Detect(context.Background(), Snapshot{})
	if err != nil || len(items) != 1 || items[0].Action.Intent.ConnectionID != "c1" || items[0].Action.Label != LabelTest {
		t.Fatalf("unexpected %+v %v", items, err)
	}
}

func TestLagGrowthDetector(t *testing.T) {
	cat, th := query.ExprCatalog{}, metrics.DefaultThresholds
	q := fakeQuerier{instants: map[query.Expr]query.Vector{
		cat.Build(query.QueryLagByRoute, query.Params{}):                               byRoute(map[string]float64{"r1": 30000, "r5": 3000, "r3": 90000}),
		cat.Build(query.QueryLagDerivByRoute, query.Params{Range: th.LagGrowthWindow}): byRoute(map[string]float64{"r1": 2, "r5": -1, "r3": 5}),
	}}
	items, err := LagGrowthDetector{Querier: q}.Detect(context.Background(), Snapshot{Routes: liveRoutes, Thresholds: th})
	if err != nil || len(items) != 1 || items[0].Action.Intent.RouteID != "r1" || items[0].Tone != metrics.ToneErr ||
		items[0].Action.Intent.Type != IntentTogglePause {
		t.Fatalf("unexpected %+v %v", items, err)
	}
}

func TestZeroMatchDetector(t *testing.T) {
	cat, th := query.ExprCatalog{}, metrics.DefaultThresholds
	p := query.Params{Range: th.ZeroMatchWindow}
	q := fakeQuerier{instants: map[query.Expr]query.Vector{
		cat.Build(query.QueryConsumedByRoute, p): byRoute(map[string]float64{"r1": 100, "r5": 50, "r3": 10}),
		cat.Build(query.QueryFilteredByRoute, p): byRoute(map[string]float64{"r1": 20, "r5": 50, "r3": 10}),
	}}
	items, err := ZeroMatchDetector{Querier: q}.Detect(context.Background(), Snapshot{Routes: liveRoutes, Thresholds: th})
	if err != nil || len(items) != 1 || items[0].Action.Intent.RouteID != "r5" || items[0].Action.Intent.Step != StepFilter {
		t.Fatalf("unexpected %+v %v", items, err)
	}
}

func TestAttentionManagerOmitsFailingDetectorAndSorts(t *testing.T) {
	failing := LagGrowthDetector{Querier: fakeQuerier{err: query.Unavailable(errors.New("down"))}}
	conns := ConnectionHealthDetector{Connections: fakeConnections{rows: []ConnectionRow{{ID: "c1", Name: "a-conn", Health: connections.HealthDegraded}}}}
	m, err := NewAttentionManager(fakeRoutes{rows: liveRoutes}, metrics.DefaultThresholds, quiet,
		[]metrics.AttentionKind{metrics.AttentionLagGrowing, metrics.AttentionRouteError, metrics.AttentionConnectionHealth},
		failing, RouteErrorDetector{}, conns)
	if err != nil {
		t.Fatal(err)
	}
	items, err := m.Detect(context.Background())
	if err != nil || len(items) != 2 {
		t.Fatalf("items %+v err %v", items, err)
	}
	if items[0].Kind != metrics.AttentionConnectionHealth || items[1].Kind != metrics.AttentionRouteError {
		t.Fatalf("order %+v", items)
	}
	if _, err := NewAttentionManager(fakeRoutes{}, metrics.DefaultThresholds, quiet, []metrics.AttentionKind{"nope"}); err == nil {
		t.Fatal("unknown detector accepted")
	}
}

func TestAttentionMergerDedupes(t *testing.T) {
	a := AttentionItem{ID: "x", Tone: metrics.ToneWarn, Title: "b"}
	b := AttentionItem{ID: "y", Tone: metrics.ToneWarn, Title: "a"}
	out := AttentionMerger{}.Merge([][]AttentionItem{{a, b}, {a}})
	if len(out) != 2 || out[0].ID != "y" {
		t.Fatalf("merge %+v", out)
	}
}

func newSummary(q query.Querier, r RouteSnapshotReader) *SummaryService {
	return &SummaryService{Counter: RouteStateCounter{Routes: r}, Querier: q, Thresholds: metrics.DefaultThresholds,
		Now: func() time.Time { return time.Unix(1700000000, 0) }, Log: quiet}
}

func TestSummaryServiceHealthy(t *testing.T) {
	cat := query.ExprCatalog{}
	q := fakeQuerier{instants: map[query.Expr]query.Vector{
		cat.Build(query.QueryProducedIncrease, query.Params{Range: time.Hour}): {{Value: 1200}},
		cat.Build(query.QueryLagTotal, query.Params{}):                         {{Value: 50000}},
		cat.Build(query.QueryDLQIncrease, query.Params{Range: 24 * time.Hour}): {{Value: 7}},
	}}
	s, err := newSummary(q, fakeRoutes{rows: liveRoutes}).Build(context.Background())
	if err != nil || !s.MetricsAvailable {
		t.Fatalf("summary %+v %v", s, err)
	}
	if s.Routes != (StateCounts{Running: 2, Paused: 1, Error: 1, Draft: 1}) {
		t.Fatalf("counts %+v", s.Routes)
	}
	if s.EventsLastHour.Total != 1200 || s.TotalLag.Tone != metrics.ToneErr || s.DLQ24h.Value != 7 || s.EventsLastHour.Spark == nil {
		t.Fatalf("cards %+v %+v %+v", s.EventsLastHour, s.TotalLag, s.DLQ24h)
	}
}

func TestSummaryServiceDegradesWhenMetricsAreDown(t *testing.T) {
	s, err := newSummary(fakeQuerier{err: query.Unavailable(errors.New("timeout"))}, fakeRoutes{rows: liveRoutes}).Build(context.Background())
	if err != nil || s.MetricsAvailable || s.EventsLastHour != nil || s.TotalLag != nil || s.DLQ24h != nil || s.Routes.Running != 2 {
		t.Fatalf("degraded summary %+v %v", s, err)
	}
	if _, err := newSummary(fakeQuerier{}, fakeRoutes{err: errors.New("pg down")}).Build(context.Background()); err == nil {
		t.Fatal("postgres failure hidden")
	}
}

type fakeCounts struct{ c OnboardingCounts }

func (f fakeCounts) OnboardingCounts(context.Context) (OnboardingCounts, error) { return f.c, nil }

func TestOnboardingStepDerivation(t *testing.T) {
	o, _ := OnboardingService{Counts: fakeCounts{OnboardingCounts{Connections: 1, Routes: 2}}}.Build(context.Background())
	if o.AllDone || !o.Steps[0].Done || !o.Steps[1].Done || o.Steps[2].Done || o.Steps[2].ID != StepDeploy {
		t.Fatalf("partial %+v", o)
	}
	o, _ = OnboardingService{Counts: fakeCounts{OnboardingCounts{Connections: 1, Routes: 1, ActiveDeployments: 1}}}.Build(context.Background())
	if !o.AllDone {
		t.Fatalf("all done %+v", o)
	}
}

type fakeAudit struct{ got audit.Filter }

func (f *fakeAudit) Query(_ context.Context, flt audit.Filter, _ audit.Page) (audit.QueryResult, error) {
	f.got = flt
	return audit.QueryResult{Items: []audit.LogEntry{
		{ID: 9, Action: routes.ActionRouteRolledBack, ObjectType: audit.ObjectRouteVersion, ObjectID: "v3", ObjectLabel: "orders v3", Actor: audit.LogActor{Name: "Ana"}, Diff: []byte(`{"routeId":"r1","no":3}`)},
		{ID: 8, Action: routes.ActionRoutePaused, ObjectType: audit.ObjectRoute, ObjectID: "r2"},
		{ID: 7, Action: routes.ActionRouteDeployed, ObjectType: audit.ObjectRouteVersion, ObjectID: "v0", Diff: []byte(`{"no":1}`)},
	}}, nil
}

func TestActivityServiceMapsRouteChanges(t *testing.T) {
	a := &fakeAudit{}
	page, err := ActivityService{Audit: a}.List(context.Background(), 10, nil)
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("page %+v %v", page, err)
	}
	first, second := page.Items[0], page.Items[1]
	if first.ID != "9" || first.Verb != VerbRolledBack || first.RouteID != "r1" || first.RouteName != "orders v3" || first.Actor != "Ana" || second.RouteName != "r2" {
		t.Fatalf("items %+v", page.Items)
	}
	if a.got.ObjectType != nil || len(a.got.Actions) != len(ActivityVerbs) {
		t.Fatalf("filter %+v", a.got)
	}
}
