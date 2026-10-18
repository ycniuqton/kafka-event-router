package rbac

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
)

// Role is a team role; it is the same type as platform-core's auth.Role (Postgres enum team_role).
type Role = auth.Role

const (
	RoleViewer   = auth.RoleViewer
	RoleEditor   = auth.RoleEditor
	RoleApprover = auth.RoleApprover
	RoleAdmin    = auth.RoleAdmin
)

// TeamID identifies a team.
type TeamID = uuid.UUID

// Scope tells whether a permission applies to one team or to the whole workspace.
type Scope string

const (
	ScopeWorkspace Scope = "workspace"
	ScopeTeam      Scope = "team"
)

// EnvTag is the environment tag of a connection / deploy target.
type EnvTag string

const (
	EnvProd    EnvTag = "prod"
	EnvStaging EnvTag = "staging"
	EnvDev     EnvTag = "dev"
)

// Roles is the role catalogue, ordered from least to most privileged.
type Roles struct{}

// Ordered returns every role, least privileged first.
func (Roles) Ordered() []Role {
	return []Role{RoleViewer, RoleEditor, RoleApprover, RoleAdmin}
}

// Valid reports whether r is a known role.
func (Roles) Valid(r Role) bool {
	for _, known := range (Roles{}).Ordered() {
		if known == r {
			return true
		}
	}
	return false
}

// RoleSet is a set of roles (e.g. the roles assignable in the current phase).
type RoleSet map[Role]bool

// ParseRoleSet converts configured role names; unknown names are an error.
func ParseRoleSet(names []string) (RoleSet, error) {
	set := RoleSet{}
	for _, n := range names {
		r := Role(strings.ToLower(strings.TrimSpace(n)))
		if !(Roles{}).Valid(r) {
			return nil, fmt.Errorf("rbac: unknown role %q", n)
		}
		set[r] = true
	}
	return set, nil
}

// Has reports whether r is in the set.
func (s RoleSet) Has(r Role) bool {
	return s[r]
}

// Ordered returns the set's roles, least privileged first.
func (s RoleSet) Ordered() []Role {
	var out []Role
	for _, r := range (Roles{}).Ordered() {
		if s[r] {
			out = append(out, r)
		}
	}
	return out
}
