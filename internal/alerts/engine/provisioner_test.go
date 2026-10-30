package engine

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/eventrouter/router/internal/alerts"
)

type gateFn func(alerts.AlertRuleKind) (bool, error)

func (g gateFn) DefaultEnabled(_ context.Context, k alerts.AlertRuleKind) (bool, error) { return g(k) }

type provisionLedger struct {
	routes  []string
	ledger  map[string]bool
	created []alerts.DefaultRule
	err     error
}

// ProvisionMissing creates the rule for every route not yet in the ledger (like the SQL statement).
func (l *provisionLedger) ProvisionMissing(_ context.Context, rule alerts.DefaultRule) (int, error) {
	if l.err != nil {
		return 0, l.err
	}
	n := 0
	for _, r := range l.routes {
		key := r + "|" + string(rule.Kind)
		if l.ledger[key] {
			continue
		}
		l.ledger[key] = true
		l.created = append(l.created, rule)
		n++
	}
	return n, nil
}

func TestDefaultRuleProvisionerOncePerRoute(t *testing.T) {
	store := &provisionLedger{routes: []string{"r1", "r2"}, ledger: map[string]bool{}}
	p := DefaultRuleProvisioner{Gate: alerts.AlwaysProvision{}, Store: store, Defaults: alerts.DefaultRules, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	p.Ensure(context.Background())
	p.Ensure(context.Background())
	if len(store.created) != 2 || store.created[0].Kind != alerts.KindRouterCausedDLQ || store.created[0].Threshold != 0 || store.created[0].Window != 15 {
		t.Fatalf("created %+v", store.created)
	}
	store.routes = append(store.routes, "r3")
	p.Ensure(context.Background())
	if len(store.created) != 3 {
		t.Fatalf("new route not provisioned: %+v", store.created)
	}
	if !alerts.KindLimits[alerts.KindRouterCausedDLQ].Contains(store.created[0].Threshold) {
		t.Fatal("provisioned threshold outside the kind limits")
	}
}

func TestDefaultRuleProvisionerRespectsWorkspaceSetting(t *testing.T) {
	store := &provisionLedger{routes: []string{"r1"}, ledger: map[string]bool{}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	off := DefaultRuleProvisioner{Gate: gateFn(func(alerts.AlertRuleKind) (bool, error) { return false, nil }), Store: store, Defaults: alerts.DefaultRules, Log: log}
	off.Ensure(context.Background())
	broken := DefaultRuleProvisioner{Gate: gateFn(func(alerts.AlertRuleKind) (bool, error) { return true, errors.New("db down") }), Store: store, Defaults: alerts.DefaultRules, Log: log}
	broken.Ensure(context.Background())
	if len(store.created) != 0 {
		t.Fatalf("provisioned while off: %+v", store.created)
	}
	store.err = errors.New("insert failed")
	DefaultRuleProvisioner{Gate: alerts.AlwaysProvision{}, Store: store, Defaults: alerts.DefaultRules, Log: log}.Ensure(context.Background())
}
