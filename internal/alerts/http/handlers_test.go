package alertshttp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/alerts"
	"github.com/eventrouter/router/internal/alerts/alertstest"
	"github.com/eventrouter/router/internal/alerts/notify"
	"github.com/eventrouter/router/internal/alerts/service"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/api/contract"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/secrets"
	"github.com/eventrouter/router/internal/store"
)

// memSecrets is an in-memory secrets.Manager.
type memSecrets struct {
	mu   sync.Mutex
	vals map[secrets.Ref]string
}

func (m *memSecrets) Put(_ context.Context, p []byte) (secrets.Ref, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.vals == nil {
		m.vals = map[secrets.Ref]string{}
	}
	ref := secrets.NewRef()
	m.vals[ref] = string(p)
	return ref, nil
}
func (m *memSecrets) Get(_ context.Context, r secrets.Ref) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return []byte(m.vals[r]), nil
}
func (m *memSecrets) Replace(_ context.Context, r secrets.Ref, p []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.vals[r] = string(p)
	return nil
}
func (m *memSecrets) Delete(_ context.Context, r secrets.Ref) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.vals, r)
	return nil
}

type access struct{ allow bool }

func (a access) CanWriteAlerts(context.Context, uuid.UUID) bool { return a.allow }

type deliver struct{ channels int }

func (d *deliver) Deliver(_ context.Context, _ notify.Notification, ch []alerts.ChannelRef) []notify.ChannelResult {
	d.channels = len(ch)
	out := []notify.ChannelResult{}
	for _, c := range ch {
		out = append(out, notify.ChannelResult{Channel: c.Type, OK: true})
	}
	return out
}

type env struct {
	h       http.Handler
	rec     *contract.Recorder
	secrets *memSecrets
	events  *alertstest.Events
	route   uuid.UUID
	user    uuid.UUID
	access  *access
	routes  alertstest.Routes
}

type principal struct{ id uuid.UUID }

func (p principal) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), auth.Principal{Kind: auth.PrincipalUser, UserID: p.id})))
	})
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{secrets: &memSecrets{}, events: &alertstest.Events{}, route: uuid.New(), user: uuid.New(), access: &access{allow: true}}
	routes := alertstest.Routes{e.route: {ID: e.route, Name: "orders", TeamID: uuid.New(), Running: true}}
	archived := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	routes[archived] = alerts.RouteInfo{ID: archived, Archived: true}
	e.routes = routes
	rules := &service.RuleService{Rules: &alertstest.Rules{}, Routes: routes, Secrets: service.ChannelSecretStore{Secrets: e.secrets},
		Access: e.access, Tx: alertstest.NoTx{}, Deliver: &deliver{}}
	responder := api.NewResponder(problem.NewCatalog(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	mod := NewModule(Deps{Rules: rules, Feed: &service.FeedService{Events: e.events, Now: time.Now}, Responder: responder,
		Pages: api.NewPageParser(store.NewCursorCodec("test-secret"))})
	r := chi.NewRouter()
	api.NewMounter(responder, nil, nil).Mount(r, mod)
	spec, err := contract.LoadDefault(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	e.rec = contract.NewRecorder(principal{id: e.user}.wrap(r), contract.NewValidator(spec))
	t.Cleanup(func() {
		for _, v := range e.rec.Violations() {
			t.Errorf("contract violation: %s", v)
		}
	})
	e.h = e.rec
	return e
}

func (e *env) do(method, path, body string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func TestAlertRulesLifecycleMatchesContract(t *testing.T) {
	e := newEnv(t)
	draft := `{"routeId":"` + e.route.String() + `","name":"Lag high","kind":"lag_above","threshold":20000,"windowMinutes":10,` +
		`"channels":[{"type":"webhook","secret":"https://hooks.example/abc"},{"type":"email","target":"a@example.com, b@example.com"}]}`
	rec := e.do(http.MethodPost, string(PathRules), draft)
	if rec.Code != http.StatusCreated || rec.Header().Get("ETag") != `"1"` || strings.Contains(rec.Body.String(), "hooks.example") {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	var rule alerts.RuleView
	_ = json.Unmarshal(rec.Body.Bytes(), &rule)
	if !rule.Channels[0].SecretSet || rule.Channels[1].Target != "a@example.com,b@example.com" || len(e.secrets.vals) != 1 {
		t.Fatalf("channels: %+v secrets=%d", rule.Channels, len(e.secrets.vals))
	}
	path := strings.Replace(string(PathRule), "{id}", rule.ID.String(), 1)
	if rec := e.do(http.MethodPatch, path, `{"threshold":5}`); rec.Code != http.StatusPreconditionRequired {
		t.Fatalf("no If-Match: %d", rec.Code)
	}
	if rec := e.do(http.MethodPatch, path, `{"threshold":5}`, "If-Match", `"9"`); rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale: %d", rec.Code)
	}
	rec = e.do(http.MethodPatch, path, `{"threshold":5,"channels":[{"type":"webhook"}]}`, "If-Match", `"1"`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"secretSet":true`) || rec.Header().Get("ETag") != `"2"` {
		t.Fatalf("patch keeps secret: %d %s", rec.Code, rec.Body)
	}
	if rec := e.do(http.MethodGet, string(PathRules)+"?routeId="+e.route.String(), ""); !strings.Contains(rec.Body.String(), `"threshold":5`) {
		t.Fatalf("list: %s", rec.Body)
	}
	if rec := e.do(http.MethodPost, strings.Replace(string(PathRuleTest), "{id}", rule.ID.String(), 1), ""); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"results":[{"channel":"webhook","ok":true}]`) {
		t.Fatalf("test: %d %s", rec.Code, rec.Body)
	}
	if rec := e.do(http.MethodDelete, path, ""); rec.Code != http.StatusNoContent || len(e.secrets.vals) != 0 {
		t.Fatalf("delete: %d secrets=%d", rec.Code, len(e.secrets.vals))
	}
	if rec := e.do(http.MethodDelete, path, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("delete again: %d", rec.Code)
	}
}

// TestAlertRuleOfArchivedRouteCanBeDeleted: archiving a route must not strand its alert rules; alert.write still applies.
func TestAlertRuleOfArchivedRouteCanBeDeleted(t *testing.T) {
	e := newEnv(t)
	draft := `{"routeId":"` + e.route.String() + `","name":"Lag","kind":"lag_above","threshold":10,"windowMinutes":5,` +
		`"channels":[{"type":"webhook","secret":"https://hooks.example/x"}]}`
	rec := e.do(http.MethodPost, string(PathRules), draft)
	var rule alerts.RuleView
	if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &rule) != nil {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	info := e.routes[e.route]
	info.Archived = true
	e.routes[e.route] = info
	path := strings.Replace(string(PathRule), "{id}", rule.ID.String(), 1)
	e.access.allow = false
	if rec := e.do(http.MethodDelete, path, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("delete without alert.write: %d", rec.Code)
	}
	e.access.allow = true
	if rec := e.do(http.MethodDelete, path, ""); rec.Code != http.StatusNoContent || len(e.secrets.vals) != 0 {
		t.Fatalf("delete on archived route: %d %s secrets=%d", rec.Code, rec.Body, len(e.secrets.vals))
	}
}

func TestAlertRuleValidationProblems(t *testing.T) {
	e := newEnv(t)
	base := `"routeId":"` + e.route.String() + `","name":"n","kind":"lag_above","threshold":10,"windowMinutes":5`
	cases := map[string]string{
		`{` + base + `,"channels":[{"type":"pager"}]}`:                                                                     "/channels/0/type",
		`{` + base + `,"channels":[{"type":"email","target":"nope"}]}`:                                                     "/channels/0/target",
		`{` + base + `,"channels":[{"type":"slack"}]}`:                                                                     "/channels/0/secret",
		`{` + base + `,"channels":[{"type":"teams","secret":"ftp://x"}]}`:                                                  "/channels/0/secret",
		`{"routeId":"` + e.route.String() + `","name":"n","kind":"x","threshold":1,"windowMinutes":5}`:                     "/kind",
		`{"routeId":"` + e.route.String() + `","name":"n","kind":"error_rate_above","threshold":101,"windowMinutes":5}`:    "/threshold",
		`{"routeId":"` + e.route.String() + `","name":"n","kind":"lag_above","threshold":1,"windowMinutes":0}`:             "/windowMinutes",
		`{"routeId":"33333333-3333-4333-8333-333333333333","name":"n","kind":"lag_above","threshold":1,"windowMinutes":5}`: "/routeId",
		`{"routeId":"` + uuid.NewString() + `","name":"n","kind":"lag_above","threshold":1,"windowMinutes":5}`:             "/routeId",
	}
	for body, path := range cases {
		rec := e.do(http.MethodPost, string(PathRules), body)
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"path":"`+path+`"`) {
			t.Errorf("%s → %d %s (want path %s)", body, rec.Code, rec.Body, path)
		}
	}
	e.access.allow = false
	if rec := e.do(http.MethodPost, string(PathRules), `{`+base+`}`); rec.Code != http.StatusForbidden {
		t.Fatalf("forbidden: %d", rec.Code)
	}
}

func TestAlertsFeedAndAck(t *testing.T) {
	e := newEnv(t)
	v := 3.0
	ev, _, _ := e.events.Fire(context.Background(), alerts.AlertEvent{RuleID: uuid.New(), RouteID: e.route, Value: &v, FiredAt: time.Now()})
	rec := e.do(http.MethodGet, string(PathFeed)+"?state=firing&limit=20", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"unread":1`) {
		t.Fatalf("feed: %d %s", rec.Code, rec.Body)
	}
	if rec := e.do(http.MethodPost, strings.Replace(string(PathAck), "{id}", "1", 1), ""); rec.Code != http.StatusNoContent || ev.ID != 1 {
		t.Fatalf("ack: %d", rec.Code)
	}
	if rec := e.do(http.MethodGet, string(PathFeed), ""); !strings.Contains(rec.Body.String(), `"unread":0`) || !strings.Contains(rec.Body.String(), `"acked":true`) {
		t.Fatalf("after ack: %s", rec.Body)
	}
	if rec := e.do(http.MethodGet, string(PathFeed)+"?state=open", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad state: %d", rec.Code)
	}
	if rec := e.do(http.MethodPost, strings.Replace(string(PathAck), "{id}", "x", 1), ""); rec.Code != http.StatusNotFound {
		t.Fatalf("bad id: %d", rec.Code)
	}
	if e.rec.Exchanges() == 0 {
		t.Fatal("no exchanges validated")
	}
}
