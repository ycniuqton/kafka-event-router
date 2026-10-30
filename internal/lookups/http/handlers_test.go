package lookupshttp_test

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
	"github.com/eventrouter/router/internal/connections/connectionstest"
	"github.com/eventrouter/router/internal/lookups"
	lookupshttp "github.com/eventrouter/router/internal/lookups/http"
	"github.com/eventrouter/router/internal/lookups/lookupstest"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/pkg/lookup"
)

const token = "tok-S3cret"

type memRecorder struct {
	mu      sync.Mutex
	entries []audit.Entry
}

func (m *memRecorder) Record(_ context.Context, e audit.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, e)
	return nil
}

func (m *memRecorder) actions() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var b strings.Builder
	for _, e := range m.entries {
		b.WriteString(string(e.Action) + " ")
	}
	return b.String()
}

type principalStage struct{ p auth.Principal }

func (s principalStage) Wrap(_ api.RouteSpec, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), s.p)))
	})
}

type harness struct {
	srv   *httptest.Server
	usage *lookupstest.MemUsage
	audit *memRecorder
	logs  *bytes.Buffer
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{usage: &lookupstest.MemUsage{Routes: map[uuid.UUID][]lookups.RouteRef{}}, audit: &memRecorder{}, logs: &bytes.Buffer{}}
	log := slog.New(slog.NewJSONHandler(h.logs, nil))
	team := uuid.New()
	repo := lookupstest.NewMemRepo()
	repo.Teams[team] = "Platform"
	svc := lookups.NewService(lookups.ServiceDeps{Repo: repo, Usage: h.usage, Vault: lookups.NewSecretVault(connectionstest.NewMemSecrets()), Tx: connectionstest.DirectTx{}})
	responder := api.NewResponder(problem.NewCatalog(), log)
	d := lookupshttp.Deps{Service: svc, Tests: lookups.TestService{Service: svc, Tester: lookup.Tester{Factories: lookup.DefaultFactories(nil)}},
		Responder: responder, Pages: api.NewPageParser(store.NewCursorCodec("test")), Recorder: h.audit}
	auditMW := audit.NewMiddleware(h.audit, audit.NewDiffBuilder(logging.NewRedactor()), api.AuditContext, log)
	principal := auth.Principal{Kind: auth.PrincipalUser, UserID: uuid.New(), Teams: []auth.TeamRole{{TeamID: team, TeamName: "Platform", Role: auth.RoleEditor}}}
	m := api.NewMounter(responder, []api.GlobalMiddleware{api.RequestIDMiddleware{}, api.LoggingMiddleware{Log: log}},
		[]api.RouteMiddleware{principalStage{p: principal}, api.AuditStage{Audit: auditMW}})
	r := chi.NewRouter()
	m.Install(r)
	m.Mount(r, lookupshttp.NewModule(d), lookupshttp.NewInternalModule(d))
	h.srv = httptest.NewServer(r)
	t.Cleanup(h.srv.Close)
	return h
}

func (h *harness) do(t *testing.T, method, path, body string, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, h.srv.URL+path, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp, raw
}

func httpBody(baseURL string) string {
	return `{"name":"crm-api","envTag":"prod","kind":"http","http":{"baseUrl":"` + baseURL + `","auth":{"mode":"bearer"},"tls":{}},
	  "secrets":{"token":"` + token + `"}}`
}

func TestLookupSourceCRUDContract(t *testing.T) {
	h := newHarness(t)
	resp, raw := h.do(t, http.MethodPost, "/api/v1/lookup-sources", httpBody("https://crm.internal/api"), nil)
	if resp.StatusCode != http.StatusCreated || resp.Header.Get("ETag") != `"1"` || bytes.Contains(raw, []byte(token)) {
		t.Fatalf("create: %d %s", resp.StatusCode, raw)
	}
	var s lookupshttp.SourceDTO
	_ = json.Unmarshal(raw, &s)
	if s.Owner.TeamName != "Platform" || len(s.SecretFields) != 1 || s.Limits.MaxConcurrency != 16 || s.Redis != nil || !bytes.Contains(raw, []byte(`"redis":null`)) {
		t.Fatalf("resource: %s", raw)
	}
	item := "/api/v1/lookup-sources/" + s.ID.String()
	resp, raw = h.do(t, http.MethodPost, "/api/v1/lookup-sources", `{"name":"x","envTag":"dev","kind":"http","http":{"baseUrl":"ftp://x","auth":{"mode":"bearer"},"tls":{}}}`, nil)
	if resp.StatusCode != http.StatusUnprocessableEntity || !bytes.Contains(raw, []byte(`"/http/baseUrl"`)) || !bytes.Contains(raw, []byte(`"/secrets/token"`)) {
		t.Fatalf("422: %d %s", resp.StatusCode, raw)
	}
	if resp, _ = h.do(t, http.MethodPatch, item, `{"description":"x"}`, nil); resp.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("428: %d", resp.StatusCode)
	}
	resp, raw = h.do(t, http.MethodPatch, item, `{"description":"CRM","limits":{"maxConcurrency":4,"ratePerSec":50,"burst":10,"breakerFailures":3,"breakerOpenMs":5000}}`, map[string]string{"If-Match": `"1"`})
	if resp.StatusCode != http.StatusOK || resp.Header.Get("ETag") != `"2"` || !bytes.Contains(raw, []byte(`"maxConcurrency":4`)) || !bytes.Contains(raw, []byte(`"secretFields":["token"]`)) {
		t.Fatalf("patch keeps the stored token: %d %s", resp.StatusCode, raw)
	}
	resp, raw = h.do(t, http.MethodGet, "/api/v1/lookup-sources?kind=http", "", nil)
	if resp.StatusCode != http.StatusOK || !bytes.Contains(raw, []byte(`"next_cursor":null`)) || !bytes.Contains(raw, []byte(`"crm-api"`)) {
		t.Fatalf("list: %d %s", resp.StatusCode, raw)
	}
	if resp, _ = h.do(t, http.MethodGet, "/api/v1/lookup-sources?kind=ftp", "", nil); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad filter: %d", resp.StatusCode)
	}
	h.usage.Routes[s.ID] = []lookups.RouteRef{{ID: uuid.New(), Name: "orders-enrich"}}
	resp, raw = h.do(t, http.MethodDelete, item, "", nil)
	if resp.StatusCode != http.StatusConflict || !bytes.Contains(raw, []byte("lookup-source-in-use")) || !bytes.Contains(raw, []byte("orders-enrich")) {
		t.Fatalf("in use: %d %s", resp.StatusCode, raw)
	}
	delete(h.usage.Routes, s.ID)
	if resp, _ = h.do(t, http.MethodDelete, item, "", nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	for _, want := range []string{"lookup_source.created", "lookup_source.updated", "lookup_source.deleted"} {
		if !strings.Contains(h.audit.actions(), want) {
			t.Fatalf("audit %q missing in %s", want, h.audit.actions())
		}
	}
	if strings.Contains(h.logs.String(), token) {
		t.Fatal("secret leaked into logs")
	}
}

func TestLookupSourceTestAndSpec(t *testing.T) {
	h := newHarness(t)
	crm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer crm.Close()
	_, raw := h.do(t, http.MethodPost, "/api/v1/lookup-sources", httpBody(crm.URL), nil)
	var s lookupshttp.SourceDTO
	_ = json.Unmarshal(raw, &s)
	resp, raw := h.do(t, http.MethodPost, "/api/v1/lookup-sources/"+s.ID.String()+"/test", "", nil)
	if resp.StatusCode != http.StatusOK || !bytes.Contains(raw, []byte(`"ok":true`)) || !bytes.Contains(raw, []byte(`"health":"healthy"`)) {
		t.Fatalf("saved test: %d %s", resp.StatusCode, raw)
	}
	_, raw = h.do(t, http.MethodGet, "/api/v1/lookup-sources/"+s.ID.String(), "", nil)
	if !bytes.Contains(raw, []byte(`"status":"healthy"`)) || !bytes.Contains(raw, []byte(`"version":1`)) {
		t.Fatalf("health stored: %s", raw)
	}
	draft := strings.Replace(httpBody(crm.URL), token, "wrong", 1)
	resp, raw = h.do(t, http.MethodPost, "/api/v1/lookup-sources/test", draft, nil)
	if resp.StatusCode != http.StatusOK || !bytes.Contains(raw, []byte(`"health":"auth_failed"`)) || !bytes.Contains(raw, []byte(`"state":"skipped"`)) {
		t.Fatalf("draft test: %d %s", resp.StatusCode, raw)
	}
	resp, raw = h.do(t, http.MethodGet, "/internal/v1/lookup-sources/"+s.ID.String()+"/spec", "", nil)
	var spec lookup.SourceSpec
	_ = json.Unmarshal(raw, &spec)
	if resp.StatusCode != http.StatusOK || spec.Secrets.Token != token || spec.HTTP.BaseURL != crm.URL || spec.Kind != lookup.KindHTTP {
		t.Fatalf("spec: %d %s", resp.StatusCode, raw)
	}
	if !strings.Contains(h.audit.actions(), string(audit.ActionSecretRead)) || !strings.Contains(h.audit.actions(), string(lookups.ActionTested)) {
		t.Fatalf("audit: %s", h.audit.actions())
	}
}
