package opa_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/rbac/opa"
)

var (
	teamA = uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	teamB = uuid.MustParse("00000000-0000-0000-0000-00000000000b")
)

// goldenMatrix loads testdata/rbac/matrix.golden.json, the matrix both engines are compared over.
func goldenMatrix(t *testing.T) rbac.RoleMatrix {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "rbac", "matrix.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc rbac.MatrixDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	m := rbac.RoleMatrix{}
	for role, perms := range doc.Roles {
		m[role] = rbac.NewPermissionSet(perms...)
	}
	return m
}

func subjects() map[string]rbac.Subject {
	out := map[string]rbac.Subject{
		"no binding":        {UserID: uuid.New()},
		"workspace admin":   {UserID: uuid.New(), WorkspaceAdmin: true},
		"disabled admin":    {UserID: uuid.New(), WorkspaceAdmin: true, Disabled: true},
		"editor A viewer B": {UserID: uuid.New(), Bindings: map[rbac.TeamID]rbac.Role{teamA: rbac.RoleEditor, teamB: rbac.RoleViewer}},
		"disabled editor":   {UserID: uuid.New(), Disabled: true, Bindings: map[rbac.TeamID]rbac.Role{teamA: rbac.RoleEditor}},
	}
	for _, role := range []rbac.Role{rbac.RoleViewer, rbac.RoleEditor, rbac.RoleApprover, rbac.RoleAdmin} {
		out[string(role)+" in A"] = rbac.Subject{UserID: uuid.New(), Bindings: map[rbac.TeamID]rbac.Role{teamA: role}}
	}
	return out
}

func engines(t *testing.T) (rbac.PolicyEngine, rbac.PolicyEngine) {
	deps := rbac.EngineDeps{Matrix: goldenMatrix(t), Catalog: rbac.DefaultPermissionCatalog()}
	builtin, err := rbac.NewEngineRegistry().Build(rbac.EngineKind("rolebinding"), deps)
	if err != nil {
		t.Fatal(err)
	}
	reg := rbac.NewEngineRegistry()
	reg.Register(rbac.EngineKind("opa"), opa.Factory(opa.Settings{}))
	policy, err := reg.Build(rbac.EngineKind("opa"), deps)
	if err != nil {
		t.Fatal(err)
	}
	return builtin, policy
}

// TestOPAEngineMatchesRoleBindingEngine: every subject × permission (plus an unknown one) × target team
// yields the same decision, and every subject × team the same permission set.
func TestOPAEngineMatchesRoleBindingEngine(t *testing.T) {
	builtin, policy := engines(t)
	if policy.Kind() != "opa" {
		t.Fatalf("kind = %s", policy.Kind())
	}
	perms := []rbac.Permission{"route.nuke"}
	for p := range rbac.DefaultPermissionCatalog() {
		perms = append(perms, p)
	}
	ctx, checked := context.Background(), 0
	for name, s := range subjects() {
		for _, team := range []rbac.TeamID{teamA, teamB, uuid.Nil} {
			for _, p := range perms {
				r := rbac.Resource{Type: rbac.ResourceRoute, TeamID: team}
				if want, got := builtin.Evaluate(ctx, s, p, r), policy.Evaluate(ctx, s, p, r); want != got {
					t.Errorf("%s / %s / team %s: rolebinding %+v, opa %+v", name, p, team, want, got)
				}
				checked++
			}
			want, got := builtin.Permissions(s, team).Sorted(), policy.Permissions(s, team).Sorted()
			if !slices.Equal(want, got) {
				t.Errorf("%s / team %s permissions: rolebinding %v, opa %v", name, team, want, got)
			}
		}
	}
	if checked < 800 {
		t.Fatalf("only %d decisions compared", checked)
	}
}

// TestOPAEngineFailsClosedOnBadPolicy: a policy without the expected rules never allows.
func TestOPAEngineFailsClosedOnBadPolicy(t *testing.T) {
	e, err := opa.New(context.Background(), rbac.EngineDeps{Matrix: rbac.DefaultRoleMatrix(), Catalog: rbac.DefaultPermissionCatalog()},
		"package router.authz\n\nimport rego.v1\n\ndecision := \"yes\"\n\npermissions := 3\n")
	if err != nil {
		t.Fatal(err)
	}
	admin := rbac.Subject{UserID: uuid.New(), WorkspaceAdmin: true}
	if d := e.Evaluate(context.Background(), admin, rbac.PermRouteRead, rbac.Resource{}); d.Allowed || d.Reason != rbac.ReasonEngineError {
		t.Fatalf("bad policy must deny with engine_error, got %+v", d)
	}
	if len(e.Permissions(admin, teamA)) != 0 {
		t.Fatal("bad policy must grant no permissions")
	}
}

// TestOPAEngineDecisionLatency keeps the in-process evaluation well under the 2 ms authorization budget.
func TestOPAEngineDecisionLatency(t *testing.T) {
	_, policy := engines(t)
	s := subjects()["editor A viewer B"]
	const rounds = 2000
	start := time.Now()
	for range rounds {
		policy.Evaluate(context.Background(), s, rbac.PermRouteWrite, rbac.Resource{TeamID: teamB})
	}
	if avg := time.Since(start) / rounds; avg > time.Millisecond {
		t.Fatalf("average decision took %s", avg)
	}
}
