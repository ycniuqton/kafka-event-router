package alerts

import "context"

// DefaultRule is a rule every route of the workspace gets once (dead-letters 00001 §5).
type DefaultRule struct {
	Kind      AlertRuleKind
	Name      string
	Threshold float64
	Window    Window
}

// DefaultRuleName names the provisioned rules.
type DefaultRuleName string

const RouterCausedRuleName DefaultRuleName = "Router-caused dead letters"

// DefaultRouterCausedWindow is the evaluation window of the provisioned router-caused rule (minutes).
const DefaultRouterCausedWindow Window = 15

// DefaultRules lists the rules provisioned per route when the workspace keeps them on.
var DefaultRules = []DefaultRule{
	{Kind: KindRouterCausedDLQ, Name: string(RouterCausedRuleName), Threshold: 0, Window: DefaultRouterCausedWindow},
}

// DefaultRuleGate tells whether the workspace provisions a default rule kind (workspace settings).
type DefaultRuleGate interface {
	DefaultEnabled(ctx context.Context, kind AlertRuleKind) (bool, error)
}

// ProvisionStore creates default rules for routes that never had one of the kind (ledger alert_rule_provisions).
type ProvisionStore interface {
	ProvisionMissing(ctx context.Context, rule DefaultRule) (int, error)
}

// AlwaysProvision provisions every default rule (deployments without workspace settings).
type AlwaysProvision struct{}

// DefaultEnabled implements DefaultRuleGate.
func (AlwaysProvision) DefaultEnabled(context.Context, AlertRuleKind) (bool, error) { return true, nil }
