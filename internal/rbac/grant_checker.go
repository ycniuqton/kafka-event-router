package rbac

import (
	"context"

	"github.com/google/uuid"
)

// ConnectionUser is the part of the Manager that answers connection-use questions.
type ConnectionUser interface {
	CanUseConnection(ctx context.Context, team TeamID, connID uuid.UUID) (bool, error)
}

// GrantChecker adapts Manager.CanUseConnection to routes' validation port (string ids, P2 grants rule).
type GrantChecker struct {
	manager ConnectionUser
}

// NewGrantChecker builds the adapter.
func NewGrantChecker(m ConnectionUser) *GrantChecker {
	return &GrantChecker{manager: m}
}

// CanUseConnection implements routes validation.GrantChecker; malformed ids are never usable.
func (g *GrantChecker) CanUseConnection(ctx context.Context, teamID, connectionID string) (bool, error) {
	team, err := uuid.Parse(teamID)
	if err != nil {
		return false, nil
	}
	conn, err := uuid.Parse(connectionID)
	if err != nil {
		return false, nil
	}
	return g.manager.CanUseConnection(ctx, team, conn)
}
