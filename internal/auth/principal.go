// Package auth authenticates users, API tokens and workloads. Business code only sees Principal;
// authenticator and login-flow implementations are selected by configuration through registries.
package auth

import (
	"context"

	"github.com/google/uuid"
)

// PrincipalKind tells what kind of caller a Principal represents.
type PrincipalKind string

const (
	PrincipalUser      PrincipalKind = "user"
	PrincipalAPIToken  PrincipalKind = "api_token"
	PrincipalWorkload  PrincipalKind = "workload"
	PrincipalAnonymous PrincipalKind = "anonymous"
)

// Role is a team role (team_role enum in Postgres).
type Role string

const (
	RoleViewer   Role = "viewer"
	RoleEditor   Role = "editor"
	RoleApprover Role = "approver"
	RoleAdmin    Role = "admin"
)

// TeamRole is one role binding of a user on a team.
type TeamRole struct {
	TeamID   uuid.UUID `json:"id"`
	TeamName string    `json:"name"`
	Role     Role      `json:"role"`
}

// Principal is the authenticated caller of a request.
type Principal struct {
	Kind               PrincipalKind
	UserID             uuid.UUID
	Email              string
	DisplayName        string
	Teams              []TeamRole
	SessionID          string
	CSRFToken          string
	TokenID            uuid.UUID
	Scopes             []TokenScope
	WorkloadName       string
	MustChangePassword bool
}

// IsAuthenticated reports whether the principal is anything but anonymous.
func (p Principal) IsAuthenticated() bool {
	return p.Kind != "" && p.Kind != PrincipalAnonymous
}

// HasRole reports whether the principal holds one of roles on any team.
func (p Principal) HasRole(roles ...Role) bool {
	for _, tr := range p.Teams {
		for _, r := range roles {
			if tr.Role == r {
				return true
			}
		}
	}
	return false
}

// Anonymous is the principal of unauthenticated requests.
var Anonymous = Principal{Kind: PrincipalAnonymous}

type principalKey struct{}

// WithPrincipal stores the principal in ctx.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom returns the request principal, or Anonymous.
func PrincipalFrom(ctx context.Context) Principal {
	if p, ok := ctx.Value(principalKey{}).(Principal); ok {
		return p
	}
	return Anonymous
}
