package routes

import (
	"context"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
)

// writeRoles are the team roles allowed to change a team's routes.
var writeRoles = map[auth.Role]bool{auth.RoleEditor: true, auth.RoleAdmin: true}

// ActorResolver turns the request principal into the routes Actor.
type ActorResolver struct{}

// From reads the principal of ctx; workloads and calls without a principal act as the system.
func (ActorResolver) From(ctx context.Context) Actor {
	p := auth.PrincipalFrom(ctx)
	if p.Kind == auth.PrincipalWorkload || !p.IsAuthenticated() {
		return Actor{System: true, Teams: map[TeamID]bool{}}
	}
	a := Actor{Teams: map[TeamID]bool{}, Ordered: []TeamID{}}
	if p.UserID != uuid.Nil {
		id := p.UserID
		a.UserID = &id
	}
	for _, tr := range p.Teams {
		if writeRoles[tr.Role] {
			a.Teams[TeamID(tr.TeamID.String())] = true
			a.Ordered = append(a.Ordered, TeamID(tr.TeamID.String()))
		}
	}
	return a
}

// WriteGuard decides whether an actor may change a team's routes (access-control may replace it).
type WriteGuard interface {
	CanWrite(a Actor, team TeamID) bool
}

// TeamRoleWriteGuard allows the system and editors/admins of the owning team.
type TeamRoleWriteGuard struct{}

// CanWrite implements WriteGuard.
func (TeamRoleWriteGuard) CanWrite(a Actor, team TeamID) bool {
	return a.System || a.Teams[team]
}
