package rbac_test

import (
	"context"
	"errors"
	"testing"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/rbac/rbactest"
	"github.com/eventrouter/router/internal/store"
)

func newSettingsService(w *rbactest.World) (*rbac.SettingsService, *rbactest.Settings) {
	st := rbactest.NewSettings()
	return rbac.NewSettingsService(rbac.SettingsServiceDeps{Tx: rbactest.PassTx{}, Store: st, Catalog: rbac.DefaultSettingCatalog(),
		Trail: rbac.NewAuditTrail(w)}), st
}

// TestSettingsServiceDefaultsVersionsAndAudit: AI assist starts off, a change bumps the version and is audited
// once with before/after, an unchanged value writes nothing, a stale If-Match is 412.
func TestSettingsServiceDefaultsVersionsAndAudit(t *testing.T) {
	w := rbactest.NewWorld()
	svc, st := newSettingsService(w)
	ctx := context.Background()
	initial, err := svc.Get(ctx)
	if err != nil || initial.AIAssistEnabled || !initial.DefaultRequireProdApproval || initial.RowVersion != 1 {
		t.Fatalf("defaults = %+v %v", initial, err)
	}
	on := true
	updated, err := svc.Update(ctx, rbac.SettingsPatch{AIAssistEnabled: &on}, initial.RowVersion)
	if err != nil || !updated.AIAssistEnabled || updated.RowVersion != 2 || st.Locks != 1 {
		t.Fatalf("update = %+v %v locks=%d", updated, err, st.Locks)
	}
	if len(w.Entries) != 1 || w.Entries[0].Action != audit.ActionSettingsUpdated || w.Entries[0].Object != audit.ObjectSettings {
		t.Fatalf("audit = %+v", w.Entries)
	}
	same, err := svc.Update(ctx, rbac.SettingsPatch{AIAssistEnabled: &on}, updated.RowVersion)
	if err != nil || same.RowVersion != 2 || len(w.Entries) != 1 {
		t.Fatalf("no-op update must not write: %+v %v entries=%d", same, err, len(w.Entries))
	}
	var stale *store.PreconditionFailedError
	if _, err := svc.Update(ctx, rbac.SettingsPatch{AIAssistEnabled: &on}, initial.RowVersion); !errors.As(err, &stale) || stale.Current != 2 {
		t.Fatalf("stale If-Match: %v", err)
	}
	if enabled, err := svc.AIAssistEnabled(ctx); err != nil || !enabled {
		t.Fatalf("gate = %v %v", enabled, err)
	}
}

// TestTeamServiceUsesWorkspaceApprovalDefault: new teams follow default_require_prod_approval.
func TestTeamServiceUsesWorkspaceApprovalDefault(t *testing.T) {
	w := rbactest.NewWorld()
	settings, _ := newSettingsService(w)
	ctx := context.Background()
	off := false
	current, _ := settings.Get(ctx)
	if _, err := settings.Update(ctx, rbac.SettingsPatch{DefaultRequireProdApproval: &off}, current.RowVersion); err != nil {
		t.Fatal(err)
	}
	teams := rbac.NewTeamService(rbac.TeamServiceDeps{Tx: rbactest.PassTx{}, Teams: rbactest.NewTeamStore(),
		Trail: rbac.NewAuditTrail(w), Defaults: settings})
	team, err := teams.Create(ctx, rbac.CreateTeam{Name: "Growth"})
	if err != nil || team.RequireProdApproval {
		t.Fatalf("team = %+v %v", team, err)
	}
	on := true
	if team, err = teams.Create(ctx, rbac.CreateTeam{Name: "Risk", RequireProdApproval: &on}); err != nil || !team.RequireProdApproval {
		t.Fatalf("explicit value wins: %+v %v", team, err)
	}
}
