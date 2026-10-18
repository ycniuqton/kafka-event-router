package testcaseshttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/library/testcases"
	"github.com/eventrouter/router/internal/library/testcaseshttp"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/testkit"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/enginetest"
)

const (
	draftSteps = `[{"kind":"rename","from":"value.a","to":"value.b"},{"kind":"drop_if","condition":{"type":"cond","field":"value.drop","op":"eq","value":true}}]`
	pinnedOut  = `{"key":null,"value":{"b":1},"headers":{}}`
)

type principalStage struct{ p auth.Principal }

func (s principalStage) Wrap(_ api.RouteSpec, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), s.p)))
	})
}

type fixture struct {
	t       *testing.T
	server  *httptest.Server
	service *testcases.TestCaseService
	route   uuid.UUID
}

// newFixture seeds a team, a user and a route whose draft renames a → b, then serves the module.
func newFixture(t *testing.T) *fixture {
	sb := testkit.NewManager().Postgres(t)
	sb.Migrate(t)
	ctx := context.Background()
	f := &fixture{t: t}
	var team, user uuid.UUID
	must(t, sb.DB.Pool().QueryRow(ctx, `INSERT INTO teams (name) VALUES ('payments') RETURNING id`).Scan(&team))
	must(t, sb.DB.Pool().QueryRow(ctx, `INSERT INTO users (email, display_name) VALUES ('ana@x.dev', 'Ana') RETURNING id`).Scan(&user))
	must(t, sb.DB.Pool().QueryRow(ctx, `INSERT INTO routes (team_id, name, draft_config) VALUES ($1, 'orders', $2) RETURNING id`,
		team, enginetest.RouteJSON("null", draftSteps)).Scan(&f.route))
	repo := testcases.NewRepository(sb.DB)
	runner := testcases.NewTestCaseRunner(testcases.RunnerDeps{Compiler: compile.NewDefaultCompiler(), Comparators: testcases.DefaultComparatorManager()})
	f.service = testcases.NewService(testcases.ServiceDeps{Store: repo, Routes: repo, Runner: runner, Limits: testcases.DefaultLimits})
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	responder := api.NewResponder(problem.NewCatalog(), log)
	p := auth.Principal{Kind: auth.PrincipalUser, UserID: user, Teams: []auth.TeamRole{{TeamID: team, Role: auth.RoleEditor}}}
	mounter := api.NewMounter(responder, []api.GlobalMiddleware{api.RequestIDMiddleware{}}, []api.RouteMiddleware{principalStage{p: p}})
	r := chi.NewRouter()
	mounter.Install(r)
	mounter.Mount(r, testcaseshttp.NewModule(testcaseshttp.Deps{Service: f.service, Owners: repo, Responder: responder}))
	f.server = httptest.NewServer(r)
	t.Cleanup(f.server.Close)
	return f
}

type reply struct {
	status int
	etag   string
	body   map[string]any
}

func (f *fixture) do(method, path, body, ifMatch string) reply {
	f.t.Helper()
	req, _ := http.NewRequest(method, f.server.URL+"/api/v1/routes/"+f.route.String()+"/test-cases"+path, bytes.NewBufferString(body))
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	res, err := http.DefaultClient.Do(req)
	must(f.t, err)
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := reply{status: res.StatusCode, etag: res.Header.Get("ETag")}
	_ = json.Unmarshal(raw, &out.body)
	return out
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func pin(name, value, outcome, output string) string {
	return `{"name":"` + name + `","inputEvent":{"key":null,"value":` + value + `,"headers":[]},"expectedOutcome":"` + outcome + `","expectedOutput":` + output + `}`
}

// TestTestCaseLifecycle: pin (passed + dropped), list, run against the stored draft, break it with an edit,
// re-lock with If-Match (412 on a stale tag), delete; the routes port reports the failing case.
func TestTestCaseLifecycle(t *testing.T) {
	f := newFixture(t)
	created := f.do(http.MethodPost, "", pin("renames a", `{"a":1}`, "passed", pinnedOut), "")
	if created.status != http.StatusCreated || created.body["createdBy"] != "Ana" || created.etag == "" {
		t.Fatalf("create = %d %v", created.status, created.body)
	}
	if r := f.do(http.MethodPost, "", pin("drops", `{"drop":true}`, "dropped", "null"), ""); r.status != http.StatusCreated {
		t.Fatalf("create dropped = %d %v", r.status, r.body)
	}
	if r := f.do(http.MethodPost, "", pin("no output", `{"a":1}`, "passed", "null"), ""); r.status != http.StatusUnprocessableEntity {
		t.Fatalf("passed without output = %d", r.status)
	}
	if r := f.do(http.MethodGet, "", "", ""); len(r.body["items"].([]any)) != 2 {
		t.Fatalf("list = %v", r.body)
	}
	run := f.do(http.MethodPost, "/run", `{"steps":`+draftSteps+`}`, "")
	if run.body["passed"] != 2.0 || run.body["failed"] != 0.0 {
		t.Fatalf("run = %v", run.body)
	}
	broken := f.do(http.MethodPost, "/run", `{"steps":[{"kind":"rename","from":"value.a","to":"value.c"}]}`, "")
	if broken.body["passed"] != 0.0 || broken.body["failed"] != 2.0 {
		t.Fatalf("broken run = %v", broken.body)
	}
	id := created.body["id"].(string)
	if r := f.do(http.MethodPatch, "/"+id, `{"name":"x"}`, `"99"`); r.status != http.StatusPreconditionFailed {
		t.Fatalf("stale patch = %d", r.status)
	}
	relock := f.do(http.MethodPatch, "/"+id, `{"expectedOutput":{"key":null,"value":{"c":1},"headers":{}}}`, created.etag)
	if relock.status != http.StatusOK || relock.body["version"] != 2.0 {
		t.Fatalf("relock = %d %v", relock.status, relock.body)
	}
	port, err := testcases.RoutePort{Service: f.service}.Run(context.Background(), f.route.String(), []byte(enginetest.RouteJSON("null", draftSteps)))
	if err != nil || len(port) != 2 || port[0].Passed || !port[1].Passed {
		t.Fatalf("port = %+v %v", port, err)
	}
	if r := f.do(http.MethodDelete, "/"+id, "", ""); r.status != http.StatusNoContent {
		t.Fatalf("delete = %d", r.status)
	}
	if r := f.do(http.MethodDelete, "/"+id, "", ""); r.status != http.StatusNotFound {
		t.Fatalf("second delete = %d", r.status)
	}
}

// TestCaseLimits: the 200-case cap and the 256 KB input cap answer 422.
func TestCaseLimits(t *testing.T) {
	f := newFixture(t)
	owner := testcases.Owner{Kind: testcases.OwnerRoute, ID: f.route}
	for i := 0; i < testcases.DefaultLimits.MaxCasesPerOwner; i++ {
		_, err := f.service.Create(context.Background(), owner, testcases.NewCase{InputEvent: json.RawMessage(`{"value":{}}`), ExpectedOutcome: testcases.OutcomeDropped})
		must(t, err)
	}
	if r := f.do(http.MethodPost, "", pin("one more", `{}`, "dropped", "null"), ""); r.status != http.StatusUnprocessableEntity {
		t.Fatalf("over cap = %d", r.status)
	}
	big := `{"s":"` + string(bytes.Repeat([]byte("x"), testcases.DefaultLimits.MaxCaseEventBytes)) + `"}`
	if r := f.do(http.MethodPost, "", pin("big", big, "dropped", "null"), ""); r.status != http.StatusUnprocessableEntity {
		t.Fatalf("big = %d", r.status)
	}
}
