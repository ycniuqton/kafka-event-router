package rbac_test

import (
	"context"
	"errors"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/rbac/rbactest"
	"testing"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/store"
)

func bindingService(w *rbactest.World, roles ...rbac.Role) *rbac.BindingService {
	set := rbac.RoleSet{}
	for _, r := range roles {
		set[r] = true
	}
	return rbac.NewBindingService(rbac.BindingServiceDeps{Tx: rbactest.PassTx{}, Bindings: w, Subjects: w, Guard: rbac.NewAdminGuard(w),
		Assignable: set, Trail: rbac.NewAuditTrail(w)})
}

func version(v int64) *store.RowVersion {
	rv := store.RowVersion(v)
	return &rv
}

func TestBindingServiceRules(t *testing.T) {
	w := rbactest.NewWorld()
	admin := w.AddUser(false, rbac.UserActive)
	editor := w.AddUser(false, rbac.UserActive)
	w.Bind(admin, teamA, rbac.RoleAdmin)
	svc := bindingService(w, rbac.RoleAdmin, rbac.RoleEditor)
	ctx := context.Background()

	if _, err := svc.Set(ctx, rbac.SetBinding{UserID: editor, TeamID: teamA, Role: rbac.RoleViewer}); !errors.Is(err, rbac.ErrRoleNotAssignable) {
		t.Fatalf("viewer not assignable in MVP: %v", err)
	}
	b, err := svc.Set(ctx, rbac.SetBinding{UserID: editor, TeamID: teamA, Role: rbac.RoleEditor})
	if err != nil || b.Role != rbac.RoleEditor {
		t.Fatalf("create: %v %+v", err, b)
	}
	if _, err := svc.Set(ctx, rbac.SetBinding{UserID: admin, TeamID: teamA, Role: rbac.RoleEditor}); !errors.Is(err, rbac.ErrBindingPrecondition) {
		t.Fatalf("change without If-Match: %v", err)
	}
	if _, err := svc.Set(ctx, rbac.SetBinding{UserID: admin, TeamID: teamA, Role: rbac.RoleEditor, IfMatch: version(9)}); !errors.Is(err, store.ErrPreconditionFailed) {
		t.Fatalf("stale If-Match: %v", err)
	}
	if _, err := svc.Set(ctx, rbac.SetBinding{UserID: admin, TeamID: teamA, Role: rbac.RoleEditor, IfMatch: version(1)}); !errors.Is(err, rbac.ErrLastAdmin) {
		t.Fatalf("demote last admin: %v", err)
	}
	if err := svc.Remove(ctx, admin, teamA, version(1)); !errors.Is(err, rbac.ErrLastAdmin) {
		t.Fatalf("remove last admin: %v", err)
	}
	if _, err := svc.Set(ctx, rbac.SetBinding{UserID: editor, TeamID: teamA, Role: rbac.RoleAdmin, IfMatch: version(1)}); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if _, err := svc.Set(ctx, rbac.SetBinding{UserID: admin, TeamID: teamA, Role: rbac.RoleEditor, IfMatch: version(1)}); err != nil {
		t.Fatalf("demote with another admin: %v", err)
	}
	if len(w.Entries) != 3 || w.Entries[0].Action != audit.ActionRoleBindingSet {
		t.Fatalf("audit entries = %+v", w.Entries)
	}
}

func TestBindingServiceWorkspaceAdminKeepsAdmin(t *testing.T) {
	w := rbactest.NewWorld()
	ws := w.AddUser(true, rbac.UserActive)
	w.Bind(ws, teamA, rbac.RoleAdmin)
	if err := bindingService(w, rbac.RoleAdmin, rbac.RoleEditor).Remove(context.Background(), ws, teamA, version(1)); err != nil {
		t.Fatalf("workspace admin stays admin through the flag: %v", err)
	}
}

func userService(w *rbactest.World) *rbac.UserService {
	return rbac.NewUserService(rbac.UserServiceDeps{Tx: rbactest.PassTx{}, Users: rbactest.UserStore{W: w}, Guard: rbac.NewAdminGuard(w), Sessions: w, Trail: rbac.NewAuditTrail(w)})
}

func TestUserServiceUpdate(t *testing.T) {
	w := rbactest.NewWorld()
	only := w.AddUser(true, rbac.UserActive)
	other := w.AddUser(false, rbac.UserActive)
	svc, ctx := userService(w), context.Background()
	yes, no, blank := true, false, "  "

	if _, err := svc.Update(ctx, only, rbac.UserChange{Disabled: &yes}, 1); !errors.Is(err, rbac.ErrLastAdmin) {
		t.Fatalf("disable last admin: %v", err)
	}
	if _, err := svc.Update(ctx, only, rbac.UserChange{WorkspaceAdmin: &no}, 1); !errors.Is(err, rbac.ErrLastAdmin) {
		t.Fatalf("demote last admin: %v", err)
	}
	if _, err := svc.Update(ctx, other, rbac.UserChange{DisplayName: &blank}, 1); !errors.Is(err, rbac.ErrInvalidDisplayName) {
		t.Fatalf("blank name: %v", err)
	}
	if _, err := svc.Update(ctx, other, rbac.UserChange{WorkspaceAdmin: &yes}, 7); !errors.Is(err, store.ErrPreconditionFailed) {
		t.Fatalf("stale: %v", err)
	}
	u, err := svc.Update(ctx, other, rbac.UserChange{WorkspaceAdmin: &yes}, 1)
	if err != nil || !u.WorkspaceAdmin || u.RowVersion != 2 {
		t.Fatalf("promote: %v %+v", err, u)
	}
	u, err = svc.Update(ctx, only, rbac.UserChange{Disabled: &yes}, 1)
	if err != nil || u.Status != rbac.UserDisabled || len(w.Revoked) != 1 || w.Revoked[0] != only {
		t.Fatalf("disable: %v %+v revoked=%v", err, u, w.Revoked)
	}
	if len(w.Entries) != 2 || w.Entries[1].Action != audit.ActionUserUpdated {
		t.Fatalf("audit = %+v", w.Entries)
	}
}

func bootstrapper(w *rbactest.World, emails ...string) *rbac.Bootstrapper {
	return rbac.NewBootstrapper(rbac.BootstrapDeps{
		Rules:  []rbac.BootstrapRule{rbac.DevSeedRule{}, rbac.NewEmailAllowListRule(emails), rbac.NewFirstAdminRule(w)},
		Admins: w, Teams: rbactest.Teams{ID: teamA}, Bindings: rbactest.Ensurer{W: w}, TeamName: "default", Recorder: w,
	})
}

func TestBootstrapperBranches(t *testing.T) {
	w := rbactest.NewWorld()
	b := bootstrapper(w, "Boss@Corp.io")
	ctx := context.Background()
	oidc := auth.Identity{Mode: config.AuthModeOIDC}

	first := w.AddUser(false, rbac.UserActive)
	if err := b.OnLogin(ctx, auth.User{ID: first, Email: "a@corp.io"}, oidc); err != nil || !w.Users[first].WorkspaceAdmin {
		t.Fatalf("first user must become admin: %v", err)
	}
	if w.Bindings[rbactest.BindingKey{first, teamA}].Role != rbac.RoleAdmin || len(w.Entries) != 1 {
		t.Fatalf("first admin binding/audit missing: %+v", w.Entries)
	}
	second := w.AddUser(false, rbac.UserActive)
	_ = b.OnLogin(ctx, auth.User{ID: second, Email: "b@corp.io"}, oidc)
	if w.Users[second].WorkspaceAdmin || len(w.Bindings) != 1 {
		t.Fatal("later users get no access until an admin assigns a role")
	}
	boss := w.AddUser(false, rbac.UserActive)
	_ = b.OnLogin(ctx, auth.User{ID: boss, Email: "boss@corp.io"}, oidc)
	if !w.Users[boss].WorkspaceAdmin {
		t.Fatal("bootstrap email must become admin")
	}
	dev := w.AddUser(false, rbac.UserActive)
	_ = b.OnLogin(ctx, auth.User{ID: dev, Email: "d@local.dev"}, auth.Identity{Mode: config.AuthModeDev})
	if !w.Users[dev].WorkspaceAdmin {
		t.Fatal("dev login must become admin")
	}
	_ = b.OnLogin(ctx, auth.User{ID: dev, Email: "d@local.dev"}, auth.Identity{Mode: config.AuthModeDev})
	if len(w.Entries) != 3 {
		t.Fatalf("repeat login must not re-audit: %d", len(w.Entries))
	}
}
