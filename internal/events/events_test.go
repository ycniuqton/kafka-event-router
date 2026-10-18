package events_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/events"
	eventshttp "github.com/eventrouter/router/internal/events/http"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/runtime/domain"
)

var (
	teamA = uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	teamB = uuid.MustParse("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
	quiet = slog.New(slog.NewTextHandler(io.Discard, nil))
)

func user(team uuid.UUID) auth.Principal {
	return auth.Principal{Kind: auth.PrincipalUser, UserID: uuid.New(), Teams: []auth.TeamRole{{TeamID: team, Role: auth.RoleViewer}}}
}

func event(t *testing.T, typ events.PushEventType, team string) events.PushEvent {
	t.Helper()
	e, err := events.EventFactory{Now: time.Now}.New(typ, team, map[string]string{"routeId": "r1"})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestVisibilityPolicies(t *testing.T) {
	a := user(teamA)
	if !(events.OpenVisibility{}).Allows(a, teamB.String()) || (events.OpenVisibility{}).Allows(auth.Anonymous, "") {
		t.Fatal("open: every authenticated user sees every team")
	}
	team := events.TeamVisibility{}
	if !team.Allows(a, teamA.String()) || team.Allows(a, teamB.String()) || !team.Allows(a, "") {
		t.Fatal("team: own team and workspace-wide only")
	}
	if _, err := events.NewVisibilityManager().Select("nope"); err == nil {
		t.Fatal("unknown visibility")
	}
}

func TestHubDropsWhenClientLagsAndFlagsResync(t *testing.T) {
	hub := events.NewHub(events.OpenVisibility{}, quiet)
	c := hub.Register(user(teamA))
	for i := 0; i < int(events.DefaultClientBuffer)+5; i++ {
		_ = hub.Publish(context.Background(), event(t, events.TypeAlertFired, ""))
	}
	if len(c.Queue()) != int(events.DefaultClientBuffer) || !c.TakeDropped() || c.TakeDropped() {
		t.Fatal("full queue must drop and flag once")
	}
	hub.Unregister(c)
	if hub.Clients() != 0 {
		t.Fatal("unregister")
	}
}

// instance is one simulated API process: its own hub fed by a RedisBus on the shared Redis.
type instance struct {
	hub *events.Hub
	pub events.Publisher
	srv *httptest.Server
}

func newInstance(t *testing.T, ctx context.Context, rdb redis.UniversalClient, vis events.VisibilityPolicy) *instance {
	t.Helper()
	bus, err := events.NewBusManager(rdb, quiet).Select(events.BusRedis)
	if err != nil {
		t.Fatal(err)
	}
	hub := events.NewHub(vis, quiet)
	go hub.Run(ctx, bus)
	responder := api.NewResponder(problem.NewCatalog(), quiet)
	up := api.NewWSUpgrader(api.NewWSOriginPolicy(nil), time.Minute, responder)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			team := uuid.MustParse(req.URL.Query().Get("team"))
			next.ServeHTTP(w, req.WithContext(auth.WithPrincipal(req.Context(), user(team))))
		})
	})
	api.NewMounter(responder, nil, nil).Mount(r, up.Modules(&eventshttp.EventsHandler{Hub: hub, Visibility: vis.Kind(), Now: time.Now}))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return &instance{hub: hub, pub: events.FallbackPublisher{Bus: bus, Local: hub, Log: quiet}, srv: srv}
}

func dial(t *testing.T, ctx context.Context, in *instance, team uuid.UUID) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(in.srv.URL, "http") + string(eventshttp.PathEvents) + "?team=" + team.String()
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.CloseNow() })
	if f := read(t, ctx, c); f.Type != events.TypeHello {
		t.Fatalf("first frame %s", f.Type)
	}
	return c
}

func read(t *testing.T, ctx context.Context, c *websocket.Conn) events.Frame {
	t.Helper()
	rctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, data, err := c.Read(rctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var f events.Frame
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func waitClients(t *testing.T, hubs ...*events.Hub) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for _, h := range hubs {
		for h.Clients() == 0 && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
	}
	time.Sleep(100 * time.Millisecond)
}

// TestRedisBusFansOutAcrossInstancesWithTeamFiltering (P2-T-05): an event published on instance 1 reaches
// sockets on both instances; with team visibility, a socket of another team never receives it.
func TestRedisBusFansOutAcrossInstancesWithTeamFiltering(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	one, two := newInstance(t, ctx, rdb, events.TeamVisibility{}), newInstance(t, ctx, rdb, events.TeamVisibility{})
	aOnTwo, bOnTwo := dial(t, ctx, two, teamA), dial(t, ctx, two, teamB)
	aOnOne := dial(t, ctx, one, teamA)
	waitClients(t, one.hub, two.hub)
	if err := one.pub.Publish(ctx, event(t, events.TypeAlertFired, teamA.String())); err != nil {
		t.Fatal(err)
	}
	if f := read(t, ctx, aOnTwo); f.Type != events.TypeAlertFired || !strings.Contains(string(f.Payload), "r1") {
		t.Fatalf("instance two, team A: %+v", f)
	}
	if f := read(t, ctx, aOnOne); f.Type != events.TypeAlertFired {
		t.Fatalf("instance one, team A: %+v", f)
	}
	if err := one.pub.Publish(ctx, event(t, events.TypeRouteStateChanged, teamB.String())); err != nil {
		t.Fatal(err)
	}
	if f := read(t, ctx, bOnTwo); f.Type != events.TypeRouteStateChanged {
		t.Fatalf("team B must only see its own event first: %+v", f)
	}
}

func TestFallbackPublisherDeliversLocallyWhenRedisIsDown(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	bus, _ := events.NewBusManager(rdb, quiet).Select(events.BusRedis)
	hub := events.NewHub(events.OpenVisibility{}, quiet)
	c := hub.Register(user(teamA))
	mr.Close()
	pub := events.FallbackPublisher{Bus: bus, Local: hub, Log: quiet}
	if err := pub.Publish(context.Background(), event(t, events.TypeAlertResolved, "")); err != nil {
		t.Fatal(err)
	}
	if len(c.Queue()) != 1 {
		t.Fatal("local fallback")
	}
}

type snapReader struct{ snaps []events.RouteSnapshot }

func (s *snapReader) Snapshot(context.Context, string) (events.RouteSnapshot, bool, error) {
	cur := s.snaps[0]
	if len(s.snaps) > 1 {
		s.snaps = s.snaps[1:]
	}
	return cur, true, nil
}

type memOut struct{ got []events.PushEvent }

func (m *memOut) Publish(_ context.Context, e events.PushEvent) error {
	m.got = append(m.got, e)
	return nil
}

func TestRuntimeChangeSourceEmitsOnlyChanges(t *testing.T) {
	reader := &snapReader{snaps: []events.RouteSnapshot{
		{TeamID: teamA.String(), Status: routes.StatusRunning, DeploymentID: "d1", Deployment: "rolling_out"},
		{TeamID: teamA.String(), Status: routes.StatusRunning, DeploymentID: "d1", Deployment: "rolling_out"},
		{TeamID: teamA.String(), Status: routes.StatusRunning, DeploymentID: "d1", Deployment: "active"},
		{TeamID: teamA.String(), Status: routes.StatusPaused, DeploymentID: "d1", Deployment: "active"},
	}}
	out := &memOut{}
	src := &events.RuntimeChangeSource{Reader: reader, Out: out, Factory: events.EventFactory{Now: time.Now}, Log: quiet}
	for i := 0; i < 4; i++ {
		if err := src.Handle(context.Background(), domain.ChangeHint{RouteID: "r1", Kind: domain.ChangeRoute}); err != nil {
			t.Fatal(err)
		}
	}
	var types []string
	for _, e := range out.got {
		types = append(types, string(e.Type))
		if e.TeamID != teamA.String() {
			t.Fatal("team")
		}
	}
	want := "route.state_changed,deployment.status_changed,deployment.status_changed,route.state_changed"
	if strings.Join(types, ",") != want {
		t.Fatalf("got %v", types)
	}
}

func TestConnectionHealthPublisher(t *testing.T) {
	out := &memOut{}
	p := events.ConnectionHealthPublisher{Out: out, Factory: events.EventFactory{Now: time.Now}}
	err := p.Publish(context.Background(), connections.HealthEvent{ConnectionID: "c1", TeamID: teamA.String(),
		Previous: connections.HealthHealthy, Current: connections.HealthUnreachable})
	if err != nil || len(out.got) != 1 || out.got[0].Type != events.TypeConnectionHealth || !strings.Contains(string(out.got[0].Payload), `"current":"unreachable"`) {
		t.Fatalf("health event: %v %+v", err, out.got)
	}
}
