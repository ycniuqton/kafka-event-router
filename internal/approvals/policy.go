package approvals

import (
	"context"
	"fmt"
)

// PolicyKind selects the ApprovalPolicy implementation (ROUTER_APPROVALS_POLICY).
type PolicyKind string

const PolicyTeamProduction PolicyKind = "team_production"

// ApprovalPolicy is the base of the extensible deploy-gate domain.
type ApprovalPolicy interface {
	Kind() PolicyKind
	Evaluate(ctx context.Context, rights CallerRights, t DeployTarget, team TeamPolicy) GateResult
}

// PolicyRegistry maps policy kinds to implementations (config-selected, no switch on concrete types).
type PolicyRegistry struct {
	policies map[PolicyKind]ApprovalPolicy
}

// NewPolicyRegistry returns a registry with the built-in team production policy.
func NewPolicyRegistry() *PolicyRegistry {
	r := &PolicyRegistry{policies: map[PolicyKind]ApprovalPolicy{}}
	r.Register(TeamProductionPolicy{})
	return r
}

// Register adds or replaces a policy.
func (r *PolicyRegistry) Register(p ApprovalPolicy) {
	r.policies[p.Kind()] = p
}

// Build returns the configured policy.
func (r *PolicyRegistry) Build(kind PolicyKind) (ApprovalPolicy, error) {
	p, ok := r.policies[kind]
	if !ok {
		return nil, fmt.Errorf("approvals: no approval policy registered for %q", kind)
	}
	return p, nil
}

// TeamProductionPolicy (flow 5.4): non-prod deploys need deploy_nonprod; prod deploys go direct only when the
// team does not require approval and the caller may deploy to prod; otherwise writers must request approval.
type TeamProductionPolicy struct{}

// Kind implements ApprovalPolicy.
func (TeamProductionPolicy) Kind() PolicyKind { return PolicyTeamProduction }

// Evaluate implements ApprovalPolicy.
func (TeamProductionPolicy) Evaluate(_ context.Context, rights CallerRights, t DeployTarget, team TeamPolicy) GateResult {
	if !t.HasProd() {
		if rights.DeployNonProd {
			return GateResult{Outcome: OutcomeDeployNow, Reason: ReasonNonProd}
		}
		return GateResult{Outcome: OutcomeDeny, Reason: ReasonMissingPermission}
	}
	if !team.RequireProdApproval && rights.DeployProd {
		return GateResult{Outcome: OutcomeDeployNow, Reason: ReasonProdDirect}
	}
	if rights.Write {
		return GateResult{Outcome: OutcomeRequireApproval, Reason: ReasonProdApproval}
	}
	return GateResult{Outcome: OutcomeDeny, Reason: ReasonMissingPermission}
}
