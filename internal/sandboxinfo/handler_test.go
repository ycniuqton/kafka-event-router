package sandboxinfo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth/workload"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/secrets"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/pkg/sandbox"
)

type memSecrets struct {
	mu   sync.Mutex
	vals map[secrets.Ref][]byte
}

func (m *memSecrets) Put(_ context.Context, p []byte) (secrets.Ref, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ref := secrets.NewRef()
	m.vals[ref] = bytes.Clone(p)
	return ref, nil
}
func (m *memSecrets) Get(_ context.Context, r secrets.Ref) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.vals[r]
	if !ok {
		return nil, secrets.ErrNotFound
	}
	return bytes.Clone(v), nil
}
func (m *memSecrets) Replace(context.Context, secrets.Ref, []byte) error { return nil }
func (m *memSecrets) Delete(context.Context, secrets.Ref) error          { return nil }

type memRefs struct{ refs map[string]string }

func (m *memRefs) HashSaltRef(_ context.Context, team string) (string, bool, error) {
	if team == "unknown" {
		return "", false, store.ErrNotFound
	}
	ref, ok := m.refs[team]
	return ref, ok, nil
}
func (m *memRefs) SetHashSaltRefIfEmpty(_ context.Context, team, ref string) (string, error) {
	if cur, ok := m.refs[team]; ok {
		return cur, nil
	}
	m.refs[team] = ref
	return ref, nil
}

type memRecorder struct{ entries []audit.Entry }

func (r *memRecorder) Record(_ context.Context, e audit.Entry) error {
	r.entries = append(r.entries, e)
	return nil
}

func newRouter(t *testing.T, stages []api.RouteMiddleware, modules ...api.Module) http.Handler {
	t.Helper()
	m := api.NewMounter(testResponder(), []api.GlobalMiddleware{api.RequestIDMiddleware{}}, stages)
	r := chi.NewRouter()
	m.Install(r)
	m.Mount(r, modules...)
	return r
}

func testResponder() *api.Responder {
	return api.NewResponder(problem.NewCatalog(), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func get(h http.Handler, path string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestRuntimeInfoHandlerShapeAndETag(t *testing.T) {
	cfg := sandbox.DefaultConfig()
	mod, err := NewModule(Deps{Info: StaticInfo{Config: cfg}, Responder: testResponder()})
	if err != nil {
		t.Fatal(err)
	}
	h := newRouter(t, nil, mod)
	ok := get(h, string(PathRuntimeInfo), nil)
	etag := ok.Header().Get("ETag")
	if ok.Code != http.StatusOK || etag == "" || ok.Header().Get("Content-Type") == "" {
		t.Fatalf("200: %d etag=%q", ok.Code, etag)
	}
	var body map[string]any
	if err := json.Unmarshal(ok.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"engine", "determinismVersion", "limits", "helpers", "typings", "safetyRules"} {
		if _, has := body[key]; !has {
			t.Errorf("missing %q in %s", key, ok.Body)
		}
	}
	for _, inm := range []string{etag, "W/" + etag, `"x", ` + etag, "*"} {
		if w := get(h, string(PathRuntimeInfo), map[string]string{"If-None-Match": inm}); w.Code != http.StatusNotModified || w.Body.Len() != 0 {
			t.Fatalf("If-None-Match %q: %d", inm, w.Code)
		}
	}
	if w := get(h, string(PathRuntimeInfo), map[string]string{"If-None-Match": `"stale"`}); w.Code != http.StatusOK {
		t.Fatalf("stale tag: %d", w.Code)
	}
}

func TestTeamSaltHandlerGeneratesOnceAndAudits(t *testing.T) {
	static, _ := workload.NewStaticTokenAuthenticator("tok")
	rec := &memRecorder{}
	salts := NewTeamSalts(&memSecrets{vals: map[secrets.Ref][]byte{}}, &memRefs{refs: map[string]string{}})
	h := newRouter(t, []api.RouteMiddleware{api.AuthenticateStage{Authenticator: workload.NewManager(static), Responder: testResponder()}},
		NewInternalModule(Deps{Salts: salts, Recorder: rec, Responder: testResponder()}))
	auth := map[string]string{"Authorization": "Bearer tok"}

	first := get(h, "/internal/v1/sandbox/teams/t1/hash-salt", auth)
	second := get(h, "/internal/v1/sandbox/teams/t1/hash-salt", auth)
	var a, b TeamSaltResponse
	_ = json.Unmarshal(first.Body.Bytes(), &a)
	_ = json.Unmarshal(second.Body.Bytes(), &b)
	raw, _ := base64.StdEncoding.DecodeString(a.ValueB64)
	if first.Code != 200 || len(raw) != 32 || a.ValueB64 != b.ValueB64 || first.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("salt: %d %s / %s", first.Code, first.Body, second.Body)
	}
	other := get(h, "/internal/v1/sandbox/teams/t2/hash-salt", auth)
	if bytes.Contains(other.Body.Bytes(), []byte(a.ValueB64)) {
		t.Fatal("teams must not share a salt")
	}
	if len(rec.entries) != 3 || rec.entries[0].Action != audit.ActionSecretRead || rec.entries[0].Object != audit.ObjectTeam {
		t.Fatalf("audit: %+v", rec.entries)
	}
	if w := get(h, "/internal/v1/sandbox/teams/unknown/hash-salt", auth); w.Code != http.StatusNotFound {
		t.Fatalf("unknown team: %d", w.Code)
	}
	if w := get(h, "/internal/v1/sandbox/teams/t1/hash-salt", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", w.Code)
	}
}

func TestSettingsLoader(t *testing.T) {
	env := map[string]string{"ROUTER_SANDBOX_MAX_TIMEOUT_MS": "500", "ROUTER_SANDBOX_MAX_MEMORY_MB": "128", "ROUTER_SANDBOX_MAX_OUTPUT_KB": "64",
		"ROUTER_SANDBOX_POOL_MAX_IDLE": "2", "ROUTER_SANDBOX_CACHE_DIR": "/tmp/x", "ROUTER_SANDBOX_RESET_POLICY": "lockdown_reuse"}
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	cfg, err := SettingsLoader{Lookup: lookup}.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Maxima.Timeout != 500*time.Millisecond || cfg.Maxima.MemoryBytes != 128<<20 || cfg.Maxima.MaxOutputBytes != 64<<10 ||
		cfg.PoolMaxIdle != 2 || cfg.CacheDir != "/tmp/x" || cfg.ResetPolicy != sandbox.ResetLockdownReuse || cfg.Engine != sandbox.EngineQuickJSWazero {
		t.Fatalf("cfg: %+v", cfg)
	}
	if sandbox.LimitMaximum.Timeout == 500*time.Millisecond {
		t.Fatal("loader must not mutate the package maxima")
	}
	env["ROUTER_SANDBOX_MAX_MEMORY_MB"] = "-1"
	if _, err := (SettingsLoader{Lookup: lookup}).Load(); err == nil {
		t.Fatal("negative memory accepted")
	}
	def, err := SettingsLoader{Lookup: func(string) (string, bool) { return "", false }}.Load()
	if err != nil || def.Maxima != sandbox.DefaultConfig().Maxima {
		t.Fatalf("defaults: %+v %v", def, err)
	}
}
