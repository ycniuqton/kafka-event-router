package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/auth/dev"
	authhandlers "github.com/eventrouter/router/internal/auth/handlers"
	"github.com/eventrouter/router/internal/auth/session"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/platform/problem"
)

// memIdentity is an in-memory stand-in for internal/store/identity.
type memIdentity struct {
	mu       sync.Mutex
	users    map[string]auth.User
	teams    map[string]uuid.UUID
	bindings map[uuid.UUID]auth.TeamRole
}

func (m *memIdentity) Upsert(_ context.Context, id auth.Identity) (auth.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id.Subject]
	if !ok {
		u = auth.User{ID: uuid.New(), Email: id.Email, DisplayName: id.DisplayName}
		m.users[id.Subject] = u
	}
	return u, nil
}

func (m *memIdentity) LoadUser(_ context.Context, id uuid.UUID) (auth.User, []auth.TeamRole, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			return u, []auth.TeamRole{m.bindings[id]}, nil
		}
	}
	return auth.User{}, nil, auth.ErrSessionNotFound
}

func (m *memIdentity) EnsureByName(_ context.Context, name string) (uuid.UUID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.teams[name]; !ok {
		m.teams[name] = uuid.New()
	}
	return m.teams[name], nil
}

func (m *memIdentity) Ensure(_ context.Context, userID, teamID uuid.UUID, role auth.Role) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.bindings[userID]; ok {
		return false, nil
	}
	m.bindings[userID] = auth.TeamRole{TeamID: teamID, TeamName: "default", Role: role}
	return true, nil
}

func (m *memIdentity) BindFirstAdmin(context.Context, auth.User, string) (bool, error) {
	return false, nil
}

type noTx struct{}

func (noTx) WithinTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type memAudit struct {
	mu      sync.Mutex
	actions []audit.Action
}

func (m *memAudit) Record(_ context.Context, e audit.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.actions = append(m.actions, e.Action)
	return nil
}

func newTestServer(t *testing.T) (*httptest.Server, *memAudit) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	rdb := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	cfg := config.Config{Workspace: "acme", Auth: config.AuthConfig{Dev: config.DevLoginConfig{AdminEmail: "admin@local.dev", TeamName: "default"}}}
	ttls := session.TTLs{Idle: time.Hour, Absolute: 24 * time.Hour}
	ids := &memIdentity{users: map[string]auth.User{}, teams: map[string]uuid.UUID{}, bindings: map[uuid.UUID]auth.TeamRole{}}
	rec := &memAudit{}
	jar := auth.NewCookieJar(false, ttls.Absolute)
	sessions := session.NewRedisStore(rdb, ttls, time.Now)
	seeder := &dev.Seeder{Users: ids, Teams: ids, Bindings: ids, Tx: noTx{}, Recorder: rec, Config: cfg.Auth.Dev}
	logins := auth.NewLoginManager(auth.LoginDeps{Users: ids, Sessions: sessions, Cookies: jar, Tx: noTx{},
		Hook: auth.NewFirstUserAdminHook(ids, "default", rec), Auditor: auth.NewLoginAuditor(rec, log)}, dev.NewFlow(seeder, cfg.Auth.Dev))
	responder := api.NewResponder(problem.NewCatalog(), log)
	audMW := audit.NewMiddleware(rec, audit.NewDiffBuilder(nil), api.AuditContext, log)
	audMW.StrictMode = true
	m := api.NewMounter(responder, []api.GlobalMiddleware{api.RequestIDMiddleware{}, api.NoStoreMiddleware{}}, []api.RouteMiddleware{
		api.AuthenticateStage{Authenticator: auth.NewManager(session.NewCookieAuthenticator(sessions, ids, jar, ttls, time.Now)), Responder: responder},
		api.CSRFStage{CSRF: auth.NewCSRFMiddleware(jar), Responder: responder},
		api.AuditStage{Audit: audMW},
	})
	router := chi.NewRouter()
	m.Install(router)
	m.Mount(router, authhandlers.NewModule(authhandlers.Deps{Logins: logins, Responder: responder, Config: cfg}))
	srv := httptest.NewServer(contractGuard(t, router))
	t.Cleanup(srv.Close)
	return srv, rec
}

func TestDevLoginMeLogout(t *testing.T) {
	srv, rec := newTestServer(t)
	do := newCookieClient(t, srv.URL).do
	if resp := do(http.MethodGet, "/api/v1/auth/me", "", ""); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me before login: %d", resp.StatusCode)
	}
	if resp := do(http.MethodPost, "/auth/dev/login", `{"email":"dev@local.dev"}`, ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("dev login: %d", resp.StatusCode)
	}
	resp := do(http.MethodGet, "/api/v1/auth/me", "", "")
	var me map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&me)
	if resp.StatusCode != 200 || me["email"] != "dev@local.dev" || me["workspace"] != "acme" || me["initials"] != "D" || me["csrf_token"] == "" {
		t.Fatalf("me: %d %v", resp.StatusCode, me)
	}
	if teams, _ := me["teams"].([]any); len(teams) != 1 {
		t.Fatalf("teams: %v", me["teams"])
	}
	if resp := do(http.MethodPost, "/auth/logout", "", ""); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("logout without csrf must be rejected: %d", resp.StatusCode)
	}
	if resp := do(http.MethodPost, "/auth/logout", "", me["csrf_token"].(string)); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout: %d", resp.StatusCode)
	}
	if resp := do(http.MethodGet, "/api/v1/auth/me", "", ""); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me after logout: %d", resp.StatusCode)
	}
	want := []audit.Action{audit.ActionDevSeed, audit.ActionAuthLogin, audit.ActionAuthLogout}
	if len(rec.actions) != len(want) {
		t.Fatalf("audit actions: %v", rec.actions)
	}
	for i := range want {
		if rec.actions[i] != want[i] {
			t.Fatalf("audit actions: %v", rec.actions)
		}
	}
}

func TestAuthConfigAndBadDevEmail(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Get(srv.URL + "/auth/config")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body authhandlers.ConfigResponse
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Modes) != 1 || body.DevLoginURL != "/auth/dev/login" || body.OIDCLoginURL != "" {
		t.Fatalf("config: %+v", body)
	}
	bad, err := http.Post(srv.URL+"/auth/dev/login", "application/json", strings.NewReader(`{"email":"nope"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Body.Close()
	var d problem.Details
	_ = json.NewDecoder(bad.Body).Decode(&d)
	if bad.StatusCode != 422 || d.Path != "/email" {
		t.Fatalf("bad email: %d %+v", bad.StatusCode, d)
	}
	unknown, err := http.Post(srv.URL+"/auth/dev/login", "application/json", strings.NewReader(`{"emial":"x@y"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer unknown.Body.Close()
	if unknown.StatusCode != 400 {
		t.Fatalf("unknown field: %d", unknown.StatusCode)
	}
}

// cookieClient is a browser-like client (cookie jar) sending JSON with an optional CSRF header.
type cookieClient struct {
	t    *testing.T
	base string
	http *http.Client
}

func newCookieClient(t *testing.T, base string) *cookieClient {
	jar, _ := cookiejar.New(nil)
	return &cookieClient{t: t, base: base, http: &http.Client{Jar: jar}}
}

func (c *cookieClient) do(method, path, body, csrf string) *http.Response {
	req, _ := http.NewRequest(method, c.base+path, strings.NewReader(body))
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	c.t.Cleanup(func() { resp.Body.Close() })
	return resp
}
