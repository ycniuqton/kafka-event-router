package auth_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/auth/apitoken"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/internal/store/identity"
	"github.com/eventrouter/router/internal/testkit"
)

// tokenFixture is the production token stack (Postgres repository, Manager, Authenticator) on a sandbox schema.
type tokenFixture struct {
	sb      *testkit.PostgresSandbox
	repo    *apitoken.PostgresRepository
	tokens  *apitoken.Manager
	users   *identity.UserRepository
	owner   auth.Principal
	batcher *apitoken.UsageBatcher
}

func newTokenFixture(t *testing.T) *tokenFixture {
	sb := testkit.NewManager().Postgres(t)
	sb.Migrate(t)
	repo, ids := apitoken.NewPostgresRepository(sb.DB), identity.New(sb.DB)
	u, err := ids.Users.Upsert(context.Background(), auth.Identity{Subject: "sub-1", Email: "owner@x.io", DisplayName: "Owner"})
	if err != nil {
		t.Fatal(err)
	}
	return &tokenFixture{sb: sb, repo: repo, users: ids.Users,
		owner:   auth.Principal{Kind: auth.PrincipalUser, UserID: u.ID},
		batcher: apitoken.NewUsageBatcher(repo, apitoken.DefaultFlushInterval, slog.New(slog.NewTextHandler(io.Discard, nil))),
		tokens: apitoken.NewManager(apitoken.ManagerDeps{Repo: repo, Tx: store.NewTxManager(sb.DB), Scopes: auth.DefaultScopeCatalog(),
			Policy: apitoken.DefaultPolicy()})}
}

func (f *tokenFixture) authenticate(t *testing.T, secret string) auth.Outcome {
	t.Helper()
	r := httptest.NewRequest("GET", "/api/v1/routes", nil)
	r.Header.Set("Authorization", "Bearer "+secret)
	_, outcome, err := apitoken.NewAuthenticator(f.repo, f.users, f.batcher, nil).Authenticate(r)
	if err != nil {
		t.Fatal(err)
	}
	return outcome
}

// TestAPITokenLifecycleOnPostgres (PC-72): create, duplicate name, authenticate, batched last use, disable, revoke.
func TestAPITokenLifecycleOnPostgres(t *testing.T) {
	f, ctx := newTokenFixture(t), context.Background()
	req := apitoken.CreateRequest{Name: "ci", Scopes: []auth.TokenScope{auth.ScopeWrite}}
	created, err := f.tokens.Create(ctx, f.owner, req)
	if err != nil {
		t.Fatal(err)
	}
	var classified problem.Pointed
	if _, err := f.tokens.Create(ctx, f.owner, req); !errors.As(err, &classified) || classified.ProblemPath() != "/name" {
		t.Fatalf("duplicate active name: %v", err)
	}
	if got := f.authenticate(t, created.Token); got != auth.OutcomeMatched {
		t.Fatalf("authenticate: %s", got)
	}
	f.batcher.Flush(ctx)
	list, _ := f.tokens.List(ctx, f.owner)
	if len(list) != 1 || list[0].LastUsedAt == nil || time.Since(*list[0].LastUsedAt) > time.Minute {
		t.Fatalf("last_used_at not flushed: %+v", list)
	}
	f.setDisabled(t, true)
	if got := f.authenticate(t, created.Token); got != auth.OutcomeRejected {
		t.Fatalf("disabled owner must be rejected: %s", got)
	}
	f.setDisabled(t, false)
	if err := f.tokens.Revoke(ctx, f.owner, created.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.authenticate(t, created.Token); got != auth.OutcomeRejected {
		t.Fatalf("revoked token must be rejected: %s", got)
	}
	if _, err := f.tokens.Create(ctx, f.owner, req); err != nil {
		t.Fatalf("name is free again after revoke: %v", err)
	}
}

func (f *tokenFixture) setDisabled(t *testing.T, disabled bool) {
	t.Helper()
	_, err := f.sb.DB.Pool().Exec(context.Background(),
		`UPDATE users SET disabled_at = CASE WHEN $2 THEN now() END WHERE id = $1`, f.owner.UserID, disabled)
	if err != nil {
		t.Fatal(err)
	}
}

// TestLastLoginKeepsRowVersion (PC-63): last_login_at writes do not invalidate If-Match on users.
func TestLastLoginKeepsRowVersion(t *testing.T) {
	f, ctx := newTokenFixture(t), context.Background()
	version := func() int64 {
		var v int64
		if err := f.sb.DB.Pool().QueryRow(ctx, `SELECT row_version FROM users WHERE id = $1`, f.owner.UserID).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	before := version()
	if err := f.users.MarkLogin(ctx, f.owner.UserID, time.Now()); err != nil {
		t.Fatal(err)
	}
	if version() != before {
		t.Fatal("last_login_at bumped row_version")
	}
	if _, err := f.sb.DB.Pool().Exec(ctx, `UPDATE users SET display_name = 'New' WHERE id = $1`, f.owner.UserID); err != nil {
		t.Fatal(err)
	}
	if version() != before+1 {
		t.Fatal("real changes must still bump row_version")
	}
	if _, _, err := f.users.LoadUser(ctx, uuid.New()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown user: %v", err)
	}
}
