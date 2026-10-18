package rbac

import (
	"context"

	"github.com/eventrouter/router/internal/auth"
)

// AuthorizerAdapter implements platform-core's auth.Authorizer port over the Manager.
type AuthorizerAdapter struct {
	manager Manager
	mapper  ResourceMapper
}

// NewAuthorizerAdapter builds the adapter.
func NewAuthorizerAdapter(m Manager) *AuthorizerAdapter {
	return &AuthorizerAdapter{manager: m}
}

// Authorize implements auth.Authorizer.
func (a *AuthorizerAdapter) Authorize(ctx context.Context, p auth.Principal, perm auth.Permission, obj auth.ObjectRef) error {
	return a.manager.Authorize(auth.WithPrincipal(ctx, p), Permission(perm.Key), a.mapper.FromObject(obj))
}

// MeKey is the member access control contributes to GET /auth/me.
type MeKey string

const MeKeyAccess MeKey = "access"

// AccessContributor adds `access` to /auth/me (implements auth handlers' MeContributor).
type AccessContributor struct {
	manager Manager
}

// NewAccessContributor builds the contributor.
func NewAccessContributor(m Manager) *AccessContributor {
	return &AccessContributor{manager: m}
}

// Key implements MeContributor.
func (c *AccessContributor) Key() string { return string(MeKeyAccess) }

// Contribute returns the caller's effective access.
func (c *AccessContributor) Contribute(ctx context.Context, p auth.Principal) (any, error) {
	return c.manager.Effective(ctx, p.UserID)
}
