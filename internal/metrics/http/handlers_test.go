package metricshttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/metrics"
	"github.com/eventrouter/router/internal/metrics/dashboard"
	"github.com/eventrouter/router/internal/metrics/flowmap"
	"github.com/eventrouter/router/internal/metrics/overview"
	"github.com/eventrouter/router/internal/metrics/query"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/api/contract"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/store"
)

const routeID = "6f1c1f7e-1111-4c1a-9a52-1f0f7bb2c001"

type stubs struct {
	overviewErr error
	gotLimit    int
}

func (s *stubs) Build(context.Context) (dashboard.Summary, error) {
	return dashboard.Summary{Routes: dashboard.StateCounts{Running: 1}}, nil
}
func (s *stubs) Detect(context.Context) ([]dashboard.AttentionItem, error) {
	return []dashboard.AttentionItem{{ID: "route_error:r", Action: dashboard.Action{Label: dashboard.LabelOpen,
		Intent: dashboard.ActionIntent{Type: dashboard.IntentRouteDetail, RouteID: "r", Tab: dashboard.TabDeadLetters}}}}, nil
}
func (s *stubs) List(_ context.Context, limit int, _ *store.Cursor) (dashboard.ActivityPage, error) {
	s.gotLimit = limit
	return dashboard.ActivityPage{Items: []dashboard.ActivityItem{{ID: "1", Verb: dashboard.VerbDeployed, At: time.Unix(0, 0)}}}, nil
}
func (s *stubs) Load(_ context.Context, _ routes.RouteID, r metrics.TimeRange) (overview.RouteMetrics, error) {
	return overview.RouteMetrics{Range: r}, s.overviewErr
}

type onboardingStub struct{}

func (onboardingStub) Build(context.Context) (dashboard.Onboarding, error) {
	return dashboard.Onboarding{AllDone: true}, nil
}

func server(t *testing.T, s *stubs) http.Handler {
	t.Helper()
	responder := api.NewResponder(problem.NewCatalog(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	mod := NewModule(Deps{Summary: s, Attention: s, Activity: s, Onboarding: onboardingStub{}, Overview: s, Responder: responder,
		Pages: api.NewPageParser(store.NewCursorCodec("test-secret"))})
	r := chi.NewRouter()
	api.NewMounter(responder, nil, nil).Mount(r, mod)
	return r
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestDashboardEndpoints(t *testing.T) {
	s := &stubs{}
	h := server(t, s)
	for path, want := range map[string]string{
		string(PathSummary):    `"running":1`,
		string(PathAttention):  `"intent":{"type":"routeDetail","routeId":"r","tab":"dead"}`,
		string(PathActivity):   `"next_cursor":null`,
		string(PathOnboarding): `"allDone":true`,
	} {
		rec := get(h, path)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%s: %d %s", path, rec.Code, rec.Body)
		}
	}
	if s.gotLimit != int(dashboard.DefaultActivityLimit) {
		t.Fatalf("default activity limit %d", s.gotLimit)
	}
	get(h, string(PathActivity)+"?limit=3")
	if s.gotLimit != 3 {
		t.Fatalf("limit %d", s.gotLimit)
	}
}

func TestRouteMetricsRangeAndErrors(t *testing.T) {
	s := &stubs{}
	h := server(t, s)
	base := strings.Replace(string(PathRouteMetrics), "{id}", routeID, 1)
	rec := get(h, base+"?range=24h")
	var body overview.RouteMetrics
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusOK || body.Range != metrics.Range24h {
		t.Fatalf("ok: %d %s", rec.Code, rec.Body)
	}
	if rec := get(h, base+"?range=2w"); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"path":"/range"`) {
		t.Fatalf("bad range: %d %s", rec.Code, rec.Body)
	}
	if rec := get(h, strings.Replace(string(PathRouteMetrics), "{id}", "nope", 1)); rec.Code != http.StatusNotFound {
		t.Fatalf("bad id: %d", rec.Code)
	}
	s.overviewErr = query.Unavailable(errors.New("down"))
	if rec := get(h, base); rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), string(query.CodeMetricsUnavailable)) {
		t.Fatalf("unavailable: %d %s", rec.Code, rec.Body)
	}
}

type flowStub struct{ got flowmap.Filters }

func (f *flowStub) Build(_ context.Context, filters flowmap.Filters) (flowmap.FlowMap, error) {
	f.got = filters
	return flowmap.FlowMap{Nodes: []flowmap.Node{{ID: "topic:c:t", Type: flowmap.NodeTopic, Label: "t", ConnectionID: "c"}},
		Edges: []flowmap.Edge{{ID: "e", RouteID: "r", RouteName: "n", TeamName: "team", Branch: "main", From: "topic:c:t", To: "topic:c:t",
			EventsPerSec: 1.5, Status: routes.StatusRunning}}, MetricsAvailable: true}, nil
}

func TestFlowMapMatchesContract(t *testing.T) {
	stub := &flowStub{}
	responder := api.NewResponder(problem.NewCatalog(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := chi.NewRouter()
	api.NewMounter(responder, nil, nil).Mount(r, NewModule(Deps{FlowMap: stub, Responder: responder}))
	spec, err := contract.LoadDefault(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	rec := contract.NewRecorder(r, contract.NewValidator(spec))
	res := get(rec, string(PathFlowMap)+"?connection=c1&team=payments&tag=prod")
	if res.Code != http.StatusOK || stub.got != (flowmap.Filters{Connection: "c1", Team: "payments", Tag: "prod"}) {
		t.Fatalf("flow map: %d %+v", res.Code, stub.got)
	}
	for _, v := range rec.Violations() {
		t.Errorf("contract violation: %s", v)
	}
}
