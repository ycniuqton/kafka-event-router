package rbac

import (
	"context"
	"encoding/json"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/store"
)

// SettingsService reads workspace settings for every member and changes them for workspace admins
// (If-Match on the aggregate version, audited as settings.updated with before/after).
type SettingsService struct {
	tx      store.TxManager
	store   SettingsStore
	catalog SettingCatalog
	trail   *AuditTrail
}

// SettingsServiceDeps groups the collaborators of a SettingsService.
type SettingsServiceDeps struct {
	Tx      store.TxManager
	Store   SettingsStore
	Catalog SettingCatalog
	Trail   *AuditTrail
}

// NewSettingsService builds the service.
func NewSettingsService(d SettingsServiceDeps) *SettingsService {
	return &SettingsService{tx: d.Tx, store: d.Store, catalog: d.Catalog, trail: d.Trail}
}

// Get returns the current settings (defaults for keys never written).
func (s *SettingsService) Get(ctx context.Context) (WorkspaceSettings, error) {
	stored, err := s.store.Load(ctx)
	if err != nil {
		return WorkspaceSettings{}, err
	}
	return s.catalog.Resolve(stored), nil
}

// Update applies the patch under If-Match; unchanged values are not rewritten.
func (s *SettingsService) Update(ctx context.Context, patch SettingsPatch, ifMatch store.RowVersion) (WorkspaceSettings, error) {
	var out WorkspaceSettings
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.store.Lock(ctx); err != nil {
			return err
		}
		before, err := s.Get(ctx)
		if err != nil {
			return err
		}
		if before.RowVersion != ifMatch {
			return &store.PreconditionFailedError{Current: before.RowVersion}
		}
		changed, err := s.write(ctx, before, patch)
		if err != nil || !changed {
			out = before
			return err
		}
		if out, err = s.Get(ctx); err != nil {
			return err
		}
		return s.trail.Record(ctx, audit.Change{Action: audit.ActionSettingsUpdated, Object: audit.ObjectSettings,
			ObjectID: string(audit.ObjectSettings), Before: before.Values(), After: out.Values()})
	})
	return out, err
}

// write stores every patched value that differs from before and reports whether anything changed.
func (s *SettingsService) write(ctx context.Context, before WorkspaceSettings, patch SettingsPatch) (bool, error) {
	changed := false
	for _, def := range s.catalog {
		next := def.Patched(patch)
		if next == nil || *next == *def.Field(&before) {
			continue
		}
		raw, err := json.Marshal(*next)
		if err != nil {
			return false, err
		}
		if err := s.store.Put(ctx, def.Key, raw, PrincipalUser{}.Of(ctx)); err != nil {
			return false, err
		}
		changed = true
	}
	return changed, nil
}

// AIAssistEnabled implements AIAssistGate for transform-studio's assistant (default off).
func (s *SettingsService) AIAssistEnabled(ctx context.Context) (bool, error) {
	current, err := s.Get(ctx)
	return current.AIAssistEnabled, err
}

// TeamDefaults implements TeamDefaultsSource: new teams follow the workspace approval default.
func (s *SettingsService) TeamDefaults(ctx context.Context) TeamDefaults {
	current, err := s.Get(ctx)
	if err != nil {
		return DefaultTeamDefaults
	}
	return TeamDefaults{RequireProdApproval: current.DefaultRequireProdApproval}
}

// AIAssistGate is the port transform-studio reads before serving assistant requests.
type AIAssistGate interface {
	AIAssistEnabled(ctx context.Context) (bool, error)
}

// Values returns the settings without their version (audit before/after).
func (w WorkspaceSettings) Values() WorkspaceSettings {
	w.RowVersion = 0
	return w
}
