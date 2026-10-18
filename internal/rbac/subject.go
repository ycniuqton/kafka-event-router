package rbac

import (
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
)

// SubjectKind mirrors platform-core's principal kinds.
type SubjectKind string

const (
	SubjectUser     SubjectKind = SubjectKind(auth.PrincipalUser)
	SubjectToken    SubjectKind = SubjectKind(auth.PrincipalAPIToken)
	SubjectWorkload SubjectKind = SubjectKind(auth.PrincipalWorkload)
)

// Subject is everything the policy engine knows about a caller.
type Subject struct {
	UserID         uuid.UUID
	Kind           SubjectKind
	WorkspaceAdmin bool
	Bindings       map[TeamID]Role
	Disabled       bool
}

// HasAnyBinding reports whether the subject belongs to at least one team.
func (s Subject) HasAnyBinding() bool {
	return len(s.Bindings) > 0
}

// RoleIn returns the subject's role in team.
func (s Subject) RoleIn(team TeamID) (Role, bool) {
	r, ok := s.Bindings[team]
	return r, ok
}
