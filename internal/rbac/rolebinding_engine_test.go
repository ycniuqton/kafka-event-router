package rbac_test

import (
	"context"
	"github.com/eventrouter/router/internal/rbac"
	"testing"

	"github.com/google/uuid"
)

var (
	teamA = uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	teamB = uuid.MustParse("00000000-0000-0000-0000-00000000000b")
)

func subjectWith(role rbac.Role) rbac.Subject {
	return rbac.Subject{UserID: uuid.New(), Kind: rbac.SubjectUser, Bindings: map[rbac.TeamID]rbac.Role{teamA: role}}
}

func TestRoleBindingEngineMatrix(t *testing.T) {
	e := rbac.NewRoleBindingEngine(rbac.DefaultRoleMatrix(), rbac.DefaultPermissionCatalog())
	m := rbac.DefaultRoleMatrix()
	for _, role := range (rbac.Roles{}).Ordered() {
		for p, meta := range rbac.DefaultPermissionCatalog() {
			d := e.Evaluate(context.Background(), subjectWith(role), p, rbac.Resource{TeamID: teamA})
			want := meta.Scope == rbac.ScopeTeam && m.Permissions(role).Has(p)
			if d.Allowed != want {
				t.Errorf("%s/%s on own team: allowed=%v want %v (%s)", role, p, d.Allowed, want, d.Reason)
			}
		}
	}
}

func TestRoleBindingEngineCases(t *testing.T) {
	e := rbac.NewRoleBindingEngine(rbac.DefaultRoleMatrix(), rbac.DefaultPermissionCatalog())
	admin := rbac.Subject{UserID: uuid.New(), WorkspaceAdmin: true}
	disabledAdmin := rbac.Subject{UserID: uuid.New(), WorkspaceAdmin: true, Disabled: true}
	cases := []struct {
		name   string
		s      rbac.Subject
		p      rbac.Permission
		r      rbac.Resource
		allow  bool
		reason rbac.DenyReason
	}{
		{"unknown permission fails closed", admin, rbac.Permission("route.nuke"), rbac.Resource{}, false, rbac.ReasonUnknownPermission},
		{"disabled admin denied", disabledAdmin, rbac.PermRouteRead, rbac.Resource{TeamID: teamA}, false, rbac.ReasonDisabledUser},
		{"workspace admin any team", admin, rbac.PermConnectionWrite, rbac.Resource{TeamID: teamB}, true, rbac.ReasonNone},
		{"workspace admin workspace perm", admin, rbac.PermAuditRead, rbac.Resource{}, true, rbac.ReasonNone},
		{"team admin lacks workspace perm", subjectWith(rbac.RoleAdmin), rbac.PermAuditRead, rbac.Resource{}, false, rbac.ReasonMissingPermission},
		{"no binding", rbac.Subject{UserID: uuid.New()}, rbac.PermRouteRead, rbac.Resource{TeamID: teamA}, false, rbac.ReasonNoBinding},
		{"cross-team route read", subjectWith(rbac.RoleViewer), rbac.PermRouteRead, rbac.Resource{TeamID: teamB}, true, rbac.ReasonNone},
		{"cross-team connection read", subjectWith(rbac.RoleViewer), rbac.PermConnectionRead, rbac.Resource{TeamID: teamB}, true, rbac.ReasonNone},
		{"cross-team write denied", subjectWith(rbac.RoleAdmin), rbac.PermRouteWrite, rbac.Resource{TeamID: teamB}, false, rbac.ReasonMissingPermission},
		{"cross-team samples denied", subjectWith(rbac.RoleEditor), rbac.PermSampleRead, rbac.Resource{TeamID: teamB}, false, rbac.ReasonMissingPermission},
		{"viewer cannot write", subjectWith(rbac.RoleViewer), rbac.PermRouteWrite, rbac.Resource{TeamID: teamA}, false, rbac.ReasonMissingPermission},
		{"collection write uses any binding", subjectWith(rbac.RoleEditor), rbac.PermRouteWrite, rbac.Resource{}, true, rbac.ReasonNone},
		{"collection admin perm denied for editor", subjectWith(rbac.RoleEditor), rbac.PermConnectionWrite, rbac.Resource{}, false, rbac.ReasonMissingPermission},
		{"editor raw samples", subjectWith(rbac.RoleEditor), rbac.PermSampleReadRaw, rbac.Resource{TeamID: teamA}, true, rbac.ReasonNone},
		{"viewer raw samples denied", subjectWith(rbac.RoleViewer), rbac.PermSampleReadRaw, rbac.Resource{TeamID: teamA}, false, rbac.ReasonMissingPermission},
	}
	for _, c := range cases {
		d := e.Evaluate(context.Background(), c.s, c.p, c.r)
		if d.Allowed != c.allow || d.Reason != c.reason {
			t.Errorf("%s: got %+v", c.name, d)
		}
		if !d.Allowed && d.Missing != c.p {
			t.Errorf("%s: missing = %q", c.name, d.Missing)
		}
	}
}

func TestEnginePermissions(t *testing.T) {
	e := rbac.NewRoleBindingEngine(rbac.DefaultRoleMatrix(), rbac.DefaultPermissionCatalog())
	if got := e.Permissions(subjectWith(rbac.RoleEditor), teamB); len(got) != 0 {
		t.Fatalf("other team perms = %v", got.Sorted())
	}
	if got := e.Permissions(rbac.Subject{WorkspaceAdmin: true}, teamB); !got.Has(rbac.PermUserManage) || !got.Has(rbac.PermConnectionWrite) {
		t.Fatal("workspace admin must hold everything")
	}
	if got := e.Permissions(rbac.Subject{WorkspaceAdmin: true, Disabled: true}, teamA); len(got) != 0 {
		t.Fatal("disabled subject holds permissions")
	}
}

func TestEngineRegistry(t *testing.T) {
	reg := rbac.NewEngineRegistry()
	if _, err := reg.Build("opa", rbac.EngineDeps{}); err == nil {
		t.Fatal("unregistered engine must fail")
	}
	e, err := reg.Build("rolebinding", rbac.EngineDeps{Matrix: rbac.DefaultRoleMatrix(), Catalog: rbac.DefaultPermissionCatalog()})
	if err != nil || e.Kind() != "rolebinding" {
		t.Fatalf("build: %v", err)
	}
}
