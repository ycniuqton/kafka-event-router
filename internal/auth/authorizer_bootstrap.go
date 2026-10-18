package auth

import "context"

// TeamRoleAuthorizer is the MVP stand-in until access-control's RBAC lands (ROUTER_AUTHZ=bootstrap):
// any member may read, editors/admins may write, only admins may administer (single team).
type TeamRoleAuthorizer struct {
	required map[Access][]Role
}

// NewTeamRoleAuthorizer returns the bootstrap policy.
func NewTeamRoleAuthorizer() *TeamRoleAuthorizer {
	return &TeamRoleAuthorizer{required: map[Access][]Role{
		AccessRead:  {RoleViewer, RoleEditor, RoleApprover, RoleAdmin},
		AccessWrite: {RoleEditor, RoleAdmin},
		AccessAdmin: {RoleAdmin},
	}}
}

// Authorize allows workloads everything on the internal listener and users by role.
func (a *TeamRoleAuthorizer) Authorize(_ context.Context, p Principal, perm Permission, _ ObjectRef) error {
	if p.Kind == PrincipalWorkload {
		return nil
	}
	if !p.IsAuthenticated() {
		return ErrUnauthenticated
	}
	if p.HasRole(a.required[perm.Access]...) {
		return nil
	}
	return ErrForbidden
}
