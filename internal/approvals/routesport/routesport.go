// Package routesport adapts approvals to the routes feature: the deploy gate as a routes ApprovalPolicy and
// routes' deployment lifecycle as the approvals Activator (re-validating the requested version first).
package routesport

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/approvals"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/routes/validation"
)

// PolicyKind is the ROUTER_APPROVAL_POLICY value that routes deploys through the access-control gate.
const PolicyKind = routes.ApprovalPolicyKind(approvals.PolicyTeamProduction)

// DenyCode is the problem code of a deploy the gate denies.
type DenyCode string

const CodeDeployNotAllowed DenyCode = "deploy_not_allowed"

// DeployDeniedError is the 403 of a deploy the caller may neither run nor request.
type DeployDeniedError struct {
	Result approvals.GateResult
}

func (e *DeployDeniedError) Error() string {
	return "You can't deploy this route: you need the Editor role in its team to deploy or request approval. Ask an admin."
}
func (e *DeployDeniedError) ProblemKind() problem.Kind { return problem.KindForbidden }
func (e *DeployDeniedError) ProblemCode() string       { return string(CodeDeployNotAllowed) }

// GatePolicy implements routes.ApprovalPolicy over approvals.Manager.Gate (strict 4-eyes on prod).
type GatePolicy struct {
	Gate         approvals.Manager
	Destinations approvals.DestinationReader
}

// Kind implements routes.ApprovalPolicy.
func (GatePolicy) Kind() routes.ApprovalPolicyKind { return PolicyKind }

// Requires reports whether the deploy must wait for approval; a denied deploy fails with 403.
func (p GatePolicy) Requires(ctx context.Context, route routes.Route, doc json.RawMessage) (bool, error) {
	team, err := uuid.Parse(string(route.TeamID))
	if err != nil {
		return false, err
	}
	id, _ := uuid.Parse(string(route.ID))
	dests, err := p.Destinations.Resources(ctx, doc)
	if err != nil {
		return false, err
	}
	res, err := p.Gate.Gate(ctx, approvals.DeployTarget{RouteID: id, TeamID: team, Destinations: dests})
	if err != nil {
		return false, err
	}
	if res.Outcome == approvals.OutcomeDeny {
		return false, &DeployDeniedError{Result: res}
	}
	return res.Outcome == approvals.OutcomeRequireApproval, nil
}

// Activator implements approvals.Activator with routes: re-validate the requested version, then activate it.
type Activator struct {
	Repos     routes.Repositories
	Validator *validation.ValidationManager
	Decider   routes.ApprovalDecider
}

// Revalidate fails with routes' 422 invalid-config problem when the requested version no longer validates
// (connection deleted, grant revoked, ...). Validators run in parallel, so it must run outside a transaction.
func (a Activator) Revalidate(ctx context.Context, deploymentID uuid.UUID) error {
	d, err := a.Repos.Deployments.Get(ctx, routes.DeploymentID(deploymentID.String()))
	if err != nil {
		return err
	}
	route, err := a.Repos.Routes.Get(ctx, d.RouteID)
	if err != nil {
		return err
	}
	v, err := a.Repos.Versions.GetByID(ctx, d.VersionID)
	if err != nil {
		return err
	}
	res := a.Validator.Run(ctx, validation.Request{Mode: validation.ModeDeploy, TeamID: string(route.TeamID), RouteID: string(route.ID),
		RouteName: route.Name, Raw: v.Config})
	if !res.Valid {
		return routes.NewInvalidConfigError(res.Issues)
	}
	return nil
}

// Activate moves pending_approval → rolling_out and switches the live version (routes' lifecycle, caller's tx).
func (a Activator) Activate(ctx context.Context, deploymentID, approver uuid.UUID) error {
	_, err := a.Decider.Activate(ctx, routes.DeploymentID(deploymentID.String()), approver)
	return err
}
