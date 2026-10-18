package routesport

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/routes"
)

// GuardTimeout bounds the subject lookup of one write check (the guard has no request context).
type GuardTimeout time.Duration

const WriteCheckTimeout GuardTimeout = GuardTimeout(2 * time.Second)

// WriteGuard replaces routes' MVP team-role guard with the policy engine: route.write on the owning team
// (Editor, Approver — which inherits Editor rights — Admin, or a workspace admin). The system always writes.
type WriteGuard struct {
	Access rbac.Manager
}

// CanWrite implements routes.WriteGuard.
func (g WriteGuard) CanWrite(a routes.Actor, team routes.TeamID) bool {
	if a.System {
		return true
	}
	if a.UserID == nil {
		return false
	}
	teamID, err := uuid.Parse(string(team))
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(WriteCheckTimeout))
	defer cancel()
	ctx = auth.WithPrincipal(ctx, auth.Principal{Kind: auth.PrincipalUser, UserID: *a.UserID})
	return g.Access.Decide(ctx, rbac.PermRouteWrite, rbac.Resource{Type: rbac.ResourceRoute, TeamID: teamID}).Allowed
}
