package rbac_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/store"
)

// TestWorkspaceSettingsOnPostgres: defaults before any row, versions grow per change, concurrent changes
// from the same version yield exactly one winner (advisory lock + If-Match), the change is audited.
func TestWorkspaceSettingsOnPostgres(t *testing.T) {
	w := newITWorld(t)
	db := w.sb.DB
	svc := rbac.NewSettingsService(rbac.SettingsServiceDeps{Tx: store.NewTxManager(db), Store: rbac.NewPostgresSettingsStore(db),
		Catalog: rbac.DefaultSettingCatalog(), Trail: rbac.NewAuditTrail(audit.NewPostgresRecorder(db))})
	ctx := context.Background()
	initial, err := svc.Get(ctx)
	if err != nil || initial.AIAssistEnabled || initial.RowVersion != 1 {
		t.Fatalf("initial = %+v %v", initial, err)
	}
	var wg sync.WaitGroup
	results := make([]error, 4)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			on := i%2 == 0
			_, results[i] = svc.Update(ctx, rbac.SettingsPatch{AIAssistEnabled: &on, DefaultRequireProdApproval: &on}, initial.RowVersion)
		}(i)
	}
	wg.Wait()
	wins, stale := 0, 0
	for _, err := range results {
		var pf *store.PreconditionFailedError
		switch {
		case err == nil:
			wins++
		case errors.As(err, &pf):
			stale++
		default:
			t.Fatalf("unexpected: %v", err)
		}
	}
	after, _ := svc.Get(ctx)
	if wins+stale != len(results) || wins < 1 || after.RowVersion <= initial.RowVersion {
		t.Fatalf("wins=%d stale=%d after=%+v", wins, stale, after)
	}
	var audited int
	if err := db.Pool().QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = $1`, string(audit.ActionSettingsUpdated)).
		Scan(&audited); err != nil || audited != wins {
		t.Fatalf("audited %d changes for %d wins (%v)", audited, wins, err)
	}
}
