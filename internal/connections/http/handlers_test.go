package connhttp_test

import (
	"bufio"
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
	"github.com/twmb/franz-go/pkg/kfake"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/connections/checks"
	"github.com/eventrouter/router/internal/connections/connectionstest"
	connhttp "github.com/eventrouter/router/internal/connections/http"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/pkg/kafka"
)

const secretPassword = "s3cr3t-Pa55word"

// memRecorder collects audit entries.
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

// principalStage injects a fixed editor principal.
type principalStage struct{ p auth.Principal }

func (s principalStage) Wrap(_ api.RouteSpec, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), s.p)))
	})
}

type harness struct {
	srv   *httptest.Server
	usage *connectionstest.MemUsage
	audit *memRecorder
	logs  *bytes.Buffer
	team  uuid.UUID
	repo  *connectionstest.MemRepo
	vault *connections.SecretVault
}

// harnessOptions adds extra team bindings and an authorization stage (RBAC tests).
type harnessOptions struct {
	extraTeams []auth.TeamRole
	authz      auth.Authorizer
}

func newHarness(t *testing.T) *harness {
	return newHarnessWith(t, harnessOptions{})
}

func newHarnessWith(t *testing.T, opts harnessOptions) *harness {
	t.Helper()
	h := &harness{usage: &connectionstest.MemUsage{Routes: map[uuid.UUID][]connections.RouteRef{}}, audit: &memRecorder{}, logs: &bytes.Buffer{}, team: uuid.New()}
	log := slog.New(slog.NewJSONHandler(h.logs, nil))
	repo := connectionstest.NewMemRepo()
	repo.Teams[h.team] = "Platform"
	vault := connections.NewSecretVault(connectionstest.NewMemSecrets())
	h.repo, h.vault = repo, vault
	validator := connections.DefaultValidator(kafka.DefaultSecurityRegistry(), kafka.DefaultPropertyCatalog())
	resolver := connections.NewSpecResolver(repo, vault)
	clients := kafka.DefaultClientFactoryRegistry()
	failures := connections.ClusterFailures{Classifier: kafka.DefaultFailureClassifier(), Hints: connections.DefaultHintCatalog()}
	runner := connections.NewCheckRunner(checks.DefaultRegistry(), failures.Classifier, failures.Hints, nil)
	responder := api.NewResponder(problem.NewCatalog(), log)
	d := connhttp.Deps{
		Service:  connections.NewService(connections.ServiceDeps{Repo: repo, Usage: h.usage, Vault: vault, Validator: validator, Tx: connectionstest.DirectTx{}}),
		Tester:   connections.NewTester(connections.TesterDeps{Repo: repo, Resolver: resolver, Vault: vault, Validator: validator, Clients: clients, Runner: runner, Plan: connections.DefaultTestPlan}),
		Topics:   connections.NewTopicBrowser(resolver, clients, connectionstest.MemRates{}, failures),
		Creator:  connections.NewTopicCreator(resolver, clients, failures),
		Resolver: resolver, Responder: responder, Pages: api.NewPageParser(store.NewCursorCodec("test")), Recorder: h.audit,
		Authz: opts.authz,
	}
	auditMW := audit.NewMiddleware(h.audit, audit.NewDiffBuilder(logging.NewRedactor()), api.AuditContext, log)
	teams := append([]auth.TeamRole{{TeamID: h.team, TeamName: "Platform", Role: auth.RoleEditor}}, opts.extraTeams...)
	principal := auth.Principal{Kind: auth.PrincipalUser, UserID: uuid.New(), Teams: teams}
	stages := []api.RouteMiddleware{principalStage{p: principal}}
	if opts.authz != nil {
		stages = append(stages, api.AuthorizeStage{Authorize: auth.NewAuthorizeMiddleware(opts.authz), Responder: responder})
	}
	stages = append(stages, api.AuditStage{Audit: auditMW})
	m := api.NewMounter(responder, []api.GlobalMiddleware{api.RequestIDMiddleware{}, api.LoggingMiddleware{Log: log}}, stages)
	r := chi.NewRouter()
	m.Install(r)
	m.Mount(r, connhttp.NewModule(d), connhttp.NewInternalModule(d))
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

func createBody(seeds string) string {
	return `{"name":"orders-prod","envTag":"prod","bootstrapServers":[` + seeds + `],
	  "security":{"mode":"sasl_scram_512","username":"svc"},"secrets":{"password":"` + secretPassword + `"},
	  "properties":{"client.id":"router"}}`
}

func TestCRUDContract(t *testing.T) {
	h := newHarness(t)
	resp, raw := h.do(t, http.MethodPost, "/api/v1/connections", createBody(`"kafka-1:9093"`), nil)
	if resp.StatusCode != http.StatusCreated || resp.Header.Get("ETag") != `"1"` || resp.Header.Get("Location") == "" {
		t.Fatalf("create: %d %s", resp.StatusCode, raw)
	}
	var c connhttp.ConnectionDTO
	_ = json.Unmarshal(raw, &c)
	if c.Security.SecretFields[kafka.SecretPassword] != connhttp.SecretSet || c.Owner.TeamName != "Platform" || c.Health.Status != connections.HealthUnknown {
		t.Fatalf("resource: %s", raw)
	}
	item := "/api/v1/connections/" + c.ID.String()

	resp, raw = h.do(t, http.MethodPost, "/api/v1/connections", createBody(`"kafka-1:9093","bad"`), nil)
	if resp.StatusCode != http.StatusUnprocessableEntity || !bytes.Contains(raw, []byte(`"path":"/bootstrapServers/1"`)) {
		t.Fatalf("422: %d %s", resp.StatusCode, raw)
	}
	resp, raw = h.do(t, http.MethodPost, "/api/v1/connections", createBody(`"kafka-1:9093"`), nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("409 name: %d %s", resp.StatusCode, raw)
	}
	resp, _ = h.do(t, http.MethodPatch, item, `{"description":"x"}`, nil)
	if resp.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("428: %d", resp.StatusCode)
	}
	resp, raw = h.do(t, http.MethodPatch, item, `{"description":"orders","secrets":{"password":"other"}}`, map[string]string{"If-Match": `"1"`})
	if resp.StatusCode != http.StatusOK || resp.Header.Get("ETag") != `"2"` {
		t.Fatalf("patch: %d %s", resp.StatusCode, raw)
	}
	resp, _ = h.do(t, http.MethodPatch, item, `{"description":"again"}`, map[string]string{"If-Match": `"1"`})
	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("412: %d", resp.StatusCode)
	}
	resp, raw = h.do(t, http.MethodPatch, item, `{"security":{"username":null}}`, map[string]string{"If-Match": `"2"`})
	if resp.StatusCode != http.StatusUnprocessableEntity || !bytes.Contains(raw, []byte(`"path":"/security/username"`)) {
		t.Fatalf("patch 422: %d %s", resp.StatusCode, raw)
	}
	resp, raw = h.do(t, http.MethodGet, "/api/v1/connections?env=prod&limit=10", "", nil)
	if resp.StatusCode != http.StatusOK || !bytes.Contains(raw, []byte(`"next_cursor":null`)) {
		t.Fatalf("list: %d %s", resp.StatusCode, raw)
	}
	resp, _ = h.do(t, http.MethodGet, "/api/v1/connections?env=qa", "", nil)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad filter: %d", resp.StatusCode)
	}

	h.usage.Routes[c.ID] = []connections.RouteRef{{ID: uuid.New(), Name: "order-paid-to-partner"}}
	resp, raw = h.do(t, http.MethodDelete, item, "", nil)
	if resp.StatusCode != http.StatusConflict || !bytes.Contains(raw, []byte(`"routes":[{"id"`)) || !bytes.Contains(raw, []byte("connection-in-use")) {
		t.Fatalf("in use: %d %s", resp.StatusCode, raw)
	}
	delete(h.usage.Routes, c.ID)
	resp, _ = h.do(t, http.MethodDelete, item, "", nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	resp, _ = h.do(t, http.MethodGet, item, "", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("gone: %d", resp.StatusCode)
	}
}

func TestDraftTestStreamsNDJSON(t *testing.T) {
	kc, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(1, "orders"))
	if err != nil {
		t.Fatal(err)
	}
	defer kc.Close()
	h := newHarness(t)
	body := `{"name":"draft","envTag":"dev","bootstrapServers":["` + kc.ListenAddrs()[0] + `"],"security":{"mode":"plaintext"},"writeTopic":"orders"}`
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/api/v1/connections/test", strings.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Content-Type") != "application/x-ndjson" {
		t.Fatalf("content type %q", resp.Header.Get("Content-Type"))
	}
	var lines []map[string]any
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var m map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &m); err != nil {
			t.Fatalf("line %q: %v", scanner.Text(), err)
		}
		lines = append(lines, m)
	}
	last := lines[len(lines)-1]
	if last["type"] != "result" || last["ok"] != true || last["health"] != "healthy" || len(lines) != 11 {
		t.Fatalf("stream: %v", lines)
	}
	resp2, raw := h.do(t, http.MethodPost, "/api/v1/connections/test", `{"name":"x","envTag":"dev","bootstrapServers":[],"security":{"mode":"plaintext"}}`, nil)
	if resp2.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid draft must be a problem before streaming: %d %s", resp2.StatusCode, raw)
	}
}

func TestSecretsNeverDisclosed(t *testing.T) {
	h := newHarness(t)
	_, raw := h.do(t, http.MethodPost, "/api/v1/connections", createBody(`"kafka-1:9093"`), nil)
	var c connhttp.ConnectionDTO
	_ = json.Unmarshal(raw, &c)
	item := "/api/v1/connections/" + c.ID.String()
	bodies := [][]byte{raw}
	_, got := h.do(t, http.MethodGet, item, "", nil)
	bodies = append(bodies, got)
	_, got = h.do(t, http.MethodGet, "/api/v1/connections", "", nil)
	bodies = append(bodies, got)
	_, got = h.do(t, http.MethodPatch, item, `{"secrets":{"password":"`+secretPassword+`-2"}}`, map[string]string{"If-Match": `"1"`})
	bodies = append(bodies, got)
	for _, b := range bodies {
		if bytes.Contains(b, []byte(secretPassword)) {
			t.Fatalf("response leaks secret: %s", b)
		}
	}
	if bytes.Contains(h.logs.Bytes(), []byte(secretPassword)) {
		t.Fatal("logs leak secret")
	}
	for _, e := range h.audit.entries {
		if bytes.Contains(e.Diff, []byte(secretPassword)) {
			t.Fatalf("audit diff leaks secret: %s", e.Diff)
		}
	}
	before := len(h.audit.entries)
	resp, spec := h.do(t, http.MethodGet, "/internal/v1/connections/"+c.ID.String()+"/spec", "", nil)
	if resp.StatusCode != http.StatusOK || !bytes.Contains(spec, []byte(secretPassword+"-2")) || len(h.audit.entries) != before+1 {
		t.Fatalf("internal spec: %d %s", resp.StatusCode, spec)
	}
	if len(h.audit.entries) < 3 {
		t.Fatalf("expected created/updated/secret_replaced entries, got %d", len(h.audit.entries))
	}
}
