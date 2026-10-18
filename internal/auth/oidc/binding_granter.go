package oidc

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sort"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/rbac"
)

// TeamDirectory resolves configured team names to existing teams (mappings never create teams).
type TeamDirectory interface {
	FindByName(ctx context.Context, name string) (uuid.UUID, bool, error)
}

// BindingWriter is the part of access-control's BindingService the granter uses (audit, guards included).
type BindingWriter interface {
	List(ctx context.Context, f rbac.BindingFilter) ([]rbac.RoleBinding, error)
	Set(ctx context.Context, cmd rbac.SetBinding) (rbac.RoleBinding, error)
}

// GrantLog names the granter's log messages and attributes.
type GrantLog string

const (
	LogGrantUnknownTeam   GrantLog = "oidc role mapping: team does not exist; grant skipped"
	LogGrantNotAssignable GrantLog = "oidc role mapping: role not assignable in this phase; grant skipped"
	LogAttrTeam           GrantLog = "team"
	LogAttrRole           GrantLog = "role"
)

// RoleRank orders team roles from least to most privileged (access-control's catalogue).
type RoleRank struct{}

// Of returns the rank of r (-1 for unknown roles).
func (RoleRank) Of(r auth.Role) int {
	return slices.Index(rbac.Roles{}.Ordered(), r)
}

// BindingGranter is additive and upgrade-only: a mapped role is created or raised, never lowered or
// removed, so bindings an admin assigned by hand always survive the next login.
type BindingGranter struct {
	teams    TeamDirectory
	bindings BindingWriter
	log      *slog.Logger
	rank     RoleRank
}

// NewBindingGranter builds the granter.
func NewBindingGranter(teams TeamDirectory, bindings BindingWriter, log *slog.Logger) *BindingGranter {
	return &BindingGranter{teams: teams, bindings: bindings, log: log}
}

// Grant implements RoleGranter.
func (g *BindingGranter) Grant(ctx context.Context, u auth.User, grants []GroupGrant) error {
	wanted := g.strongest(grants)
	if len(wanted) == 0 {
		return nil
	}
	current, err := g.bindings.List(ctx, rbac.BindingFilter{UserID: &u.ID})
	if err != nil {
		return err
	}
	byTeam := map[uuid.UUID]rbac.RoleBinding{}
	for _, b := range current {
		byTeam[b.TeamID] = b
	}
	teams := make([]string, 0, len(wanted))
	for team := range wanted {
		teams = append(teams, team)
	}
	sort.Strings(teams)
	for _, team := range teams {
		if err := g.apply(ctx, u, GroupGrant{Team: team, Role: wanted[team]}, byTeam); err != nil {
			return err
		}
	}
	return nil
}

// strongest keeps the most privileged role per team.
func (g *BindingGranter) strongest(grants []GroupGrant) map[string]auth.Role {
	out := map[string]auth.Role{}
	for _, gr := range grants {
		if have, ok := out[gr.Team]; !ok || g.rank.Of(gr.Role) > g.rank.Of(have) {
			out[gr.Team] = gr.Role
		}
	}
	return out
}

func (g *BindingGranter) apply(ctx context.Context, u auth.User, gr GroupGrant, byTeam map[uuid.UUID]rbac.RoleBinding) error {
	teamID, found, err := g.teams.FindByName(ctx, gr.Team)
	if err != nil || !found {
		g.skipped(ctx, err, LogGrantUnknownTeam, gr)
		return err
	}
	cmd := rbac.SetBinding{UserID: u.ID, TeamID: teamID, Role: gr.Role}
	if existing, ok := byTeam[teamID]; ok {
		if g.rank.Of(existing.Role) >= g.rank.Of(gr.Role) {
			return nil
		}
		version := existing.RowVersion
		cmd.IfMatch = &version
	}
	_, err = g.bindings.Set(ctx, cmd)
	if errors.Is(err, rbac.ErrRoleNotAssignable) {
		g.skipped(ctx, nil, LogGrantNotAssignable, gr)
		return nil
	}
	return err
}

func (g *BindingGranter) skipped(ctx context.Context, err error, msg GrantLog, gr GroupGrant) {
	if err != nil {
		return
	}
	g.log.WarnContext(ctx, string(msg), string(LogAttrTeam), gr.Team, string(LogAttrRole), gr.Role)
}
