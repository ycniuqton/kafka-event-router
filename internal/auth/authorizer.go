package auth

import (
	"fmt"
	"net/http"

	"github.com/eventrouter/router/internal/platform/config"
)

// AuthzMode selects the Authorizer implementation (ROUTER_AUTHZ).
type AuthzMode = config.AuthzMode

// AuthorizerFactory builds an Authorizer implementation.
type AuthorizerFactory func() (Authorizer, error)

// AuthorizerRegistry selects the configured Authorizer; access-control registers "rbac".
type AuthorizerRegistry struct {
	factories map[AuthzMode]AuthorizerFactory
}

// NewAuthorizerRegistry returns a registry pre-loaded with the bootstrap authorizer.
func NewAuthorizerRegistry() *AuthorizerRegistry {
	reg := &AuthorizerRegistry{factories: map[AuthzMode]AuthorizerFactory{}}
	reg.Register(config.AuthzBootstrap, func() (Authorizer, error) { return NewTeamRoleAuthorizer(), nil })
	return reg
}

// Register adds a factory for a mode.
func (r *AuthorizerRegistry) Register(mode AuthzMode, f AuthorizerFactory) {
	r.factories[mode] = f
}

// Build instantiates the configured Authorizer.
func (r *AuthorizerRegistry) Build(mode AuthzMode) (Authorizer, error) {
	f, ok := r.factories[mode]
	if !ok {
		return nil, fmt.Errorf("auth: no authorizer registered for %q", mode)
	}
	return f()
}

// ObjectResolver extracts the object a request targets (route params) for authorization.
type ObjectResolver func(r *http.Request) ObjectRef

// AuthorizeMiddleware enforces one permission per route through the Authorizer port.
type AuthorizeMiddleware struct {
	authz Authorizer
}

// NewAuthorizeMiddleware wraps an Authorizer.
func NewAuthorizeMiddleware(authz Authorizer) *AuthorizeMiddleware {
	return &AuthorizeMiddleware{authz: authz}
}

// Check authorizes the request principal; the returned error is a classified problem.
func (m *AuthorizeMiddleware) Check(r *http.Request, perm Permission, resolve ObjectResolver) error {
	if perm.IsZero() {
		return nil
	}
	obj := ObjectRef{}
	if resolve != nil {
		obj = resolve(r)
	}
	return m.authz.Authorize(r.Context(), PrincipalFrom(r.Context()), perm, obj)
}
