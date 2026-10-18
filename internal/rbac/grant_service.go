package rbac

import (
	"context"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/store"
)

// ConnectionAccess answers "may team use this connection": it owns it or holds a grant (design FR-09).
type ConnectionAccess struct {
	connections ConnectionDirectory
	grants      GrantStore
}

// NewConnectionAccess builds the rule over the connection directory and grants.
func NewConnectionAccess(connections ConnectionDirectory, grants GrantStore) *ConnectionAccess {
	return &ConnectionAccess{connections: connections, grants: grants}
}

// Lookup returns one connection (ok=false when it does not exist).
func (a *ConnectionAccess) Lookup(ctx context.Context, id uuid.UUID) (ConnectionInfo, bool, error) {
	found, err := a.connections.Find(ctx, []uuid.UUID{id})
	if err != nil {
		return ConnectionInfo{}, false, err
	}
	c, ok := found[id]
	return c, ok, nil
}

// CanUse reports whether team owns the connection or was granted it; unknown connections are not usable.
func (a *ConnectionAccess) CanUse(ctx context.Context, team TeamID, conn uuid.UUID) (bool, error) {
	c, ok, err := a.Lookup(ctx, conn)
	if err != nil || !ok {
		return false, err
	}
	if c.TeamID == team {
		return true, nil
	}
	return a.grants.Exists(ctx, team, conn)
}

// GrantService manages team connection grants (workspace admins; audited; sole owner of grants).
type GrantService struct {
	tx     store.TxManager
	grants GrantStore
	access *ConnectionAccess
	teams  TeamStore
	trail  *AuditTrail
}

// GrantServiceDeps groups the collaborators of a GrantService.
type GrantServiceDeps struct {
	Tx     store.TxManager
	Grants GrantStore
	Access *ConnectionAccess
	Teams  TeamStore
	Trail  *AuditTrail
}

// NewGrantService builds the service.
func NewGrantService(d GrantServiceDeps) *GrantService {
	return &GrantService{tx: d.Tx, grants: d.Grants, access: d.Access, teams: d.Teams, trail: d.Trail}
}

// List returns the grants of an existing team.
func (s *GrantService) List(ctx context.Context, team TeamID) ([]ConnectionGrant, error) {
	if _, err := s.teams.Get(ctx, team); err != nil {
		return nil, err
	}
	return s.grants.List(ctx, team)
}

// Add grants team the use of conn; granting a team its own connection is rejected, repeats are no-ops.
func (s *GrantService) Add(ctx context.Context, team TeamID, conn uuid.UUID) (ConnectionGrant, error) {
	var out ConnectionGrant
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		c, err := s.target(ctx, team, conn)
		if err != nil {
			return err
		}
		if c.TeamID == team {
			return ErrGrantToOwner
		}
		added, err := s.grants.Insert(ctx, team, conn, PrincipalUser{}.Of(ctx))
		if err != nil {
			return err
		}
		if out, err = s.find(ctx, team, conn); err != nil || !added {
			return err
		}
		return s.record(ctx, audit.ActionGrantAdded, team, conn, nil, &out)
	})
	return out, err
}

// Remove revokes a grant (404 when there is none); routes using it fail validation on their next deploy.
func (s *GrantService) Remove(ctx context.Context, team TeamID, conn uuid.UUID) error {
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		before, err := s.find(ctx, team, conn)
		if err != nil {
			return err
		}
		if _, err := s.grants.Delete(ctx, team, conn); err != nil {
			return err
		}
		return s.record(ctx, audit.ActionGrantRemoved, team, conn, &before, nil)
	})
}

// target checks that the team and the connection exist.
func (s *GrantService) target(ctx context.Context, team TeamID, conn uuid.UUID) (ConnectionInfo, error) {
	if _, err := s.teams.Get(ctx, team); err != nil {
		return ConnectionInfo{}, err
	}
	c, ok, err := s.access.Lookup(ctx, conn)
	if err != nil {
		return ConnectionInfo{}, err
	}
	if !ok {
		return ConnectionInfo{}, ErrUnknownConnection
	}
	return c, nil
}

// find returns one grant of team (store.ErrNotFound when absent).
func (s *GrantService) find(ctx context.Context, team TeamID, conn uuid.UUID) (ConnectionGrant, error) {
	all, err := s.grants.List(ctx, team)
	if err != nil {
		return ConnectionGrant{}, err
	}
	for _, g := range all {
		if g.ConnectionID == conn {
			return g, nil
		}
	}
	return ConnectionGrant{}, store.ErrNotFound
}

func (s *GrantService) record(ctx context.Context, action audit.Action, team TeamID, conn uuid.UUID, before, after *ConnectionGrant) error {
	return s.trail.Record(ctx, audit.Change{Action: action, Object: audit.ObjectGrant, ObjectID: GrantKey{}.Of(team, conn),
		TeamID: &team, Before: GrantAudit{}.Of(before), After: GrantAudit{}.Of(after)})
}

// GrantKey renders the audit object id of a grant ("<teamId>/<connectionId>").
type GrantKey struct{}

// Of returns the key.
func (GrantKey) Of(team TeamID, conn uuid.UUID) string {
	return team.String() + "/" + conn.String()
}

// GrantAudit renders grants for audit diffs.
type GrantAudit struct{}

// Of returns the audited fields of g.
func (GrantAudit) Of(g *ConnectionGrant) any {
	if g == nil {
		return nil
	}
	return map[string]any{"teamId": g.TeamID, "connectionId": g.ConnectionID, "connection": g.ConnectionName}
}
