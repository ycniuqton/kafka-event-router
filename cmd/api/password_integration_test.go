package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
	"github.com/redis/go-redis/v9"

	"github.com/eventrouter/router/internal/auth/password"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/app"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/platform/metrics"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/internal/store/identity"
	"github.com/eventrouter/router/internal/testkit"
)

// pwWorld is an API built from the production wiring steps over a testkit Postgres sandbox and Redis.
type pwWorld struct {
	t      *testing.T
	srv    *httptest.Server
	router http.Handler
	wiring *Wiring
	db     *store.DB
	rdb    *redis.Client
	admin  string
	emails []string
}

const pwBootstrapPassword = "bootstrap-password-1"

func newPWWorld(t *testing.T) *pwWorld {
	mgr := testkit.NewManager()
	sb := mgr.Postgres(t)
	sb.Migrate(t)
	rdb := pwRedis(t, mgr)
	tag := strings.ToLower(ulid.Make().String())
	world := &pwWorld{t: t, db: sb.DB, rdb: rdb, admin: "admin-" + tag + "@example.com"}
	cfg := pwConfig(t, world.admin)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deps := &app.Deps{Config: cfg, Log: log, DB: sb.DB, Redis: rdb}
	w := &Wiring{Deps: deps, Tx: store.NewTxManager(sb.DB), Notifier: store.NewNotifier(sb.DB), Identity: identity.New(sb.DB)}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	for _, step := range []func(context.Context) error{w.buildAudit, w.buildPlatform, w.buildAuth, w.buildAccess, w.buildPassword,
		w.buildMounters(metrics.NewRegistry()), w.buildModules} {
		if err := step(ctx); err != nil {
			t.Fatal(err)
		}
	}
	router := chi.NewRouter()
	w.Public.Install(router)
	w.Public.Mount(router, w.AuthModule, w.Access.Module)
	world.wiring, world.router, world.srv = w, router, httptest.NewServer(contractGuard(t, router))
	world.emails = append(world.emails, world.admin)
	t.Cleanup(world.cleanup)
	return world
}

func pwRedis(t *testing.T, mgr *testkit.Manager) *redis.Client {
	ep := mgr.Require(t, testkit.ServiceRedis)
	opts, err := redis.ParseURL(ep.URL)
	if err != nil {
		t.Fatal(err)
	}
	return redis.NewClient(opts)
}

func pwConfig(t *testing.T, admin string) config.Config {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	env := map[string]string{"ROUTER_ENV": "staging", "ROUTER_AUTH_MODES": "password,api_token", "ROUTER_COOKIE_SECURE": "false",
		"ROUTER_COOKIE_SECRET": "it-cookie-secret", "ROUTER_CURSOR_SECRET": "it-cursor-secret", "ROUTER_SECRETS_PROVIDER": "static_key",
		"ROUTER_SECRETS_MASTER_KEY": base64.StdEncoding.EncodeToString(key), "ROUTER_BOOTSTRAP_ADMIN_EMAIL": admin,
		"ROUTER_BOOTSTRAP_ADMIN_PASSWORD": pwBootstrapPassword, "ROUTER_LOGIN_MAX_FAILURES": "3", "ROUTER_LOGIN_LOCKOUT": "2m",
		"ROUTER_LOGIN_IP_PER_MINUTE": "1000"}
	cfg, err := config.NewLoaderWithLookup(func(k string) (string, bool) { v, ok := env[k]; return v, ok }).Load(config.ServiceAPI)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// cleanup removes the Redis keys this test created (sandbox schema is dropped by testkit).
func (w *pwWorld) cleanup() {
	ctx := context.Background()
	rows, err := w.db.Pool().Query(ctx, `SELECT id FROM users`)
	if err == nil {
		for rows.Next() {
			var id uuid.UUID
			if rows.Scan(&id) == nil {
				_ = w.wiring.Sessions.RevokeAllForUser(ctx, id)
			}
		}
		rows.Close()
	}
	for _, email := range w.emails {
		sum := sha256.Sum256([]byte(strings.ToLower(email)))
		w.rdb.Del(ctx, string(password.KeyLoginFailures)+hex.EncodeToString(sum[:]))
	}
	w.srv.Close()
	_ = w.rdb.Close()
}

func (w *pwWorld) email(name string) string {
	e := name + "-" + strings.ToLower(ulid.Make().String()) + "@example.com"
	w.emails = append(w.emails, e)
	return e
}

// pwClient is one browser: cookie jar plus the CSRF token of its current session.
type pwClient struct {
	*cookieClient
	csrf string
}

func (w *pwWorld) client() *pwClient {
	return &pwClient{cookieClient: newCookieClient(w.t, w.srv.URL)}
}

func (c *pwClient) login(email, pw string) *http.Response {
	body, _ := json.Marshal(map[string]string{"email": email, "password": pw})
	resp := c.do(http.MethodPost, "/auth/password/login", string(body), "")
	if resp.StatusCode == http.StatusNoContent {
		c.refresh()
	}
	return resp
}

func (c *pwClient) refresh() map[string]any {
	me := pwJSON(c.t, c.do(http.MethodGet, "/api/v1/auth/me", "", ""))
	c.csrf, _ = me["csrf_token"].(string)
	return me
}

func (c *pwClient) send(method, path, body string) *http.Response {
	return c.do(method, path, body, c.csrf)
}

func pwJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out
}

func pwExpect(t *testing.T, resp *http.Response, status int, code string) map[string]any {
	t.Helper()
	body := pwJSON(t, resp)
	if resp.StatusCode != status || (code != "" && body["code"] != code) {
		t.Fatalf("%s %s: status=%d want %d code=%v want %q body=%v", resp.Request.Method, resp.Request.URL.Path,
			resp.StatusCode, status, body["code"], code, body)
	}
	return body
}

// TestIntegrationPasswordBootstrapAndForcedChange covers config, first-start admin, forced change, session renewal and reseed.
func TestIntegrationPasswordBootstrapAndForcedChange(t *testing.T) {
	w := newPWWorld(t)
	anon := w.client()
	cfg := pwExpect(t, anon.do(http.MethodGet, "/auth/config", "", ""), http.StatusOK, "")
	if cfg["password_login_url"] != "/auth/password/login" || !strings.Contains(strings.Join(pwStrings(cfg["modes"]), ","), "password") {
		t.Fatalf("config %v", cfg)
	}
	admin, other := w.client(), w.client()
	pwExpect(t, admin.login(w.admin, pwBootstrapPassword), http.StatusNoContent, "")
	pwExpect(t, other.login(strings.ToUpper(w.admin), pwBootstrapPassword), http.StatusNoContent, "")
	if me := admin.refresh(); me["must_change_password"] != true {
		t.Fatalf("bootstrap admin must change its password: %v", me)
	}
	pwExpect(t, admin.send(http.MethodGet, "/api/v1/users", ""), http.StatusForbidden, string(password.CodeChangeRequired))
	pwExpect(t, admin.send(http.MethodPost, "/api/v1/auth/password", `{"current_password":"nope-nope-nope","new_password":"admin-new-password"}`),
		http.StatusUnprocessableEntity, string(password.CodeInvalidCurrentPassword))
	pwExpect(t, admin.send(http.MethodPost, "/api/v1/auth/password", `{"current_password":"`+pwBootstrapPassword+`","new_password":"short"}`),
		http.StatusUnprocessableEntity, string(password.CodeWeakPassword))
	pwExpect(t, admin.send(http.MethodPost, "/api/v1/auth/password", `{"current_password":"`+pwBootstrapPassword+`","new_password":"admin-new-password"}`),
		http.StatusNoContent, "")
	if me := admin.refresh(); me["must_change_password"] != false {
		t.Fatalf("change must clear the flag: %v", me)
	}
	pwExpect(t, admin.send(http.MethodGet, "/api/v1/users", ""), http.StatusOK, "")
	pwExpect(t, other.do(http.MethodGet, "/api/v1/auth/me", "", ""), http.StatusUnauthorized, "")
	pwExpect(t, w.client().login(w.admin, pwBootstrapPassword), http.StatusUnauthorized, string(password.CodeInvalidCredentials))
	if err := w.wiring.buildPassword(context.Background()); err != nil {
		t.Fatal(err)
	}
	pwExpect(t, w.client().login(w.admin, "admin-new-password"), http.StatusNoContent, "")
	if n := pwCount(t, w.db, `SELECT count(*) FROM user_passwords`); n != 1 {
		t.Fatalf("reseed created credentials: %d", n)
	}
	if n := pwCount(t, w.db, `SELECT count(*) FROM audit_log WHERE action = 'auth.password_changed'`); n != 1 {
		t.Fatalf("password change audited %d times", n)
	}
}

// TestIntegrationPasswordLoginFailuresLockAccount covers generic failures, lockout and the login audit.
func TestIntegrationPasswordLoginFailuresLockAccount(t *testing.T) {
	w := newPWWorld(t)
	c := w.client()
	for i := 0; i < 3; i++ {
		pwExpect(t, c.login(w.admin, "wrong-password-xx"), http.StatusUnauthorized, string(password.CodeInvalidCredentials))
	}
	pwExpect(t, c.login(w.email("nobody"), "wrong-password-xx"), http.StatusUnauthorized, string(password.CodeInvalidCredentials))
	resp := c.login(w.admin, pwBootstrapPassword)
	if resp.Header.Get("Retry-After") == "" {
		t.Fatal("locked login must carry Retry-After")
	}
	pwExpect(t, resp, http.StatusTooManyRequests, string(password.CodeLoginLocked))
	pwExpect(t, c.login(w.admin, ""), http.StatusUnprocessableEntity, "")
	if n := pwCount(t, w.db, `SELECT count(*) FROM audit_log WHERE action = 'auth.login_failed'`); n < 5 {
		t.Fatalf("login failures audited %d times", n)
	}
	if n := pwCount(t, w.db, `SELECT count(*) FROM audit_log WHERE action = 'auth.login'`); n != 0 {
		t.Fatalf("no successful login expected, got %d", n)
	}
}

// TestIntegrationPasswordAdministration covers admin create, duplicate, reset, disable/enable and session revocation.
func TestIntegrationPasswordAdministration(t *testing.T) {
	w := newPWWorld(t)
	admin := w.activeAdmin()
	email := w.email("dana")
	created := pwExpect(t, admin.send(http.MethodPost, "/api/v1/users", `{"email":"`+email+`","displayName":"Dana"}`), http.StatusCreated, "")
	generated, _ := created["generatedPassword"].(string)
	user, _ := created["user"].(map[string]any)
	if len(generated) < 12 || user["passwordLogin"] != true || user["mustChangePassword"] != true {
		t.Fatalf("created %v", created)
	}
	id := user["id"].(string)
	pwExpect(t, admin.send(http.MethodPost, "/api/v1/users", `{"email":"`+strings.ToUpper(email)+`"}`), http.StatusConflict, "user_exists")
	pwExpect(t, admin.send(http.MethodPost, "/api/v1/users", `{"email":"`+w.email("weak")+`","password":"short"}`),
		http.StatusUnprocessableEntity, string(password.CodeWeakPassword))
	dana := w.client()
	pwExpect(t, dana.login(email, generated), http.StatusNoContent, "")
	pwExpect(t, dana.send(http.MethodPost, "/api/v1/auth/password", `{"current_password":"`+generated+`","new_password":"dana-own-password"}`),
		http.StatusNoContent, "")
	dana.refresh()
	pwExpect(t, dana.send(http.MethodPost, "/api/v1/users", `{"email":"`+w.email("sneaky")+`"}`), http.StatusForbidden, "")
	reset := pwExpect(t, admin.send(http.MethodPost, "/api/v1/users/"+id+"/password-reset", `{}`), http.StatusOK, "")
	pwExpect(t, dana.do(http.MethodGet, "/api/v1/auth/me", "", ""), http.StatusUnauthorized, "")
	pwExpect(t, w.client().login(email, "dana-own-password"), http.StatusUnauthorized, string(password.CodeInvalidCredentials))
	pwExpect(t, dana.login(email, reset["generatedPassword"].(string)), http.StatusNoContent, "")
	if me := dana.refresh(); me["must_change_password"] != true {
		t.Fatalf("reset must force a change: %v", me)
	}
	w.setDisabled(admin, id, true)
	pwExpect(t, dana.do(http.MethodGet, "/api/v1/auth/me", "", ""), http.StatusUnauthorized, "")
	pwExpect(t, w.client().login(email, reset["generatedPassword"].(string)), http.StatusForbidden, "")
	w.setDisabled(admin, id, false)
	pwExpect(t, w.client().login(email, reset["generatedPassword"].(string)), http.StatusNoContent, "")
	for action, want := range map[string]int{"user.created": 1, "user.password_reset": 1, "user.updated": 2} {
		if n := pwCount(t, w.db, `SELECT count(*) FROM audit_log WHERE action = $1`, action); n != want {
			t.Errorf("%s audited %d times, want %d", action, n, want)
		}
	}
}

// TestIntegrationPasswordNoSelfRegistration checks that no route creates a user without an admin session.
func TestIntegrationPasswordNoSelfRegistration(t *testing.T) {
	w := newPWWorld(t)
	anon := w.client()
	body := `{"email":"` + w.email("self") + `","password":"self-chosen-password"}`
	for _, path := range []string{"/auth/password/register", "/auth/register", "/auth/password/signup", "/api/v1/auth/register"} {
		rec := httptest.NewRecorder()
		w.router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
	for _, h := range append(w.wiring.AuthModule.Handlers(), w.wiring.Access.Module.Handlers()...) {
		spec := h.Route()
		if key := spec.Method + " " + spec.Pattern; spec.Auth == api.AuthNone && !pwPublicRoutes[key] {
			t.Errorf("unexpected unauthenticated route %s", key)
		}
	}
	pwExpect(t, anon.do(http.MethodPost, "/api/v1/users", body, ""), http.StatusUnauthorized, "")
	pwExpect(t, anon.login(w.email("self"), "self-chosen-password"), http.StatusUnauthorized, string(password.CodeInvalidCredentials))
	if n := pwCount(t, w.db, `SELECT count(*) FROM users`); n != 1 {
		t.Fatalf("only the bootstrap admin may exist, got %d users", n)
	}
}

// activeAdmin signs in the bootstrap admin and completes its forced change.
func (w *pwWorld) activeAdmin() *pwClient {
	admin := w.client()
	pwExpect(w.t, admin.login(w.admin, pwBootstrapPassword), http.StatusNoContent, "")
	pwExpect(w.t, admin.send(http.MethodPost, "/api/v1/auth/password", `{"current_password":"`+pwBootstrapPassword+`","new_password":"admin-new-password"}`),
		http.StatusNoContent, "")
	admin.refresh()
	return admin
}

func (w *pwWorld) setDisabled(admin *pwClient, id string, disabled bool) {
	resp := admin.send(http.MethodGet, "/api/v1/users/"+id, "")
	etag := resp.Header.Get("ETag")
	pwExpect(w.t, resp, http.StatusOK, "")
	req, _ := http.NewRequest(http.MethodPatch, w.srv.URL+"/api/v1/users/"+id, strings.NewReader(map[bool]string{true: `{"disabled":true}`, false: `{"disabled":false}`}[disabled]))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", admin.csrf)
	req.Header.Set("If-Match", etag)
	patched, err := admin.http.Do(req)
	if err != nil {
		w.t.Fatal(err)
	}
	defer patched.Body.Close()
	pwExpect(w.t, patched, http.StatusOK, "")
}

// pwPublicRoutes are the only routes reachable without a session in password mode (none creates users).
var pwPublicRoutes = map[string]bool{"GET /auth/config": true, "GET /auth/oidc/login": true, "GET /auth/oidc/callback": true,
	"POST /auth/password/login": true}

func pwCount(t *testing.T, db *store.DB, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := db.Pool().QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func pwStrings(v any) []string {
	items, _ := v.([]any)
	out := []string{}
	for _, i := range items {
		s, _ := i.(string)
		out = append(out, s)
	}
	return out
}
