package main

import (
	"context"

	"github.com/eventrouter/router/internal/alerts"
	"github.com/eventrouter/router/internal/alerts/engine"
	"github.com/eventrouter/router/internal/alerts/pgstore"
	"github.com/eventrouter/router/internal/rbac"
)

// alertDefaultSettings maps default rule kinds to the workspace setting that switches them.
var alertDefaultSettings = map[alerts.AlertRuleKind]func(rbac.WorkspaceSettings) bool{
	alerts.KindRouterCausedDLQ: func(s rbac.WorkspaceSettings) bool { return s.RouterCausedAlertDefault },
}

// WorkspaceAlertDefaults reads the workspace settings that turn default alert rules on or off.
type WorkspaceAlertDefaults struct {
	Settings *rbac.SettingsService
}

// DefaultEnabled implements alerts.DefaultRuleGate; kinds without a setting are never provisioned.
func (d WorkspaceAlertDefaults) DefaultEnabled(ctx context.Context, kind alerts.AlertRuleKind) (bool, error) {
	field, ok := alertDefaultSettings[kind]
	if !ok {
		return false, nil
	}
	s, err := d.Settings.Get(ctx)
	if err != nil {
		return false, err
	}
	return field(s), nil
}

// alertDefaults builds the default-rule provisioner (dead-letters 00001 §5): the router-caused dead-letter rule
// for every route, on unless an admin turned the workspace default off.
func alertDefaults(w *Wiring) *engine.DefaultRuleProvisioner {
	var gate alerts.DefaultRuleGate = alerts.AlwaysProvision{}
	if w.Access != nil && w.Access.Settings != nil {
		gate = WorkspaceAlertDefaults{Settings: w.Access.Settings}
	}
	return &engine.DefaultRuleProvisioner{Gate: gate, Store: pgstore.NewProvisionStore(w.Deps.DB), Defaults: alerts.DefaultRules, Log: w.Deps.Log}
}
