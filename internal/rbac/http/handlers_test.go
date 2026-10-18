package http_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	authhandlers "github.com/eventrouter/router/internal/auth/handlers"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/rbac"
	rbachttp "github.com/eventrouter/router/internal/rbac/http"
	"github.com/eventrouter/router/internal/rbac/rbactest"
	"github.com/eventrouter/router/internal/store"
)

var (
	team      = uuid.MustParse("00000000-0000-0000-0000-0000000000aa")
	otherConn = uuid.MustParse("00000000-0000-0000-0000-0000000000cc")
)

const testUserHeader = "X-Test-User"

type headerAuth struct{}

func (headerAuth) Authenticate(r *http.Request) (auth.Principal, error) {
	id, err := uuid.Parse(r.Header.Get(testUserHeader))
	if err != nil {
		return auth.Anonymous, auth.ErrUnauthenticated
	}
	return auth.Principal{Kind: auth.PrincipalUser, UserID: id}, nil
}

type fixture struct {
	w      *rbactest.World
	teams  *rbactest.TeamStore
	grants *rbactest.Grants
	router chi.Router
	admin  uuid.UUID
	editor uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	w := rbactest.NewWorld()
	f := &fixture{w: w, admin: w.AddUser(false, rbac.UserActive), editor: w.AddUser(false, rbac.UserActive)}
	w.Bind(f.admin, team, rbac.RoleAdmin)
	w.Bind(f.editor, team, rbac.RoleEditor)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	responder := api.NewResponder(problem.NewCatalog(), log)
	matrix, perms := rbac.DefaultRoleMatrix(), rbac.DefaultPermissionCatalog()
	manager := rbac.NewManager(rbac.ManagerDeps{Engine: rbac.NewRoleBindingEngine(matrix, perms),
		Subjects: rbac.NewSubjectResolver(rbac.NewTTLSubjectCache(0, time.Now), w), Matrix: matrix, Catalog: perms, Log: log})
	guard, trail := rbac.NewAdminGuard(w), rbac.NewAuditTrail(w)
	f.teams, f.grants = rbactest.NewTeamStore(), rbactest.NewGrants(rbactest.Connections{otherConn: {ID: otherConn, TeamID: uuid.New(), Name: "shared", EnvTag: rbac.EnvDev}})
	f.teams.Items[team] = rbac.Team{ID: team, Name: "default", RequireProdApproval: true, RowVersion: 1}
	all := rbac.RoleSet{rbac.RoleViewer: true, rbac.RoleEditor: true, rbac.RoleApprover: true, rbac.RoleAdmin: true}
	mod := rbachttp.NewModule(rbachttp.Deps{
		Teams: rbac.NewTeamService(rbac.TeamServiceDeps{Tx: rbactest.PassTx{}, Teams: f.teams, Trail: trail, Defaults: rbac.DefaultTeamDefaults}),
		Grants: rbac.NewGrantService(rbac.GrantServiceDeps{Tx: rbactest.PassTx{}, Grants: f.grants, Access: rbac.NewConnectionAccess(f.grants.Connections, f.grants),
			Teams: f.teams, Trail: trail}),
		Inviter: rbac.NewInviter(rbac.InviterDeps{Tx: rbactest.PassTx{}, Invites: rbactest.Invitations{W: w}, Bindings: w, Teams: f.teams,
			Users: rbactest.UserStore{W: w}, Assignable: all, Trail: trail}),
		Users:    rbac.NewUserService(rbac.UserServiceDeps{Tx: rbactest.PassTx{}, Users: rbactest.UserStore{W: w}, Guard: guard, Sessions: w, Trail: trail}),
		Bindings: rbac.NewBindingService(rbac.BindingServiceDeps{Tx: rbactest.PassTx{}, Bindings: w, Subjects: w, Guard: guard, Assignable: rbac.RoleSet{rbac.RoleAdmin: true, rbac.RoleEditor: true}, Trail: trail}),
		Settings: rbac.NewSettingsService(rbac.SettingsServiceDeps{Tx: rbactest.PassTx{}, Store: rbactest.NewSettings(),
			Catalog: rbac.DefaultSettingCatalog(), Trail: trail}),
		Responder: responder, Pages: api.NewPageParser(store.NewCursorCodec("test")),
	})
	m := api.NewMounter(responder, nil, []api.RouteMiddleware{
		api.AuthenticateStage{Authenticator: headerAuth{}, Responder: responder},
		api.AuthorizeStage{Authorize: auth.NewAuthorizeMiddleware(rbac.NewAuthorizerAdapter(manager)), Responder: responder},
		api.AuditStage{Audit: audit.NewMiddleware(w, audit.NewDiffBuilder(logging.NewRedactor()), api.AuditContext, log)},
	})
	f.router = chi.NewRouter()
	m.Mount(f.router, mod)
	return f
}

func (f *fixture) do(as uuid.UUID, method, path, body, ifMatch string) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(testUserHeader, as.String())
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestUsersEndpoints(t *testing.T) {
	f := newFixture(t)
	rec, body := f.do(f.editor, http.MethodGet, "/api/v1/users", "", "")
	if rec.Code != http.StatusOK || len(body["items"].([]any)) != 2 {
		t.Fatalf("list: %d %v", rec.Code, body)
	}
	rec, body = f.do(f.editor, http.MethodPatch, "/api/v1/users/"+f.admin.String(), `{"displayName":"x"}`, `"1"`)
	if rec.Code != http.StatusForbidden || body["missingPermission"] != "user.manage" {
		t.Fatalf("editor patch: %d %v", rec.Code, body)
	}
	f.w.Users[f.admin].WorkspaceAdmin = true
	rec, _ = f.do(f.admin, http.MethodPatch, "/api/v1/users/"+f.editor.String(), `{"displayName":"Hoa"}`, "")
	if rec.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing If-Match: %d", rec.Code)
	}
	rec, body = f.do(f.admin, http.MethodPatch, "/api/v1/users/"+f.admin.String(), `{"disabled":true}`, `"1"`)
	if rec.Code != http.StatusConflict || body["code"] != "last_admin" {
		t.Fatalf("disable last admin: %d %v", rec.Code, body)
	}
	rec, body = f.do(f.admin, http.MethodPatch, "/api/v1/users/"+f.editor.String(), `{"displayName":"Hoa"}`, `"1"`)
	if rec.Code != http.StatusOK || body["displayName"] != "Hoa" || rec.Header().Get("ETag") != `"2"` {
		t.Fatalf("patch: %d %v", rec.Code, body)
	}
}

func TestRoleBindingEndpoints(t *testing.T) {
	f := newFixture(t)
	newcomer := f.w.AddUser(false, rbac.UserActive)
	path := "/api/v1/role-bindings/" + newcomer.String() + "/" + team.String()
	rec, body := f.do(f.editor, http.MethodPut, path, `{"role":"editor"}`, "")
	if rec.Code != http.StatusForbidden || body["missingPermission"] != "team.members_manage" {
		t.Fatalf("editor put: %d %v", rec.Code, body)
	}
	rec, body = f.do(f.admin, http.MethodPut, path, `{"role":"viewer"}`, "")
	if rec.Code != http.StatusUnprocessableEntity || body["path"] != "/role" {
		t.Fatalf("viewer in MVP: %d %v", rec.Code, body)
	}
	rec, body = f.do(f.admin, http.MethodPut, path, `{"role":"editor"}`, "")
	if rec.Code != http.StatusOK || body["role"] != "editor" {
		t.Fatalf("put: %d %v", rec.Code, body)
	}
	last := f.w.Entries[len(f.w.Entries)-1]
	if last.Action != audit.ActionRoleBindingSet || last.Object != audit.ObjectRoleBinding || last.Actor.UserID == nil {
		t.Fatalf("audit = %+v", last)
	}
	self := "/api/v1/role-bindings/" + f.admin.String() + "/" + team.String()
	rec, body = f.do(f.admin, http.MethodDelete, self, "", `"1"`)
	if rec.Code != http.StatusConflict || body["code"] != "last_admin" {
		t.Fatalf("remove last admin: %d %v", rec.Code, body)
	}
	rec, body = f.do(f.editor, http.MethodGet, "/api/v1/role-bindings?teamId="+team.String(), "", "")
	if rec.Code != http.StatusOK || len(body["items"].([]any)) != 3 || len(body["assignableRoles"].([]any)) != 2 {
		t.Fatalf("list: %d %v", rec.Code, body)
	}
}

func TestEveryRouteHasAKnownPermission(t *testing.T) {
	authModule := authhandlers.NewModule(authhandlers.Deps{Logins: auth.NewLoginManager(auth.LoginDeps{}), Config: config.Config{}})
	table := rbac.NewRuleTable(rbachttp.NewModule(rbachttp.Deps{}), authModule)
	public := []string{"GET /auth/config", "GET /auth/oidc/login", "GET /auth/oidc/callback", "POST /auth/logout", "GET /api/v1/auth/me"}
	rep := rbac.NewCoverageChecker(rbac.DefaultPermissionCatalog(), public...).Check(table)
	if !rep.OK() {
		t.Fatalf("unguarded=%v unknown=%v", rep.Unguarded, rep.Unknown)
	}
	bad := rbac.NewRuleTable(api.Modules{fakeHandler{perm: auth.Permission{Key: "route.nuke"}}, fakeHandler{pattern: "/x"}})
	rep = rbac.NewCoverageChecker(rbac.DefaultPermissionCatalog()).Check(bad)
	if len(rep.Unknown) != 1 || len(rep.Unguarded) != 1 {
		t.Fatalf("checker missed problems: %+v", rep)
	}
}

type fakeHandler struct {
	perm    auth.Permission
	pattern string
}

func (h fakeHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: "/api/v1/fake" + h.pattern, Permission: h.perm}
}

func (fakeHandler) Serve(http.ResponseWriter, *http.Request) error { return nil }
