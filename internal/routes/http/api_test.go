package routeshttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/routes/assembly"
	"github.com/eventrouter/router/internal/routes/bulk"
	routeshttp "github.com/eventrouter/router/internal/routes/http"
	"github.com/eventrouter/router/internal/routes/portability"
	"github.com/eventrouter/router/internal/routes/routestest"
	"github.com/eventrouter/router/internal/routes/validation"
	"github.com/eventrouter/router/internal/store"
)

type recorder struct {
	mu      sync.Mutex
	entries []audit.Entry
}

func (r *recorder) Record(_ context.Context, e audit.Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, e)
	return nil
}

type connections map[string]validation.ConnectionInfo

func (c connections) Find(_ context.Context, ids []string) (map[string]validation.ConnectionInfo, error) {
	out := map[string]validation.ConnectionInfo{}
	for _, id := range ids {
		if info, ok := c[id]; ok {
			out[id] = info
		}
	}
	return out, nil
}

func (c connections) FindByNames(_ context.Context, teamID string, names []string) (map[string]validation.ConnectionInfo, error) {
	out := map[string]validation.ConnectionInfo{}
	for _, info := range c {
		for _, n := range names {
			if info.Name == n {
				out[n] = info
			}
		}
	}
	return out, nil
}

type principalStage struct{ p auth.Principal }

func (s principalStage) Wrap(_ api.RouteSpec, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), s.p)))
	})
}

type harness struct {
	t         *testing.T
	observe   func(method, path string, status int, media string, body []byte)
	server    *httptest.Server
	db        *routestest.DB
	events    *routestest.Publisher
	audit     *recorder
	lifecycle *routes.DeploymentLifecycle
	replays   *routestest.Replays
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	team := uuid.New()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := routestest.NewDB(routes.TeamID(team.String()), "payments")
	repos := db.Repositories()
	conns := connections{"c1": {ID: "c1", TeamID: team.String(), Name: "orders-prod"}, "c2": {ID: "c2", TeamID: team.String(), Name: "partner"}}
	validator, err := assembly.ValidatorFactory{Connections: conns, Names: repos.Routes.(*routestest.RouteRepo), Scripts: assembly.SkipScriptChecker{}}.Build(assembly.Settings{})
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, db: db, events: &routestest.Publisher{}, audit: &recorder{}, replays: routestest.NewReplays()}
	core := &routes.Core{Repos: repos, Tx: routestest.Tx{DB: db}, Validator: validator, Auditor: routes.RouteAuditor{Fallback: h.audit},
		Views:  routes.SummaryBuilder{Connections: conns, Metrics: routes.NullMetricsReader{}, Log: log},
		Events: h.events, Guard: routes.TeamRoleWriteGuard{}, Log: log}
	approval, _ := routes.NewApprovalPolicyManager(routes.ApprovalNone)
	h.lifecycle = routes.NewDeploymentLifecycle(core)
	responder := api.NewResponder(problem.NewCatalog(), log)
	rm, sm, vm := routes.NewRouteManager(core), routes.NewStateManager(core), routes.NewVersionManager(core)
	codecs, err := portability.DefaultCodecs()
	if err != nil {
		t.Fatal(err)
	}
	pm := portability.NewPortabilityManager(portability.Deps{Routes: rm, Versions: vm, Repos: repos, Validator: validator,
		Resolver: portability.ConnectionResolver{Directory: conns}, Codecs: codecs, Auditor: core.Auditor})
	module := routeshttp.NewModule(routeshttp.Deps{Routes: rm, State: sm, Deploy: routes.NewDeployManager(core, approval),
		Versions: vm, Validate: routes.NewValidateManager(core), Portability: pm,
		Bulk: bulk.NewBulkActionManager(problem.NewCatalog(), bulk.PauseHandler{State: sm}, bulk.ResumeHandler{State: sm},
			bulk.AddTagHandler{Routes: rm}, bulk.ExportHandler{Portability: pm}),
		Replays:   routes.NewReplayManager(core, h.replays, routes.ReplayRules{}),
		Responder: responder, Pages: api.NewPageParser(store.NewCursorCodec("test-secret"))})
	mw := audit.NewMiddleware(h.audit, audit.NewDiffBuilder(logging.NewRedactor()), api.AuditContext, log)
	mw.StrictMode = true
	p := auth.Principal{Kind: auth.PrincipalUser, UserID: uuid.New(), Teams: []auth.TeamRole{{TeamID: team, Role: auth.RoleEditor}}}
	mounter := api.NewMounter(responder, []api.GlobalMiddleware{api.RequestIDMiddleware{}, api.RecoverMiddleware{Responder: responder}},
		[]api.RouteMiddleware{principalStage{p: p}, api.AuditStage{Audit: mw}})
	r := chi.NewRouter()
	mounter.Install(r)
	mounter.Mount(r, module)
	h.server = httptest.NewServer(r)
	t.Cleanup(h.server.Close)
	return h
}

type response struct {
	status int
	etag   string
	body   map[string]any
	raw    string
	media  string
}

func (h *harness) do(method, path string, body any, ifMatch string) response {
	h.t.Helper()
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	return h.send(method, path, "application/json", raw, ifMatch)
}

// send issues a request with a raw body of the given media type.
func (h *harness) send(method, path, media string, body []byte, ifMatch string) response {
	h.t.Helper()
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, _ := http.NewRequest(method, h.server.URL+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", media)
	}
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if h.observe != nil {
		h.observe(method, path, res.StatusCode, res.Header.Get("Content-Type"), raw)
	}
	out := response{status: res.StatusCode, etag: res.Header.Get("ETag"), raw: string(raw), media: res.Header.Get("Content-Type")}
	_ = json.Unmarshal(raw, &out.body)
	return out
}

func (h *harness) expect(r response, status int) response {
	h.t.Helper()
	if r.status != status {
		h.t.Fatalf("status %d, want %d: %s", r.status, status, r.raw)
	}
	return r
}

func doc(amount int, source string) map[string]any {
	return map[string]any{
		"apiVersion": "router/v1", "name": "orders",
		"source": map[string]any{"connectionId": source, "topics": []string{"order-events"}, "format": map[string]any{"type": "json"}},
		"branches": []any{map[string]any{
			"id": "main",
			"filter": map[string]any{"type": "and", "children": []any{
				map[string]any{"type": "cond", "field": "value.eventType", "op": "eq", "value": "OrderPaid"},
				map[string]any{"type": "cond", "field": "value.amount", "op": "gt", "value": amount},
			}},
			"transform":   map[string]any{"steps": []any{map[string]any{"kind": "js", "code": "function transform(event, ctx) { return event; }"}}},
			"destination": map[string]any{"connectionId": "c2", "topic": "orders-public"},
		}},
	}
}

const base = "/api/v1/routes"

func TestRouteLifecycleOverHTTP(t *testing.T) {
	newHarness(t).runLifecycle()
}

// runLifecycle drives create → save → validate → deploy → pause/resume → rollback → diff → bulk → archive; returns the route id.
func (h *harness) runLifecycle() string {
	id, item := h.createAndDeploy()
	h.pauseRollbackAndDiff(item)
	h.checkAutoRollback(id)
	dup := h.checkBulkDuplicateArchive(id, item)
	h.checkUnarchive(item)
	h.checkExport(id, dup, item)
	h.checkImport(item)
	h.checkReplay(item)
	return id
}

// createAndDeploy covers create, If-Match handling, validate, deploy, nothing-to-deploy and the list.
func (h *harness) createAndDeploy() (string, string) {
	t := h.t
	created := h.expect(h.do(http.MethodPost, base, map[string]any{"name": "orders", "draft": doc(100, "c1")}, ""), http.StatusCreated)
	id := created.body["id"].(string)
	if created.etag != `"1"` || created.body["status"] != "draft" || len(asList(created.body["issues"])) != 0 {
		t.Fatalf("%s %s", created.etag, created.raw)
	}
	item := base + "/" + id

	h.expect(h.do(http.MethodPatch, item, map[string]any{"description": "x"}, ""), http.StatusPreconditionRequired)
	stale := h.expect(h.do(http.MethodPatch, item, map[string]any{"description": "x"}, `"0"`), http.StatusPreconditionFailed)
	if stale.body["current_etag"] != `"1"` {
		t.Fatal(stale.raw)
	}
	saved := h.expect(h.do(http.MethodPatch, item, map[string]any{"draft": doc(200, "c1")}, created.etag), http.StatusOK)

	valid := h.expect(h.do(http.MethodPost, item+"/validate", map[string]any{"mode": "deploy"}, ""), http.StatusOK)
	if valid.body["valid"] != true {
		t.Fatal(valid.raw)
	}
	h.expect(h.do(http.MethodPost, item+"/deploy", map[string]any{"changeNote": "first"}, created.etag), http.StatusPreconditionFailed)
	deployed := h.expect(h.do(http.MethodPost, item+"/deploy", map[string]any{"changeNote": "first"}, saved.etag), http.StatusAccepted)
	if v := deployed.body["version"].(map[string]any); v["no"].(float64) != 1 || v["origin"] != "deploy" {
		t.Fatal(deployed.raw)
	}
	if len(h.events.Events) != 1 || h.events.DuringTx != 0 {
		t.Fatalf("events %+v during tx %d", h.events.Events, h.events.DuringTx)
	}
	nothing := h.expect(h.do(http.MethodPost, item+"/deploy", nil, deployed.etag), http.StatusConflict)
	if !strings.Contains(nothing.raw, "Nothing to deploy") {
		t.Fatal(nothing.raw)
	}

	list := h.expect(h.do(http.MethodGet, base+"?status=running", nil, ""), http.StatusOK)
	items := asList(list.body["items"])
	if len(items) != 1 || items[0].(map[string]any)["filterSummary"].(map[string]any)["headline"] != "eventType is OrderPaid" {
		t.Fatal(list.raw)
	}
	if items[0].(map[string]any)["source"].(map[string]any)["connectionName"] != "orders-prod" {
		t.Fatal(list.raw)
	}
	return id, item
}

// pauseRollbackAndDiff covers pause/resume, invalid drafts, rollback, diff, versions and deployments.
func (h *harness) pauseRollbackAndDiff(item string) {
	t := h.t
	h.statusOf(h.expect(h.do(http.MethodPost, item+"/pause", nil, ""), http.StatusOK), "paused")
	h.statusOf(h.expect(h.do(http.MethodPost, item+"/pause", nil, ""), http.StatusOK), "paused")
	resumed := h.statusOf(h.expect(h.do(http.MethodPost, item+"/resume", nil, ""), http.StatusOK), "running")

	broken := h.expect(h.do(http.MethodPatch, item, map[string]any{"draft": doc(300, "nope")}, resumed.etag), http.StatusOK)
	if issues := asList(broken.body["issues"]); len(issues) != 1 || issues[0].(map[string]any)["code"] != "connection_not_found" {
		t.Fatal(broken.raw)
	}
	rejected := h.expect(h.do(http.MethodPost, item+"/deploy", nil, broken.etag), http.StatusUnprocessableEntity)
	if rejected.body["path"] != "/source/connectionId" || len(asList(rejected.body["issues"])) != 1 {
		t.Fatal(rejected.raw)
	}
	fixed := h.expect(h.do(http.MethodPatch, item, map[string]any{"draft": doc(300, "c1")}, broken.etag), http.StatusOK)
	h.expect(h.do(http.MethodPost, item+"/deploy", nil, fixed.etag), http.StatusAccepted)

	rolled := h.expect(h.do(http.MethodPost, item+"/rollback", map[string]any{"version": 1}, ""), http.StatusAccepted)
	v3 := rolled.body["version"].(map[string]any)
	if v3["no"].(float64) != 3 || v3["origin"] != "rollback" || v3["basedOn"].(float64) != 1 {
		t.Fatal(rolled.raw)
	}
	h.expect(h.do(http.MethodPost, item+"/rollback", map[string]any{"version": 3}, ""), http.StatusConflict)
	h.expect(h.do(http.MethodPost, item+"/rollback", map[string]any{"version": 9}, ""), http.StatusNotFound)

	d12 := h.expect(h.do(http.MethodGet, item+"/versions/1/diff/2", nil, ""), http.StatusOK)
	if !strings.Contains(d12.raw, `amount \u003e 200 → amount \u003e 300`) {
		t.Fatal(d12.raw)
	}
	same := h.expect(h.do(http.MethodGet, item+"/versions/active/diff/draft", nil, ""), http.StatusOK)
	if strings.Contains(same.raw, `"mark"`) {
		t.Fatal("rollback replaced the draft, so active and draft match: " + same.raw)
	}
	h.expect(h.do(http.MethodGet, item+"/versions/x/diff/draft", nil, ""), http.StatusBadRequest)

	versions := h.expect(h.do(http.MethodGet, item+"/versions?limit=2", nil, ""), http.StatusOK)
	vs := asList(versions.body["items"])
	if len(vs) != 2 || vs[0].(map[string]any)["no"].(float64) != 3 || vs[0].(map[string]any)["active"] != true || versions.body["next_cursor"] == nil {
		t.Fatal(versions.raw)
	}
	h.expect(h.do(http.MethodGet, item+"/versions/2", nil, ""), http.StatusOK)
	deps := h.expect(h.do(http.MethodGet, item+"/deployments", nil, ""), http.StatusOK)
	if len(asList(deps.body["items"])) != 3 {
		t.Fatal(deps.raw)
	}
}

func (h *harness) statusOf(r response, want string) response {
	h.t.Helper()
	if r.body["status"] != want {
		h.t.Fatalf("status %v want %s: %s", r.body["status"], want, r.raw)
	}
	return r
}

func (h *harness) checkAutoRollback(id string) {
	h.t.Helper()
	ctx := context.Background()
	repos := h.db.Repositories()
	latest, _ := repos.Deployments.Latest(ctx, routes.RouteID(id))
	out, err := h.lifecycle.MarkFailed(ctx, latest.ID, "consumer crashed")
	if err != nil || out.Rollback == nil {
		h.t.Fatalf("%+v %v", out, err)
	}
	if out.Rollback.Version.Origin != routes.OriginAutoRollback || out.Rollback.Version.No != 4 || *out.Rollback.Version.BasedOnNo != 2 {
		h.t.Fatalf("%+v", out.Rollback.Version)
	}
	if _, err := h.lifecycle.MarkFailed(ctx, latest.ID, "again"); err != routes.ErrTransitionNotAllowed {
		h.t.Fatalf("expected transition error, got %v", err)
	}
	if _, err := h.lifecycle.MarkActive(ctx, out.Rollback.Deployment.ID); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) checkBulkDuplicateArchive(id, item string) string {
	h.t.Helper()
	res := h.expect(h.do(http.MethodPost, base+"/bulk", map[string]any{"action": "add_tag", "ids": []string{id, uuid.NewString()}, "tag": "pii"}, ""), http.StatusOK)
	results := asList(res.body["results"])
	if results[0].(map[string]any)["ok"] != true || results[1].(map[string]any)["problem"].(map[string]any)["status"].(float64) != 404 {
		h.t.Fatal(res.raw)
	}
	h.expect(h.do(http.MethodPost, base+"/bulk", map[string]any{"action": "explode", "ids": []string{id}}, ""), http.StatusBadRequest)
	dup := h.expect(h.do(http.MethodPost, item+"/duplicate", nil, ""), http.StatusCreated)
	if dup.body["name"] != "orders-copy" || dup.body["status"] != "draft" {
		h.t.Fatal(dup.raw)
	}
	h.expect(h.do(http.MethodPost, base+"/"+dup.body["id"].(string)+"/resume", nil, ""), http.StatusConflict)
	h.expect(h.do(http.MethodPost, base, map[string]any{"name": "orders"}, ""), http.StatusConflict)
	h.expect(h.do(http.MethodDelete, item, nil, ""), http.StatusNoContent)
	h.expect(h.do(http.MethodDelete, item, nil, ""), http.StatusNoContent)
	h.expect(h.do(http.MethodPost, item+"/pause", nil, ""), http.StatusConflict)
	all := h.expect(h.do(http.MethodGet, base, nil, ""), http.StatusOK)
	if len(asList(all.body["items"])) != 1 {
		h.t.Fatal(all.raw)
	}
	archived := h.expect(h.do(http.MethodGet, base+"?status=archived", nil, ""), http.StatusOK)
	if len(asList(archived.body["items"])) != 1 {
		h.t.Fatal(archived.raw)
	}
	h.expect(h.do(http.MethodGet, base+"?status=bogus", nil, ""), http.StatusUnprocessableEntity)
	h.expect(h.do(http.MethodGet, base+"/not-a-uuid", nil, ""), http.StatusNotFound)
	return dup.body["id"].(string)
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}
