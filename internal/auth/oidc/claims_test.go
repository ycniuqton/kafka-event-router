package oidc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/store"
)

func TestClaimPathValues(t *testing.T) {
	claims := map[string]any{
		"groups":       []any{"ops", "", 3, "dev"},
		"scope":        "a b",
		"realm_access": map[string]any{"roles": []any{"admin"}},
		"flag":         true,
	}
	cases := map[ClaimPath][]string{
		"groups": {"ops", "dev"}, "scope": {"a", "b"}, "realm_access.roles": {"admin"},
		"flag": nil, "missing": nil, "groups.deeper": nil, "": nil,
	}
	for path, want := range cases {
		if got := path.Values(claims); !slices.Equal(got, want) {
			t.Errorf("%q: got %v want %v", path, got, want)
		}
	}
}

func TestParseRoleMapping(t *testing.T) {
	m, err := ParseRoleMapping([]string{"ops=Platform:admin", "ops=Payments:Viewer", " dev = Payments : editor "}, rbac.Roles{}.Valid)
	if err != nil {
		t.Fatal(err)
	}
	got := m.Grants([]string{"dev", "ops", "nobody"})
	want := []GroupGrant{{"Payments", auth.RoleEditor}, {"Platform", auth.RoleAdmin}, {"Payments", auth.RoleViewer}}
	if !slices.Equal(got, want) || m.Empty() {
		t.Fatalf("grants: %v", got)
	}
	for _, bad := range []string{"ops", "ops=Platform", "=Platform:admin", "ops=:admin", "ops=Platform:owner"} {
		if _, err := ParseRoleMapping([]string{bad}, rbac.Roles{}.Valid); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestOIDCFlowExtractsConfiguredGroupClaim(t *testing.T) {
	idp := newFakeIdP(t)
	f := newFlow(t, idp)
	f.cfg.RoleClaims = config.OIDCRoleClaimConfig{Claim: "realm_access.roles"}
	s := begin(t, f, idp)
	idp.claims = idp.baseClaims(s.nonce, time.Now().Add(time.Hour))
	idp.claims["realm_access"] = map[string]any{"roles": []string{"ops", "dev"}}
	id, err := complete(f, s, s.state, "good-code")
	if err != nil || !slices.Equal(id.Groups, []string{"ops", "dev"}) {
		t.Fatalf("groups: %v %v", id.Groups, err)
	}
}

type recordingHook struct{ calls int }

func (h *recordingHook) OnLogin(context.Context, auth.User, auth.Identity) error {
	h.calls++
	return nil
}

type recordingGranter struct{ grants [][]GroupGrant }

func (g *recordingGranter) Grant(_ context.Context, _ auth.User, grants []GroupGrant) error {
	g.grants = append(g.grants, grants)
	return nil
}

func TestClaimRoleHookRunsNextThenGrantsOIDCOnly(t *testing.T) {
	mapping, _ := ParseRoleMapping([]string{"ops=Platform:admin"}, rbac.Roles{}.Valid)
	next, granter := &recordingHook{}, &recordingGranter{}
	hook := NewClaimRoleHook(next, mapping, granter)
	ctx, u := context.Background(), auth.User{ID: uuid.New()}
	_ = hook.OnLogin(ctx, u, auth.Identity{Mode: config.AuthModeDev, Groups: []string{"ops"}})
	_ = hook.OnLogin(ctx, u, auth.Identity{Mode: config.AuthModeOIDC, Groups: []string{"ops"}})
	if next.calls != 2 || len(granter.grants) != 1 || granter.grants[0][0] != (GroupGrant{"Platform", auth.RoleAdmin}) {
		t.Fatalf("next %d grants %v", next.calls, granter.grants)
	}
}

type memTeams map[string]uuid.UUID

func (m memTeams) FindByName(_ context.Context, name string) (uuid.UUID, bool, error) {
	id, ok := m[name]
	return id, ok, nil
}

type memBindings struct {
	rows       map[uuid.UUID]rbac.RoleBinding
	sets       []rbac.SetBinding
	assignable rbac.RoleSet
}

func (m *memBindings) List(_ context.Context, _ rbac.BindingFilter) ([]rbac.RoleBinding, error) {
	var out []rbac.RoleBinding
	for _, b := range m.rows {
		out = append(out, b)
	}
	return out, nil
}

func (m *memBindings) Set(_ context.Context, cmd rbac.SetBinding) (rbac.RoleBinding, error) {
	if !m.assignable.Has(cmd.Role) {
		return rbac.RoleBinding{}, rbac.ErrRoleNotAssignable
	}
	if b, ok := m.rows[cmd.TeamID]; ok && (cmd.IfMatch == nil || *cmd.IfMatch != b.RowVersion) {
		return rbac.RoleBinding{}, errors.New("precondition")
	}
	m.sets = append(m.sets, cmd)
	b := rbac.RoleBinding{UserID: cmd.UserID, TeamID: cmd.TeamID, Role: cmd.Role, RowVersion: store.RowVersion(len(m.sets))}
	m.rows[cmd.TeamID] = b
	return b, nil
}

func TestBindingGranterIsAdditiveAndUpgradeOnly(t *testing.T) {
	platform, payments, data, ml := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	teams := memTeams{"Platform": platform, "Payments": payments, "Data": data, "ML": ml}
	u := auth.User{ID: uuid.New()}
	bindings := &memBindings{assignable: rbac.RoleSet{auth.RoleViewer: true, auth.RoleEditor: true, auth.RoleAdmin: true}, rows: map[uuid.UUID]rbac.RoleBinding{
		payments: {UserID: u.ID, TeamID: payments, Role: auth.RoleViewer, RowVersion: 4},
		data:     {UserID: u.ID, TeamID: data, Role: auth.RoleAdmin, RowVersion: 2},
	}}
	g := NewBindingGranter(teams, bindings, slog.New(slog.NewTextHandler(io.Discard, nil)))
	err := g.Grant(context.Background(), u, []GroupGrant{
		{"Platform", auth.RoleViewer}, {"Platform", auth.RoleEditor}, {"Payments", auth.RoleEditor},
		{"Data", auth.RoleViewer}, {"Unknown", auth.RoleAdmin}, {"ML", auth.RoleApprover},
	})
	if err != nil {
		t.Fatal(err)
	}
	if bindings.rows[platform].Role != auth.RoleEditor || bindings.rows[payments].Role != auth.RoleEditor || bindings.rows[data].Role != auth.RoleAdmin {
		t.Fatalf("bindings: %+v", bindings.rows)
	}
	if _, ok := bindings.rows[ml]; ok || len(bindings.sets) != 2 {
		t.Fatalf("unexpected writes: %+v", bindings.sets)
	}
	if up := bindings.sets[0]; up.TeamID != payments || up.IfMatch == nil || *up.IfMatch != 4 {
		t.Fatalf("upgrade must carry the current version: %+v", up)
	}
}
