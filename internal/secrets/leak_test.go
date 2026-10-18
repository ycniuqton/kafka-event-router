package secrets_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/connections/checks"
	connhttp "github.com/eventrouter/router/internal/connections/http"
	"github.com/eventrouter/router/internal/connections/pgstore"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/secrets"
	"github.com/eventrouter/router/internal/secrets/local"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/internal/testkit"
	"github.com/eventrouter/router/pkg/kafka"
)

// Distinctive secret values; none of them may appear in logs, audit rows, connection rows or responses.
var leakCanaries = []string{"Canary-Create-pw-7f3a", "Canary-Patch-pw-91be", "Canary-Draft-pw-c4d2"}

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mu  chan struct{}
	buf bytes.Buffer
}

func newSyncBuffer() *syncBuffer { return &syncBuffer{mu: make(chan struct{}, 1)} }

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu <- struct{}{}
	defer func() { <-b.mu }()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu <- struct{}{}
	defer func() { <-b.mu }()
	return b.buf.String()
}

// principalStage injects a fixed editor principal (authentication is not under test).
type principalStage struct{ p auth.Principal }

func (s principalStage) Wrap(_ api.RouteSpec, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), s.p)))
	})
}

// leakFixture serves the production connections module over a Postgres sandbox with the real envelope
// manager, audit recorder + diff builder and the production JSON logger writing into logs.
type leakFixture struct {
	sb   *testkit.PostgresSandbox
	srv  *httptest.Server
	logs *syncBuffer
	resp bytes.Buffer
}

func newLeakFixture(t *testing.T) *leakFixture {
	sb := testkit.NewManager().Postgres(t)
	sb.Migrate(t)
	f := &leakFixture{sb: sb, logs: newSyncBuffer()}
	log := logging.NewFactory(f.logs).New(config.Config{Service: config.ServiceAPI, Log: config.LogConfig{Level: -4}})
	provider, err := local.Open(filepath.Join(t.TempDir(), "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	db, tx := sb.DB, store.NewTxManager(sb.DB)
	vault := connections.NewSecretVault(secrets.NewEnvelopeManager(provider, secrets.NewPostgresRepository(db), secrets.AESGCMCipher{}))
	repo := pgstore.NewConnectionRepository(db, tx)
	security, props := kafka.DefaultSecurityRegistry(), kafka.DefaultPropertyCatalog()
	validator, resolver := connections.DefaultValidator(security, props), connections.NewSpecResolver(repo, vault)
	failures := connections.ClusterFailures{Classifier: kafka.DefaultFailureClassifier(), Hints: connections.DefaultHintCatalog()}
	runner := connections.NewCheckRunner(checks.DefaultRegistry(), failures.Classifier, failures.Hints, nil)
	recorder, responder := audit.NewPostgresRecorder(db), api.NewResponder(problem.NewCatalog(), log)
	d := connhttp.Deps{
		Service: connections.NewService(connections.ServiceDeps{Repo: repo, Usage: pgstore.NewUsageResolver(db), Vault: vault, Validator: validator, Tx: tx}),
		Tester: connections.NewTester(connections.TesterDeps{Repo: repo, Resolver: resolver, Vault: vault, Validator: validator,
			Clients: kafka.DefaultClientFactoryRegistry(), Runner: runner, Plan: connections.DefaultTestPlan}),
		Resolver: resolver, Responder: responder, Pages: api.NewPageParser(store.NewCursorCodec("leak")), Recorder: recorder,
	}
	auditMW := audit.NewMiddleware(recorder, audit.NewDiffBuilder(logging.NewRedactor()), api.AuditContext, log)
	stages := []api.RouteMiddleware{principalStage{p: f.principal(t)}, api.AuditStage{Audit: auditMW}}
	m := api.NewMounter(responder, []api.GlobalMiddleware{api.RequestIDMiddleware{}, api.LoggingMiddleware{Log: log}}, stages)
	r := chi.NewRouter()
	m.Install(r)
	m.Mount(r, connhttp.NewModule(d))
	f.srv = httptest.NewServer(r)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *leakFixture) principal(t *testing.T) auth.Principal {
	t.Helper()
	ctx := context.Background()
	var user, team uuid.UUID
	q := f.sb.DB.Pool()
	if err := q.QueryRow(ctx, `INSERT INTO users (email, display_name) VALUES ('leak@test', 'Leak') RETURNING id`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := q.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('Platform') RETURNING id`).Scan(&team); err != nil {
		t.Fatal(err)
	}
	return auth.Principal{Kind: auth.PrincipalUser, UserID: user, Email: "leak@test",
		Teams: []auth.TeamRole{{TeamID: team, TeamName: "Platform", Role: auth.RoleEditor}}}
}

func (f *leakFixture) do(t *testing.T, method, path, body string, headers map[string]string) (int, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, method, f.srv.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	f.resp.Write(raw)
	return resp.StatusCode, raw
}

func (f *leakFixture) persisted(t *testing.T) string {
	t.Helper()
	var out strings.Builder
	for _, q := range []string{`SELECT row_to_json(c)::text FROM connections c`, `SELECT row_to_json(a)::text FROM audit_log a`,
		`SELECT encode(ciphertext, 'escape') || encode(wrapped_key, 'escape') FROM secrets`} {
		rows, err := f.sb.DB.Pool().Query(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var s string
			_ = rows.Scan(&s)
			out.WriteString(s + "\n")
		}
		rows.Close()
	}
	return out.String()
}

func connectionBody(password string) string {
	return `{"name":"dev-leak-orders","envTag":"dev","bootstrapServers":["127.0.0.1:1"],
	  "security":{"mode":"sasl_scram_512","username":"svc"},"secrets":{"password":"` + password + `"}}`
}

// TestNoSecretLeaksAcrossConnectionFlows (PC-77): create, update and draft-test a connection carrying
// passwords, then assert no password reaches logs, audit_log diffs, connection rows or API responses.
func TestNoSecretLeaksAcrossConnectionFlows(t *testing.T) {
	f := newLeakFixture(t)
	status, raw := f.do(t, http.MethodPost, string(connhttp.PathCollection), connectionBody(leakCanaries[0]), nil)
	if status != http.StatusCreated {
		t.Fatalf("create: %d %s", status, raw)
	}
	var created struct {
		ID uuid.UUID `json:"id"`
	}
	_ = json.Unmarshal(raw, &created)
	item := string(connhttp.PathCollection) + "/" + created.ID.String()
	patch := `{"description":"rotated","secrets":{"password":"` + leakCanaries[1] + `"}}`
	if status, raw = f.do(t, http.MethodPatch, item, patch, map[string]string{"If-Match": `"1"`}); status != http.StatusOK {
		t.Fatalf("patch: %d %s", status, raw)
	}
	f.do(t, http.MethodPost, string(connhttp.PathTestDraft), connectionBody(leakCanaries[2]), nil)
	f.do(t, http.MethodPost, item+"/test", `{}`, nil)
	f.do(t, http.MethodGet, item, "", nil)
	logs, stored := f.logs.String(), f.persisted(t)
	if !strings.Contains(logs, `"method":"PATCH"`) || !strings.Contains(logs, string(connhttp.PathTestDraft)) ||
		!strings.Contains(stored, string(audit.ActionConnectionSecretSet)) {
		t.Fatalf("capture is empty (logs %d bytes, stored %d bytes); the assertion would be vacuous", len(logs), len(stored))
	}
	for _, canary := range leakCanaries {
		for where, text := range map[string]string{"logs": logs, "database": stored, "responses": f.resp.String()} {
			if strings.Contains(text, canary) {
				t.Errorf("secret %q leaked into %s", canary, where)
			}
		}
	}
}
