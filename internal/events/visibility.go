package events

import (
	"fmt"

	"github.com/eventrouter/router/internal/auth"
)

// VisibilityKind selects which team events a socket receives (ROUTER_EVENTS_VISIBILITY).
type VisibilityKind string

const (
	VisibilityOpen VisibilityKind = "open"
	VisibilityTeam VisibilityKind = "team"
)

// VisibilityPolicy decides whether principal p may see an event of team ("" = workspace-wide).
type VisibilityPolicy interface {
	Kind() VisibilityKind
	Allows(p auth.Principal, team string) bool
}

// OpenVisibility follows the "read is open across teams" decision: every authenticated user sees every event.
type OpenVisibility struct{}

func (OpenVisibility) Kind() VisibilityKind { return VisibilityOpen }

// Allows implements VisibilityPolicy.
func (OpenVisibility) Allows(p auth.Principal, _ string) bool {
	return p.IsAuthenticated()
}

// TeamVisibility only sends a team's events to its members (workspace-wide events go to everyone).
type TeamVisibility struct{}

func (TeamVisibility) Kind() VisibilityKind { return VisibilityTeam }

// Allows implements VisibilityPolicy.
func (TeamVisibility) Allows(p auth.Principal, team string) bool {
	if !p.IsAuthenticated() {
		return false
	}
	if team == "" {
		return true
	}
	for _, t := range p.Teams {
		if t.TeamID.String() == team {
			return true
		}
	}
	return false
}

// VisibilityManager selects the configured policy.
type VisibilityManager struct {
	byKind map[VisibilityKind]VisibilityPolicy
}

// NewVisibilityManager registers the built-in policies.
func NewVisibilityManager() *VisibilityManager {
	m := &VisibilityManager{byKind: map[VisibilityKind]VisibilityPolicy{}}
	for _, p := range []VisibilityPolicy{OpenVisibility{}, TeamVisibility{}} {
		m.byKind[p.Kind()] = p
	}
	return m
}

// Select returns the policy of kind.
func (m *VisibilityManager) Select(kind VisibilityKind) (VisibilityPolicy, error) {
	p, ok := m.byKind[kind]
	if !ok {
		return nil, fmt.Errorf("events: unknown visibility %q", kind)
	}
	return p, nil
}
