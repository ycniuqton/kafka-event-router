package audit_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/internal/testkit"
)

// auditFixture wires the production write side (PostgresRecorder, Scope, TxManager) to a sandbox.
type auditFixture struct {
	sb       *testkit.PostgresSandbox
	txm      *store.PgTxManager
	recorder *audit.PostgresRecorder
	diff     *audit.DiffBuilder
}

func newAuditFixture(t *testing.T) *auditFixture {
	sb := testkit.NewManager().Postgres(t)
	sb.Migrate(t)
	return &auditFixture{sb: sb, txm: store.NewTxManager(sb.DB), recorder: audit.NewPostgresRecorder(sb.DB),
		diff: audit.NewDiffBuilder(logging.NewRedactor())}
}

func (f *auditFixture) count(t *testing.T, requestID string) int {
	t.Helper()
	var n int
	err := f.sb.DB.Pool().QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE request_id = $1`, requestID).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (f *auditFixture) mutateAndRecord(ctx context.Context, scope *audit.Scope, fail error) error {
	return f.txm.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := f.sb.DB.Q(ctx).Exec(ctx, `INSERT INTO teams (name) VALUES ($1)`, scope.Request().RequestID); err != nil {
			return err
		}
		err := scope.Record(ctx, audit.Change{Action: "team.create", Object: audit.ObjectTeam, ObjectID: "t",
			After: map[string]string{"name": "t", "password": "p"}})
		return errors.Join(err, fail)
	})
}

// TestScopeWritesInSameTx: a rolled-back unit of work leaves no audit row; a committed one does.
func TestScopeWritesInSameTx(t *testing.T) {
	f := newAuditFixture(t)
	ctx := context.Background()
	rolled := audit.NewScope(audit.RequestContext{Actor: audit.Actor{Kind: audit.ActorSystem}, RequestID: "req-rollback"}, f.recorder, f.diff)
	if err := f.mutateAndRecord(ctx, rolled, errors.New("boom")); err == nil {
		t.Fatal("expected failure")
	}
	if n := f.count(t, "req-rollback"); n != 0 {
		t.Fatalf("rolled-back tx left %d audit rows", n)
	}
	committed := audit.NewScope(audit.RequestContext{Actor: audit.Actor{Kind: audit.ActorSystem}, RequestID: "req-commit", IP: "10.0.0.9"}, f.recorder, f.diff)
	if err := f.mutateAndRecord(ctx, committed, nil); err != nil {
		t.Fatal(err)
	}
	var diff, ip, kind string
	err := f.sb.DB.Pool().QueryRow(ctx,
		`SELECT diff::text, host(ip), actor_kind FROM audit_log WHERE request_id = 'req-commit'`).Scan(&diff, &ip, &kind)
	if err != nil || ip != "10.0.0.9" || kind != string(audit.ActorSystem) || diff != `{"name": "t", "password": "[REDACTED]"}` {
		t.Fatalf("row: diff=%s ip=%s kind=%s err=%v", diff, ip, kind, err)
	}
}

// TestStrictModePanicsOnUnrecordedMutation: a handler that writes without auditing panics in tests,
// while the non-strict middleware persists the fallback entry instead.
func TestStrictModePanicsOnUnrecordedMutation(t *testing.T) {
	f := newAuditFixture(t)
	src := func(*http.Request) audit.RequestContext {
		return audit.RequestContext{Actor: audit.Actor{Kind: audit.ActorUser}, RequestID: "req-strict"}
	}
	m := audit.NewMiddleware(f.recorder, f.diff, src, slog.New(slog.NewTextHandler(io.Discard, nil)))
	unaudited := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = f.sb.DB.Q(r.Context()).Exec(r.Context(), `INSERT INTO teams (name) VALUES ('unaudited')`)
		w.WriteHeader(http.StatusCreated)
	})
	serve := func() {
		m.Wrap("team.create", unaudited).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/teams", nil))
	}
	m.StrictMode = true
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("StrictMode must panic")
			}
		}()
		serve()
	}()
	m.StrictMode = false
	_, _ = f.sb.DB.Pool().Exec(context.Background(), `DELETE FROM teams WHERE name = 'unaudited'`)
	serve()
	if n := f.count(t, "req-strict"); n != 1 {
		t.Fatalf("fallback rows = %d, want 1", n)
	}
}
