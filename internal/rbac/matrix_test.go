package rbac_test

import (
	"encoding/json"
	"flag"
	"github.com/eventrouter/router/internal/rbac"
	"os"
	"path/filepath"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata/rbac/matrix.golden.json")

func goldenPath() string {
	return filepath.Join("..", "..", "testdata", "rbac", "matrix.golden.json")
}

func TestRoleMatrixGolden(t *testing.T) {
	got, err := json.MarshalIndent(rbac.DefaultRoleMatrix().Document(rbac.DefaultPermissionCatalog()), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	if *updateGolden {
		if err := os.WriteFile(goldenPath(), got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(goldenPath())
	if err != nil {
		t.Fatalf("read golden (run with -update): %v", err)
	}
	if string(want) != string(got) {
		t.Fatalf("role matrix changed; review and run `go test ./internal/rbac -run Golden -update`\n%s", got)
	}
}

func TestMatrixIsCumulativeAndCatalogued(t *testing.T) {
	m, cat := rbac.DefaultRoleMatrix(), rbac.DefaultPermissionCatalog()
	roles := rbac.Roles{}.Ordered()
	for i := 1; i < len(roles); i++ {
		for p := range m.Permissions(roles[i-1]) {
			if !m.Permissions(roles[i]).Has(p) {
				t.Errorf("%s lacks %s held by %s", roles[i], p, roles[i-1])
			}
		}
	}
	for _, r := range roles {
		for p := range m.Permissions(r) {
			if meta, ok := cat[p]; !ok || meta.Scope != rbac.ScopeTeam {
				t.Errorf("%s grants %s which is not a catalogued team permission", r, p)
			}
		}
	}
	if !m.Permissions(rbac.RoleApprover).Has(rbac.PermDeploymentApprove) || m.Permissions(rbac.RoleEditor).Has(rbac.PermDeploymentApprove) {
		t.Fatal("Approver = Editor ∪ approve violated")
	}
}
