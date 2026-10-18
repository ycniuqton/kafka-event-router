package routes

import (
	"context"
	"encoding/json"
	"fmt"
)

// ApprovalPolicyKind selects the policy through ROUTER_APPROVAL_POLICY.
type ApprovalPolicyKind string

const (
	ApprovalNone ApprovalPolicyKind = "none"
	// ApprovalProduction is access-control's deploy gate (routesport.GatePolicy): production destinations need a
	// second person when the owning team requires it (strict 4-eyes, enforced by access-control).
	ApprovalProduction ApprovalPolicyKind = "team_production"
	// ApprovalAuto selects the production gate when it is registered, else no approval.
	ApprovalAuto ApprovalPolicyKind = "auto"
)

// ApprovalPolicy decides whether a deploy needs a second person (P2: production destinations).
type ApprovalPolicy interface {
	Kind() ApprovalPolicyKind
	Requires(ctx context.Context, route Route, doc json.RawMessage) (bool, error)
}

// NoApprovalPolicy never requires approval (MVP).
type NoApprovalPolicy struct{}

func (NoApprovalPolicy) Kind() ApprovalPolicyKind { return ApprovalNone }

func (NoApprovalPolicy) Requires(context.Context, Route, json.RawMessage) (bool, error) {
	return false, nil
}

// ApprovalPolicyManager is the only entry point deploys use; it delegates to the configured policy.
type ApprovalPolicyManager struct {
	policies map[ApprovalPolicyKind]ApprovalPolicy
	active   ApprovalPolicy
}

// NewApprovalPolicyManager registers policies and selects kind.
func NewApprovalPolicyManager(kind ApprovalPolicyKind, policies ...ApprovalPolicy) (*ApprovalPolicyManager, error) {
	m := &ApprovalPolicyManager{policies: map[ApprovalPolicyKind]ApprovalPolicy{}}
	for _, p := range append([]ApprovalPolicy{NoApprovalPolicy{}}, policies...) {
		m.policies[p.Kind()] = p
	}
	if kind == ApprovalAuto {
		kind = m.auto()
	}
	active, ok := m.policies[kind]
	if !ok {
		return nil, fmt.Errorf("routes: no approval policy %q registered", kind)
	}
	m.active = active
	return m, nil
}

// auto picks the production gate when registered.
func (m *ApprovalPolicyManager) auto() ApprovalPolicyKind {
	if _, ok := m.policies[ApprovalProduction]; ok {
		return ApprovalProduction
	}
	return ApprovalNone
}

// Kind is the selected policy.
func (m *ApprovalPolicyManager) Kind() ApprovalPolicyKind {
	return m.active.Kind()
}

// Requires asks the active policy.
func (m *ApprovalPolicyManager) Requires(ctx context.Context, route Route, doc json.RawMessage) (bool, error) {
	return m.active.Requires(ctx, route, doc)
}

// ApprovalRequester registers a pending deployment with the approvals feature (access-control) after the deploy
// transaction committed, so approvers are notified and older pending requests of the route are cancelled.
type ApprovalRequester interface {
	RequestApproval(ctx context.Context, d Deployment) error
}
