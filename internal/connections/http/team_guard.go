package connhttp

import (
	"context"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
)

// TeamGuard enforces a permission on a target team for routes whose team comes from the body
// (create, draft test): editor+ of that team, through the configured Authorizer (CN-108).
type TeamGuard struct {
	authz auth.Authorizer
}

// Require returns the Authorizer's error when the caller lacks perm on team; without an Authorizer it allows.
func (g TeamGuard) Require(ctx context.Context, perm auth.Permission, team uuid.UUID) error {
	if g.authz == nil {
		return nil
	}
	obj := auth.ObjectRef{Type: ObjectConnection, TeamID: team.String()}
	return g.authz.Authorize(ctx, auth.PrincipalFrom(ctx), perm, obj)
}

// DraftTeam resolves the team a draft test runs for: the stored connection's owner when editing,
// else the requested or default team of the caller; the caller must be allowed to write there.
type DraftTeam struct {
	d Deps
}

// Resolve returns the team after checking the write permission on it.
func (t DraftTeam) Resolve(ctx context.Context, in DraftTestInput) (uuid.UUID, error) {
	team, err := t.target(ctx, in)
	if err != nil {
		return uuid.Nil, err
	}
	return team, TeamGuard{authz: t.d.Authz}.Require(ctx, PermWrite, team)
}

func (t DraftTeam) target(ctx context.Context, in DraftTestInput) (uuid.UUID, error) {
	if in.ConnectionID == nil {
		return OwnerTeamPolicy{}.Resolve(auth.PrincipalFrom(ctx), in.Owner)
	}
	owner, err := t.d.Service.TeamOf(ctx, in.ConnectionID.String())
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(owner)
}
