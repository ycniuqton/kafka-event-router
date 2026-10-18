package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/auth/apitoken"
	"github.com/eventrouter/router/internal/auth/apitoken/apitokentest"
	tokenhttp "github.com/eventrouter/router/internal/auth/apitoken/httpapi"
	"github.com/eventrouter/router/internal/auth/dev"
	authhandlers "github.com/eventrouter/router/internal/auth/handlers"
	"github.com/eventrouter/router/internal/auth/session"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/platform/ratelimit"
)

// newPlatformServer mounts auth + API tokens behind the production stage order with in-memory stores.
func newPlatformServer(t *testing.T, rule ratelimit.Rule) (*httptest.Server, *memAudit) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	rdb := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	cfg := config.Config{Workspace: "acme", Auth: config.AuthConfig{Dev: config.DevLoginConfig{AdminEmail: "admin@local.dev", TeamName: "default"}}}
	ttls := session.TTLs{Idle: time.Hour, Absolute: 24 * time.Hour}
	ids := &memIdentity{users: map[string]auth.User{}, teams: map[string]uuid.UUID{}, bindings: map[uuid.UUID]auth.TeamRole{}}
	rec, jar, sessions := &memAudit{}, auth.NewCookieJar(false, ttls.Absolute), session.NewRedisStore(rdb, ttls, time.Now)
	seeder := &dev.Seeder{Users: ids, Teams: ids, Bindings: ids, Tx: noTx{}, Recorder: rec, Config: cfg.Auth.Dev}
	logins := auth.NewLoginManager(auth.LoginDeps{Users: ids, Sessions: sessions, Cookies: jar, Tx: noTx{},
		Hook: auth.NewFirstUserAdminHook(ids, "default", rec), Auditor: auth.NewLoginAuditor(rec, log)}, dev.NewFlow(seeder, cfg.Auth.Dev))
	repo, scopes := apitokentest.NewMemoryRepository(), auth.DefaultScopeCatalog()
	tokens := apitoken.NewManager(apitoken.ManagerDeps{Repo: repo, Tx: noTx{}, Scopes: scopes, Policy: apitoken.DefaultPolicy()})
	authn := auth.NewManager(session.NewCookieAuthenticator(sessions, ids, jar, ttls, time.Now),
		apitoken.NewAuthenticator(repo, ids, apitoken.NewUsageBatcher(repo, apitoken.DefaultFlushInterval, log), time.Now))
	responder := api.NewResponder(problem.NewCatalog(), log)
	audMW := audit.NewMiddleware(rec, audit.NewDiffBuilder(logging.NewRedactor()), api.AuditContext, log)
	audMW.StrictMode = true
	m := api.NewMounter(responder, []api.GlobalMiddleware{api.RequestIDMiddleware{}, api.NoStoreMiddleware{}}, []api.RouteMiddleware{
		api.AuthenticateStage{Authenticator: authn, Responder: responder},
		api.RateLimitStage{Guard: ratelimit.NewGuard(ratelimit.NewRedisLimiter(rdb), rule, log), Responder: responder},
		api.CSRFStage{CSRF: auth.NewCSRFMiddleware(jar), Responder: responder},
		api.ScopeStage{Scopes: auth.NewScopeGuard(scopes), Responder: responder},
		api.AuditStage{Audit: audMW},
	})
	router := chi.NewRouter()
	m.Install(router)
	m.Mount(router, authhandlers.NewModule(authhandlers.Deps{Logins: logins, Responder: responder, Config: cfg}),
		tokenhttp.NewModule(tokenhttp.Deps{Tokens: tokens, Responder: responder}))
	srv := httptest.NewServer(contractGuard(t, router))
	t.Cleanup(srv.Close)
	return srv, rec
}

// platformClient sends JSON with an optional bearer token and the session's CSRF token.
type platformClient struct {
	t    *testing.T
	base string
	http *http.Client
	csrf string
}

func newPlatformClient(t *testing.T, base string) *platformClient {
	jar, _ := cookiejar.New(nil)
	c := &platformClient{t: t, base: base, http: &http.Client{Jar: jar}}
	if resp := c.do(http.MethodPost, "/auth/dev/login", `{}`, ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("dev login: %d", resp.StatusCode)
	}
	var me map[string]any
	_ = json.NewDecoder(c.do(http.MethodGet, "/api/v1/auth/me", "", "").Body).Decode(&me)
	c.csrf, _ = me["csrf_token"].(string)
	return c
}

func (c *platformClient) do(method, path, body, bearer string) *http.Response {
	req, _ := http.NewRequest(method, c.base+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
		resp, err := http.DefaultClient.Do(req)
		return c.done(resp, err)
	}
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	resp, err := c.http.Do(req)
	return c.done(resp, err)
}

func (c *platformClient) done(resp *http.Response, err error) *http.Response {
	if err != nil {
		c.t.Fatal(err)
	}
	c.t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// TestAPITokenLifecycle (PC-72): create → bearer works within scope → tokens cannot manage tokens → revoke → 401.
func TestAPITokenLifecycle(t *testing.T) {
	srv, rec := newPlatformServer(t, ratelimit.Rule{PerMinute: 6000, Burst: 1000})
	c := newPlatformClient(t, srv.URL)
	resp := c.do(http.MethodPost, "/api/v1/auth/tokens", `{"name":"ci","scopes":["read"],"expires_in_days":30}`, "")
	var created struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&created)
	if resp.StatusCode != http.StatusCreated || !strings.HasPrefix(created.Token, "ert_") {
		t.Fatalf("create: %d %+v", resp.StatusCode, created)
	}
	steps := []struct {
		name, method, path, bearer string
		want                       int
	}{
		{"read via token", http.MethodGet, "/api/v1/auth/me", created.Token, http.StatusOK},
		{"write beyond scope", http.MethodPost, "/auth/logout", created.Token, http.StatusForbidden},
		{"token cannot list tokens", http.MethodGet, "/api/v1/auth/tokens", created.Token, http.StatusForbidden},
		{"session lists tokens", http.MethodGet, "/api/v1/auth/tokens", "", http.StatusOK},
		{"revoke", http.MethodDelete, "/api/v1/auth/tokens/" + created.ID, "", http.StatusNoContent},
		{"revoked token", http.MethodGet, "/api/v1/auth/me", created.Token, http.StatusUnauthorized},
		{"revoke twice", http.MethodDelete, "/api/v1/auth/tokens/" + created.ID, "", http.StatusNotFound},
	}
	for _, s := range steps {
		if got := c.do(s.method, s.path, "", s.bearer).StatusCode; got != s.want {
			t.Fatalf("%s: status %d, want %d", s.name, got, s.want)
		}
	}
	if !slices.Contains(rec.actions, apitoken.ActionCreated) || !slices.Contains(rec.actions, apitoken.ActionRevoked) {
		t.Fatalf("audit actions: %v", rec.actions)
	}
}

// TestRateLimitReturns429WithRetryAfter (PC-72): the per-principal limiter answers 429 + Retry-After.
func TestRateLimitReturns429WithRetryAfter(t *testing.T) {
	srv, _ := newPlatformServer(t, ratelimit.Rule{PerMinute: 60, Burst: 4})
	c := newPlatformClient(t, srv.URL)
	var last *http.Response
	for i := 0; i < 4; i++ {
		last = c.do(http.MethodGet, "/api/v1/auth/tokens", "", "")
	}
	var d problem.Details
	_ = json.NewDecoder(last.Body).Decode(&d)
	if last.StatusCode != http.StatusTooManyRequests || last.Header.Get("Retry-After") == "" || d.RetryAfter < 1 {
		t.Fatalf("want 429 with Retry-After, got %d %q %+v", last.StatusCode, last.Header.Get("Retry-After"), d)
	}
}
