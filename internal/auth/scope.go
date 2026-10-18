package auth

import (
	"net/http"

	"github.com/eventrouter/router/internal/platform/problem"
)

// TokenScope limits what an API token may do on top of its owner's roles (never more than the owner).
type TokenScope string

const (
	ScopeRead  TokenScope = "read"
	ScopeWrite TokenScope = "write"
	ScopeAdmin TokenScope = "admin"
)

// ErrScopeDenied rejects an API-token request whose scopes do not cover the route's access level.
var ErrScopeDenied = problem.NewError(problem.KindForbidden,
	"This API token's scopes don't allow this action. Create a token with a wider scope.")

// ScopeCatalog lists every token scope and the access levels it grants.
type ScopeCatalog struct {
	grants map[TokenScope]map[Access]bool
	order  []TokenScope
}

// DefaultScopeCatalog returns read ⊂ write ⊂ admin.
func DefaultScopeCatalog() *ScopeCatalog {
	return &ScopeCatalog{
		grants: map[TokenScope]map[Access]bool{
			ScopeRead:  {AccessRead: true},
			ScopeWrite: {AccessRead: true, AccessWrite: true},
			ScopeAdmin: {AccessRead: true, AccessWrite: true, AccessAdmin: true},
		},
		order: []TokenScope{ScopeRead, ScopeWrite, ScopeAdmin},
	}
}

// Known reports whether s is a catalogued scope.
func (c *ScopeCatalog) Known(s TokenScope) bool {
	_, ok := c.grants[s]
	return ok
}

// Scopes lists every scope from narrowest to widest.
func (c *ScopeCatalog) Scopes() []TokenScope {
	return append([]TokenScope(nil), c.order...)
}

// Allows reports whether any of scopes grants access.
func (c *ScopeCatalog) Allows(scopes []TokenScope, access Access) bool {
	for _, s := range scopes {
		if c.grants[s][access] {
			return true
		}
	}
	return false
}

// ScopeGuard enforces token scopes per route; other principal kinds pass untouched.
type ScopeGuard struct {
	catalog *ScopeCatalog
	safe    map[string]bool
}

// NewScopeGuard builds the guard over a scope catalog.
func NewScopeGuard(catalog *ScopeCatalog) *ScopeGuard {
	return &ScopeGuard{catalog: catalog, safe: map[string]bool{
		http.MethodGet: true, http.MethodHead: true, http.MethodOptions: true,
	}}
}

// Check rejects token requests whose scopes miss the access the route needs.
func (g *ScopeGuard) Check(r *http.Request, perm Permission) error {
	p := PrincipalFrom(r.Context())
	if p.Kind != PrincipalAPIToken {
		return nil
	}
	if g.catalog.Allows(p.Scopes, g.required(r.Method, perm)) {
		return nil
	}
	return ErrScopeDenied
}

// required is the permission's access level, or read/write by HTTP method when the route names none.
func (g *ScopeGuard) required(method string, perm Permission) Access {
	if perm.Access != "" {
		return perm.Access
	}
	if g.safe[method] {
		return AccessRead
	}
	return AccessWrite
}
