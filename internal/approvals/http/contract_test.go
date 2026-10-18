package http_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/approvals"
	"github.com/eventrouter/router/internal/approvals/approvalstest"
	approvalshttp "github.com/eventrouter/router/internal/approvals/http"
	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/rbac/rbactest"
	"github.com/eventrouter/router/internal/store"
)

var team = uuid.MustParse("00000000-0000-0000-0000-0000000000aa")

const testUserHeader = "X-Test-User"

type headerAuth struct{}

func (headerAuth) Authenticate(r *http.Request) (auth.Principal, error) {
	id, err := uuid.Parse(r.Header.Get(testUserHeader))
	if err != nil {
		return auth.Anonymous, auth.ErrUnauthenticated
	}
	return auth.Principal{Kind: auth.PrincipalUser, UserID: id}, nil
}

// fixture mounts the approvals module over in-memory stores behind the real authorizer and audit stages.
type fixture struct {
	router                   chi.Router
	store                    *approvalstest.Store
	editor, approver, viewer uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	w := rbactest.NewWorld()
	f := &fixture{store: approvalstest.NewStore()}
	for role, id := range map[rbac.Role]*uuid.UUID{rbac.RoleEditor: &f.editor, rbac.RoleApprover: &f.approver, rbac.RoleViewer: &f.viewer} {
		*id = w.AddUser(false, rbac.UserActive)
		w.Bind(*id, team, role)
	}
	f.store.Approvals = []uuid.UUID{f.approver}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	responder := api.NewResponder(problem.NewCatalog(), log)
	access := rbac.NewManager(rbac.ManagerDeps{Engine: rbac.NewRoleBindingEngine(rbac.DefaultRoleMatrix(), rbac.DefaultPermissionCatalog()),
		Subjects: rbac.NewSubjectResolver(rbac.NewTTLSubjectCache(0, nil), w), Matrix: rbac.DefaultRoleMatrix(), Catalog: rbac.DefaultPermissionCatalog(), Log: log})
	manager := approvals.NewManager(approvals.Deps{Tx: rbactest.PassTx{}, Store: f.store, Access: access,
		Teams: approvalstest.Teams{team: {ID: team, RequireProdApproval: true}}, Destinations: approvals.DestinationReader{Connections: approvalstest.Connections{}},
		Policy: approvals.TeamProductionPolicy{}, Activator: &approvalstest.Activator{Store: f.store}, Notifier: &approvals.MemoryNotifier{},
		Trail: rbac.NewAuditTrail(w), Log: log})
	mod := approvalshttp.NewModule(approvalshttp.Deps{Approvals: manager, Owner: f.store, Responder: responder, Pages: api.NewPageParser(store.NewCursorCodec("test"))})
	m := api.NewMounter(responder, nil, []api.RouteMiddleware{
		api.AuthenticateStage{Authenticator: headerAuth{}, Responder: responder},
		api.AuthorizeStage{Authorize: auth.NewAuthorizeMiddleware(rbac.NewAuthorizerAdapter(access)), Responder: responder},
		api.AuditStage{Audit: audit.NewMiddleware(w, audit.NewDiffBuilder(logging.NewRedactor()), api.AuditContext, log)},
	})
	f.router = chi.NewRouter()
	m.Mount(f.router, mod)
	return f
}

func (f *fixture) do(as uuid.UUID, method, path, body, ifMatch string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(testUserHeader, as.String())
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

// TestApprovalsMatchOpenAPI (AC-134) drives every approvals endpoint and validates bodies against the spec.
func TestApprovalsMatchOpenAPI(t *testing.T) {
	f, contract := newFixture(t), rbactest.NewOpenAPIContract(t)
	first, second := f.store.AddPending(team, f.editor), f.store.AddPending(team, f.editor)
	one, two := "/api/v1/approvals/"+first.String(), "/api/v1/approvals/"+second.String()
	steps := []struct {
		as                       uuid.UUID
		method, path, body, etag string
		key                      string
		status                   int
		media                    rbactest.MediaType
	}{
		{f.viewer, http.MethodGet, "/api/v1/approvals?status=pending_approval", "", "", "approvals", http.StatusOK, rbactest.MediaJSON},
		{f.viewer, http.MethodGet, "/api/v1/approvals?status=bogus", "", "", "approvals", http.StatusUnprocessableEntity, rbactest.MediaProblem},
		{f.approver, http.MethodGet, one, "", "", "approval", http.StatusOK, rbactest.MediaJSON},
		{f.editor, http.MethodPost, one + "/approve", `{}`, `"1"`, "approve", http.StatusForbidden, rbactest.MediaProblem},
		{f.approver, http.MethodPost, one + "/approve", ``, "", "approve", http.StatusPreconditionRequired, rbactest.MediaProblem},
		{f.approver, http.MethodPost, one + "/approve", `{"note":"ok"}`, `"1"`, "approve", http.StatusOK, rbactest.MediaJSON},
		{f.approver, http.MethodPost, one + "/reject", `{"note":"late"}`, `"2"`, "reject", http.StatusConflict, rbactest.MediaProblem},
		{f.viewer, http.MethodPost, two + "/comments", `{"body":"why?"}`, "", "comments", http.StatusCreated, rbactest.MediaJSON},
		{f.approver, http.MethodPost, two + "/reject", `{"note":""}`, `"1"`, "reject", http.StatusUnprocessableEntity, rbactest.MediaProblem},
		{f.viewer, http.MethodPost, two + "/withdraw", ``, "", "withdraw", http.StatusForbidden, rbactest.MediaProblem},
		{f.editor, http.MethodPost, two + "/withdraw", ``, "", "withdraw", http.StatusOK, rbactest.MediaJSON},
		{f.approver, http.MethodGet, two, "", "", "approval", http.StatusOK, rbactest.MediaJSON},
		{f.approver, http.MethodGet, "/api/v1/approvals/" + uuid.NewString(), "", "", "approval", http.StatusNotFound, rbactest.MediaProblem},
	}
	for _, s := range steps {
		rec := f.do(s.as, s.method, s.path, s.body, s.etag)
		if rec.Code != s.status {
			t.Fatalf("%s %s: status %d, want %d: %s", s.method, s.path, rec.Code, s.status, rec.Body)
		}
		contract.Check(t, "approvals.yaml", s.key, s.method, rec.Code, s.media, rec.Body.Bytes())
	}
	if _, err := f.store.Get(context.Background(), first); err != nil {
		t.Fatal(err)
	}
}
