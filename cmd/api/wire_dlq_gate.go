package main

import (
	"context"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/approvals"
	"github.com/eventrouter/router/internal/dlq"
	"github.com/eventrouter/router/internal/platform/logging"
)

// gateDecisions maps the deploy gate's outcomes to replay decisions.
var gateDecisions = map[approvals.GateOutcome]dlq.GateDecision{
	approvals.OutcomeDeployNow:       dlq.GateAllow,
	approvals.OutcomeRequireApproval: dlq.GateRequireApproval,
	approvals.OutcomeDeny:            dlq.GateDeny,
}

// DLQReplayGate is the strict 4-eyes gate of dead-letter replays over the deploy gate: a replay reaches the active
// version's destinations, so it needs exactly the approval a deploy of that version to them would need.
type DLQReplayGate struct {
	Gate         approvals.Manager
	Destinations approvals.DestinationReader
}

// Check implements dlq.ReplayGate; unknown outcomes deny (fail closed).
func (g DLQReplayGate) Check(ctx context.Context, target dlq.RetryTarget) (dlq.GateDecision, error) {
	team, err := uuid.Parse(target.Route.TeamID)
	if err != nil {
		return dlq.GateDeny, nil
	}
	route, _ := uuid.Parse(target.Route.ID)
	dests, err := g.Destinations.Resources(ctx, target.Version.Config)
	if err != nil {
		return "", err
	}
	res, err := g.Gate.Gate(ctx, approvals.DeployTarget{RouteID: route, TeamID: team, Destinations: dests})
	if err != nil {
		return "", err
	}
	if d, ok := gateDecisions[res.Outcome]; ok {
		return d, nil
	}
	return dlq.GateDeny, nil
}

// dlqGate returns the replay gate; without approvals every replay is allowed (logged once at build).
func dlqGate(w *Wiring) dlq.ReplayGate {
	c, err := w.ApprovalsComponents()
	if err != nil {
		w.Deps.Log.Warn(string(LogApprovalsDisabled), string(logging.AttrError), err)
		return dlq.AlwaysAllowGate{}
	}
	return DLQReplayGate{Gate: c.Manager, Destinations: c.Reader}
}
