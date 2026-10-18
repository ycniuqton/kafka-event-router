package rbac

import (
	"context"

	"github.com/google/uuid"
)

// AdminGuard enforces "the last administrator can never be removed, demoted or disabled".
type AdminGuard struct {
	admins AdminStore
}

// NewAdminGuard builds the guard.
func NewAdminGuard(admins AdminStore) *AdminGuard {
	return &AdminGuard{admins: admins}
}

// Lock serialises admin-affecting changes for the current transaction.
func (g *AdminGuard) Lock(ctx context.Context) error {
	return g.admins.Lock(ctx)
}

// CheckBindingLoss fails when user loses its admin binding on team and nobody else administers it.
func (g *AdminGuard) CheckBindingLoss(ctx context.Context, before RoleBinding, workspaceAdmin bool) error {
	if before.Role != RoleAdmin || workspaceAdmin {
		return nil
	}
	team, err := g.admins.ActiveTeamAdmins(ctx, before.TeamID, before.UserID)
	if err != nil {
		return err
	}
	return g.requireOthers(ctx, team, before.UserID)
}

// CheckUserLoss fails when an active workspace admin is demoted or disabled and is the last one.
func (g *AdminGuard) CheckUserLoss(ctx context.Context, u User, c UserChange) error {
	loses := (c.WorkspaceAdmin != nil && !*c.WorkspaceAdmin) || (c.Disabled != nil && *c.Disabled)
	if !u.WorkspaceAdmin || u.Status == UserDisabled || !loses {
		return nil
	}
	return g.requireOthers(ctx, 0, u.ID)
}

func (g *AdminGuard) requireOthers(ctx context.Context, others int, user uuid.UUID) error {
	ws, err := g.admins.ActiveWorkspaceAdmins(ctx, user)
	if err != nil {
		return err
	}
	if others+ws == 0 {
		return ErrLastAdmin
	}
	return nil
}
