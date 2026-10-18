package auth

import (
	"errors"
	"net/http/httptest"
	"testing"
)

func TestScopeGuard(t *testing.T) {
	g := NewScopeGuard(DefaultScopeCatalog())
	cases := []struct {
		name   string
		kind   PrincipalKind
		scopes []TokenScope
		method string
		perm   Permission
		denied bool
	}{
		{"user passes", PrincipalUser, nil, "POST", Permission{Key: "x", Access: AccessAdmin}, false},
		{"read token GET", PrincipalAPIToken, []TokenScope{ScopeRead}, "GET", Permission{}, false},
		{"read token POST no access", PrincipalAPIToken, []TokenScope{ScopeRead}, "POST", Permission{}, true},
		{"read token write perm", PrincipalAPIToken, []TokenScope{ScopeRead}, "GET", Permission{Key: "x", Access: AccessWrite}, true},
		{"write token write perm", PrincipalAPIToken, []TokenScope{ScopeWrite}, "POST", Permission{Key: "x", Access: AccessWrite}, false},
		{"write token admin perm", PrincipalAPIToken, []TokenScope{ScopeWrite}, "POST", Permission{Key: "x", Access: AccessAdmin}, true},
		{"admin token admin perm", PrincipalAPIToken, []TokenScope{ScopeAdmin}, "DELETE", Permission{Key: "x", Access: AccessAdmin}, false},
		{"no scopes", PrincipalAPIToken, nil, "GET", Permission{}, true},
	}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, "/", nil)
		r = r.WithContext(WithPrincipal(r.Context(), Principal{Kind: c.kind, Scopes: c.scopes}))
		err := g.Check(r, c.perm)
		if c.denied != errors.Is(err, ErrScopeDenied) {
			t.Errorf("%s: err = %v", c.name, err)
		}
	}
}
