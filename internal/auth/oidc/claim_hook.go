package oidc

import (
	"context"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/config"
)

// RoleGranter applies the team roles an OIDC login's groups grant to the user's role bindings.
type RoleGranter interface {
	Grant(ctx context.Context, u auth.User, grants []GroupGrant) error
}

// ClaimRoleHook decorates the configured LoginHook (access-control's Bootstrapper): after it ran, OIDC
// group claims mapped by ROUTER_OIDC_ROLE_MAPPINGS are applied inside the same login transaction.
type ClaimRoleHook struct {
	next    auth.LoginHook
	mapping *RoleMapping
	granter RoleGranter
}

// NewClaimRoleHook wraps next.
func NewClaimRoleHook(next auth.LoginHook, mapping *RoleMapping, granter RoleGranter) *ClaimRoleHook {
	return &ClaimRoleHook{next: next, mapping: mapping, granter: granter}
}

// OnLogin implements auth.LoginHook.
func (h *ClaimRoleHook) OnLogin(ctx context.Context, u auth.User, id auth.Identity) error {
	if h.next != nil {
		if err := h.next.OnLogin(ctx, u, id); err != nil {
			return err
		}
	}
	if id.Mode != config.AuthModeOIDC || h.mapping.Empty() {
		return nil
	}
	return h.granter.Grant(ctx, u, h.mapping.Grants(id.Groups))
}
