package engine

import (
	"context"
	"log/slog"

	"github.com/eventrouter/router/internal/alerts"
	"github.com/eventrouter/router/internal/platform/logging"
)

// ProvisionLog names provisioner log messages.
type ProvisionLog string

const (
	LogProvisioned     ProvisionLog = "alerts: default rules provisioned"
	LogProvisionFailed ProvisionLog = "alerts: default rule provisioning failed"
)

// Provisioner log attributes.
const (
	AttrKind  LogAttr = "kind"
	AttrCount LogAttr = "count"
)

// DefaultRuleProvisioner gives every route the workspace's default rules once: a route without a ledger entry
// for the kind gets the rule (enabled); a rule the user deleted keeps its ledger entry and is never re-created.
type DefaultRuleProvisioner struct {
	Gate     alerts.DefaultRuleGate
	Store    alerts.ProvisionStore
	Defaults []alerts.DefaultRule
	Log      *slog.Logger
}

// Ensure provisions the enabled defaults; failures are logged and retried on the next tick.
func (p DefaultRuleProvisioner) Ensure(ctx context.Context) {
	for _, rule := range p.Defaults {
		on, err := p.Gate.DefaultEnabled(ctx, rule.Kind)
		if err != nil || !on {
			continue
		}
		n, err := p.Store.ProvisionMissing(ctx, rule)
		if err != nil {
			p.Log.Warn(string(LogProvisionFailed), string(AttrKind), string(rule.Kind), string(logging.AttrError), err)
			continue
		}
		if n > 0 {
			p.Log.Info(string(LogProvisioned), string(AttrKind), string(rule.Kind), string(AttrCount), n)
		}
	}
}
