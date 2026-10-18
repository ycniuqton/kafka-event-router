package validation

import (
	"context"

	"github.com/eventrouter/router/pkg/config"
)

// ConnectionAccessValidator checks that every referenced connection exists and, once grants
// exist (access-control P2), that the route's team may use connections of other teams.
type ConnectionAccessValidator struct {
	Lookup ConnectionLookup
	Grants GrantChecker
}

func (ConnectionAccessValidator) Name() ValidatorName     { return ValidatorConnectionAccess }
func (ConnectionAccessValidator) Modes() []ValidationMode { return AllModes }

// Validate reports unknown connections and forbidden cross-team use as errors.
func (v ConnectionAccessValidator) Validate(ctx context.Context, in ValidationInput) ([]config.Issue, error) {
	if in.Config == nil {
		return nil, nil
	}
	refs := ConnectionRefs{}.Of(in.Config)
	if len(refs) == 0 {
		return nil, nil
	}
	found, err := v.Lookup.Find(ctx, ConnectionRefs{}.IDs(refs))
	if err != nil {
		return nil, err
	}
	issues := []config.Issue{}
	for _, ref := range refs {
		issue, err := v.check(ctx, in.TeamID, ref, found)
		if err != nil {
			return nil, err
		}
		if issue != nil {
			issues = append(issues, *issue)
		}
	}
	return issues, nil
}

// check validates one reference.
func (v ConnectionAccessValidator) check(ctx context.Context, teamID string, ref ConnectionRef, found map[string]ConnectionInfo) (*config.Issue, error) {
	conn, ok := found[ref.ID]
	if !ok {
		is := config.NewError(config.IssueConnectionNotFound, ref.Path, "Connection %q does not exist. Pick an existing connection.", ref.ID)
		return &is, nil
	}
	if v.Grants == nil || conn.TeamID == teamID {
		return nil, nil
	}
	allowed, err := v.Grants.CanUseConnection(ctx, teamID, ref.ID)
	if err != nil || allowed {
		return nil, err
	}
	is := config.NewError(config.IssueConnectionForbidden, ref.Path, "Your team has no grant to use connection %q. Ask its owners for a grant.", conn.Name)
	return &is, nil
}
