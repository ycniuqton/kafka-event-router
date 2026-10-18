package rbac

import (
	"context"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/store"
)

// CreateTeam is the command behind POST /teams.
type CreateTeam struct {
	Name                string
	RequireProdApproval *bool
}

// TeamDefaults holds what new teams get when the request leaves a field out.
type TeamDefaults struct {
	RequireProdApproval bool
}

// DefaultTeamDefaults: production deploys of new teams need a second person.
var DefaultTeamDefaults = TeamDefaults{RequireProdApproval: true}

// TeamDefaultsSource supplies the defaults of new teams (fixed, or the workspace settings).
type TeamDefaultsSource interface {
	TeamDefaults(ctx context.Context) TeamDefaults
}

// TeamDefaults implements TeamDefaultsSource with fixed values.
func (d TeamDefaults) TeamDefaults(context.Context) TeamDefaults {
	return d
}

// TeamService creates, renames, re-policies and deletes teams (workspace admins; audited).
type TeamService struct {
	tx       store.TxManager
	teams    TeamStore
	trail    *AuditTrail
	defaults TeamDefaultsSource
}

// TeamServiceDeps groups the collaborators of a TeamService.
type TeamServiceDeps struct {
	Tx       store.TxManager
	Teams    TeamStore
	Trail    *AuditTrail
	Defaults TeamDefaultsSource
}

// NewTeamService builds the service.
func NewTeamService(d TeamServiceDeps) *TeamService {
	return &TeamService{tx: d.Tx, teams: d.Teams, trail: d.Trail, defaults: d.Defaults}
}

// List returns every team.
func (s *TeamService) List(ctx context.Context) ([]Team, error) {
	return s.teams.List(ctx)
}

// Get returns one team.
func (s *TeamService) Get(ctx context.Context, id TeamID) (Team, error) {
	return s.teams.Get(ctx, id)
}

// Create validates the name (unique, case-insensitive) and creates the team.
func (s *TeamService) Create(ctx context.Context, cmd CreateTeam) (Team, error) {
	name, err := TeamName{}.Normalise(cmd.Name)
	if err != nil {
		return Team{}, err
	}
	require := s.defaults.TeamDefaults(ctx).RequireProdApproval
	if cmd.RequireProdApproval != nil {
		require = *cmd.RequireProdApproval
	}
	var out Team
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.uniqueName(ctx, name, NilTeam); err != nil {
			return err
		}
		if out, err = s.teams.Insert(ctx, name, require); err != nil {
			return err
		}
		return s.record(ctx, audit.ActionTeamCreated, nil, &out)
	})
	return out, err
}

// Update renames the team or changes its approval policy under If-Match.
func (s *TeamService) Update(ctx context.Context, id TeamID, c TeamChange, ifMatch store.RowVersion) (Team, error) {
	if c.Name != nil {
		name, err := TeamName{}.Normalise(*c.Name)
		if err != nil {
			return Team{}, err
		}
		c.Name = &name
	}
	var out Team
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		before, err := s.locked(ctx, id, ifMatch)
		if err != nil {
			return err
		}
		if c.Name != nil {
			if err := s.uniqueName(ctx, *c.Name, id); err != nil {
				return err
			}
		}
		if err := s.teams.Update(ctx, id, c, ifMatch); err != nil {
			return err
		}
		if out, err = s.teams.Get(ctx, id); err != nil {
			return err
		}
		return s.record(ctx, audit.ActionTeamUpdated, &before, &out)
	})
	return out, err
}

// Delete removes an empty team under If-Match; teams owning objects answer 409 team_not_empty.
func (s *TeamService) Delete(ctx context.Context, id TeamID, ifMatch store.RowVersion) error {
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		before, err := s.locked(ctx, id, ifMatch)
		if err != nil {
			return err
		}
		usage, err := s.teams.Usage(ctx, id)
		if err != nil {
			return err
		}
		if !usage.Empty() {
			return &TeamNotEmptyError{Usage: usage}
		}
		if err := s.teams.Delete(ctx, id); err != nil {
			return err
		}
		return s.record(ctx, audit.ActionTeamDeleted, &before, nil)
	})
}

// locked loads the team and checks the If-Match version.
func (s *TeamService) locked(ctx context.Context, id TeamID, ifMatch store.RowVersion) (Team, error) {
	before, err := s.teams.Get(ctx, id)
	if err != nil {
		return Team{}, err
	}
	if before.RowVersion != ifMatch {
		return Team{}, &store.PreconditionFailedError{Current: before.RowVersion}
	}
	return before, nil
}

func (s *TeamService) uniqueName(ctx context.Context, name string, except TeamID) error {
	taken, err := s.teams.NameTaken(ctx, name, except)
	if err != nil {
		return err
	}
	if taken {
		return ErrTeamNameTaken
	}
	return nil
}

func (s *TeamService) record(ctx context.Context, action audit.Action, before, after *Team) error {
	ref := after
	if ref == nil {
		ref = before
	}
	team := ref.ID
	return s.trail.Record(ctx, audit.Change{Action: action, Object: audit.ObjectTeam, ObjectID: ref.ID.String(), TeamID: &team,
		Before: TeamAudit{}.Of(before), After: TeamAudit{}.Of(after)})
}

// TeamAudit renders teams for audit diffs (nil → nil so creations and deletions diff fully).
type TeamAudit struct{}

// Of returns the audited fields of t.
func (TeamAudit) Of(t *Team) any {
	if t == nil {
		return nil
	}
	return map[string]any{"name": t.Name, "requireProdApproval": t.RequireProdApproval}
}
