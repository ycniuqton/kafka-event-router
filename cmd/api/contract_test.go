package main

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"

	tokenhttp "github.com/eventrouter/router/internal/auth/apitoken/httpapi"
	"github.com/eventrouter/router/internal/platform/api/contract"
)

// ContractScope lists route prefixes whose every mounted route must be in the OpenAPI spec (fail), and
// the report for the rest of the API (logged until each feature folder finishes its paths/*.yaml).
var strictContractPrefixes = []string{"/auth/", "/api/v1/auth/"}

func loadContract(t *testing.T) *contract.Spec {
	t.Helper()
	spec, err := contract.LoadDefault(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

// TestMountedRoutesAreDocumented (PC-71) checks every mounted public route against api/openapi/openapi.yaml.
func TestMountedRoutesAreDocumented(t *testing.T) {
	spec := loadContract(t)
	modules := append(featureModules(), tokenhttp.NewModule(tokenhttp.Deps{}))
	var strict, other []string
	for _, m := range modules {
		for _, h := range m.Handlers() {
			r := h.Route()
			if spec.Documents(r.Method, r.Pattern) {
				continue
			}
			key := r.Method + " " + r.Pattern
			if hasAnyPrefix(r.Pattern, strictContractPrefixes) {
				strict = append(strict, key)
			} else {
				other = append(other, key)
			}
		}
	}
	sort.Strings(other)
	if len(other) > 0 {
		t.Logf("%d mounted routes are not in the OpenAPI spec yet (owning feature folders):\n  %s", len(other), strings.Join(other, "\n  "))
	}
	if len(strict) > 0 {
		t.Fatalf("platform routes missing from the spec: %v", strict)
	}
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// contractGuard wraps a test router so every exchange is validated against the spec; violations fail the test.
func contractGuard(t *testing.T, next http.Handler) http.Handler {
	t.Helper()
	rec := contract.NewRecorder(next, contract.NewValidator(loadContract(t)))
	t.Cleanup(func() {
		for _, v := range rec.Violations() {
			t.Errorf("contract violation: %s", v)
		}
	})
	return rec
}
