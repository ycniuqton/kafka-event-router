package oidc_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/auth/oidc"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/internal/store/identity"
	"github.com/eventrouter/router/internal/testkit"
)

// claimFixture wires the production claim mapping (identity teams, rbac BindingService, audit) to a sandbox.
type claimFixture struct {
	sb   *testkit.PostgresSandbox
	tx   *store.PgTxManager
	ids  *identity.Repositories
	hook auth.LoginHook
}

func newClaimFixture(t *testing.T, mappings ...string) *claimFixture {
	sb := testkit.NewManager().Postgres(t)
	sb.Migrate(t)
	f := &claimFixture{sb: sb, tx: store.NewTxManager(sb.DB), ids: identity.New(sb.DB)}
	mapping, err := oidc.ParseRoleMapping(mappings, rbac.Roles{}.Valid)
	if err != nil {
		t.Fatal(err)
	}
	assignable, _ := rbac.ParseRoleSet([]string{"viewer", "editor", "approver", "admin"})
	bindings := rbac.NewBindingService(rbac.BindingServiceDeps{Tx: f.tx, Bindings: rbac.NewPostgresBindingStore(sb.DB),
		Subjects: rbac.NewPostgresSubjectLoader(sb.DB), Guard: rbac.NewAdminGuard(rbac.NewPostgresAdminStore(sb.DB)),
		Assignable: assignable, Trail: rbac.NewAuditTrail(audit.NewPostgresRecorder(sb.DB))})
	granter := oidc.NewBindingGranter(f.ids.Teams, bindings, slog.New(slog.NewTextHandler(io.Discard, nil)))
	f.hook = oidc.NewClaimRoleHook(nil, mapping, granter)
	return f
}

func (f *claimFixture) login(t *testing.T, groups ...string) auth.User {
	t.Helper()
	id := auth.Identity{Mode: config.AuthModeOIDC, Subject: "sub-ada", Email: "ada@example.com", DisplayName: "Ada", Groups: groups}
	var u auth.User
	err := f.tx.WithinTx(context.Background(), func(ctx context.Context) error {
		var err error
		if u, err = f.ids.Users.Upsert(ctx, id); err != nil {
			return err
		}
		return f.hook.OnLogin(ctx, u, id)
	})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func (f *claimFixture) role(t *testing.T, team string) string {
	t.Helper()
	var role string
	err := f.sb.DB.Pool().QueryRow(context.Background(), `SELECT rb.role::text FROM role_bindings rb JOIN teams t ON t.id = rb.team_id
		WHERE t.name = $1`, team).Scan(&role)
	if err != nil {
		return ""
	}
	return role
}

// TestClaimMappingAppliesBindingsOnLogin: groups create/raise bindings with audit, never lower them, never create teams.
func TestClaimMappingAppliesBindingsOnLogin(t *testing.T) {
	f := newClaimFixture(t, "ops=Platform:editor", "leads=Platform:admin", "ghosts=Nowhere:admin")
	ctx := context.Background()
	for _, team := range []string{"Platform"} {
		if _, err := f.ids.Teams.EnsureByName(ctx, team); err != nil {
			t.Fatal(err)
		}
	}
	f.login(t, "ops", "ghosts")
	if got := f.role(t, "Platform"); got != "editor" {
		t.Fatalf("first login: %q", got)
	}
	if _, found, _ := f.ids.Teams.FindByName(ctx, "Nowhere"); found {
		t.Fatal("mapping created a team")
	}
	f.login(t, "leads")
	if got := f.role(t, "Platform"); got != "admin" {
		t.Fatalf("upgrade: %q", got)
	}
	f.login(t, "ops")
	if got := f.role(t, "Platform"); got != "admin" {
		t.Fatalf("downgraded by a weaker group: %q", got)
	}
	var n int
	_ = f.sb.DB.Pool().QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = $1 AND actor_kind = 'system'`,
		string(audit.ActionRoleBindingSet)).Scan(&n)
	if n != 2 {
		t.Fatalf("role_binding.set audit rows: %d", n)
	}
}
