package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eventrouter/router/internal/platform/api/contract"
)

func loadSpec(t *testing.T) *contract.Spec {
	t.Helper()
	spec, err := contract.LoadDefault(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

func TestSpecResolvesServerPrefixes(t *testing.T) {
	spec := loadSpec(t)
	cases := []struct {
		method, path, template string
		ok                     bool
	}{
		{"GET", "/auth/config", "/auth/config", true},
		{"GET", "/api/v1/auth/me", "/auth/me", true},
		{"DELETE", "/api/v1/auth/tokens/3f1c", "/auth/tokens/{id}", true},
		{"PUT", "/api/v1/auth/tokens/3f1c", "", false},
		{"GET", "/api/v1/nope", "", false},
	}
	for _, c := range cases {
		route, params, ok := spec.Route(c.method, c.path)
		if ok != c.ok || (ok && route.Path != c.template) {
			t.Errorf("%s %s: ok=%v route=%v", c.method, c.path, ok, route)
		}
		if c.template == "/auth/tokens/{id}" && params["id"] != "3f1c" {
			t.Errorf("params = %v", params)
		}
	}
	if !spec.Documents("DELETE", "/api/v1/auth/tokens/{id}") || spec.Documents("PATCH", "/api/v1/auth/tokens/{id}") {
		t.Fatal("Documents mismatch")
	}
}

func TestValidatorFlagsContractBreaks(t *testing.T) {
	v := contract.NewValidator(loadSpec(t))
	req := httptest.NewRequest(http.MethodGet, "/auth/config", nil)
	json := http.Header{"Content-Type": {"application/json"}}
	cases := []struct {
		name   string
		status int
		body   string
		broken bool
	}{
		{"conforming", 200, `{"modes":["dev"],"workspace":"acme"}`, false},
		{"missing required member", 200, `{"modes":["dev"]}`, true},
		{"wrong enum", 200, `{"modes":["kerberos"],"workspace":"acme"}`, true},
		{"undocumented status", 418, `{}`, true},
	}
	for _, c := range cases {
		got := v.Check(context.Background(), req, nil, c.status, json, []byte(c.body))
		if (len(got) > 0) != c.broken {
			t.Errorf("%s: violations %v", c.name, got)
		}
	}
	missing := v.Check(context.Background(), httptest.NewRequest("GET", "/api/v1/unknown", nil), nil, 200, json, []byte(`{}`))
	if len(missing) != 1 || !strings.Contains(missing[0].Reason, "not documented") {
		t.Fatalf("undocumented path: %v", missing)
	}
}
