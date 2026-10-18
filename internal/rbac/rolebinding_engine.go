package rbac

import (
	"context"

	"github.com/eventrouter/router/internal/platform/config"
)

// RoleBindingEngine evaluates permissions from the subject's team role bindings (design flow 5.1).
type RoleBindingEngine struct {
	matrix  RoleMatrix
	catalog PermissionCatalog
}

// NewRoleBindingEngine builds the engine over a matrix and catalog.
func NewRoleBindingEngine(matrix RoleMatrix, catalog PermissionCatalog) *RoleBindingEngine {
	return &RoleBindingEngine{matrix: matrix, catalog: catalog}
}

// Kind implements PolicyEngine.
func (e *RoleBindingEngine) Kind() EngineKind { return config.RBACEngineRoleBinding }

// Evaluate fails closed: unknown permission, disabled subject or no grant all deny.
func (e *RoleBindingEngine) Evaluate(_ context.Context, s Subject, p Permission, r Resource) Decision {
	meta, known := e.catalog[p]
	switch {
	case !known:
		return Deny(ReasonUnknownPermission, p)
	case s.Disabled:
		return Deny(ReasonDisabledUser, p)
	case s.WorkspaceAdmin:
		return Allow()
	case meta.Scope == ScopeWorkspace:
		return Deny(ReasonMissingPermission, p)
	case !s.HasAnyBinding():
		return Deny(ReasonNoBinding, p)
	case e.grantedOnTarget(s, p, r):
		return Allow()
	case meta.CrossTeam:
		return Allow()
	}
	return Deny(ReasonMissingPermission, p)
}

// Permissions returns what s may do in team (workspace admins: everything in every team).
func (e *RoleBindingEngine) Permissions(s Subject, team TeamID) PermissionSet {
	if s.Disabled {
		return NewPermissionSet()
	}
	if s.WorkspaceAdmin {
		return e.catalog.InScope(ScopeTeam).Union(e.catalog.InScope(ScopeWorkspace))
	}
	role, ok := s.RoleIn(team)
	if !ok {
		return NewPermissionSet()
	}
	return e.matrix.Permissions(role).Union(NewPermissionSet())
}

// grantedOnTarget checks the binding of the resource's team, or any binding for collection targets.
func (e *RoleBindingEngine) grantedOnTarget(s Subject, p Permission, r Resource) bool {
	if r.HasTeam() {
		role, ok := s.RoleIn(r.TeamID)
		return ok && e.matrix.Permissions(role).Has(p)
	}
	for _, role := range s.Bindings {
		if e.matrix.Permissions(role).Has(p) {
			return true
		}
	}
	return false
}
