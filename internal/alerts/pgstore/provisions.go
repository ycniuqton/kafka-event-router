package pgstore

import (
	"context"

	"github.com/eventrouter/router/internal/alerts"
	"github.com/eventrouter/router/internal/store"
)

// provisionSQL inserts the default rule for live routes without a ledger entry of the kind and records the entry
// (migration 3100); routes created later are picked up by the next tick.
const provisionSQL = `WITH missing AS (
    SELECT r.id FROM routes r
     WHERE r.desired_state <> 'archived'
       AND NOT EXISTS (SELECT 1 FROM alert_rule_provisions p WHERE p.route_id = r.id AND p.kind = $1::alert_rule_kind)
), created AS (
    INSERT INTO alert_rules (route_id, name, kind, threshold, "window", channels, enabled)
    SELECT id, $2, $1::alert_rule_kind, $3, make_interval(mins => $4), '[]'::jsonb, true FROM missing
    RETURNING route_id
)
INSERT INTO alert_rule_provisions (route_id, kind)
SELECT route_id, $1::alert_rule_kind FROM created
ON CONFLICT DO NOTHING`

// ProvisionStore implements alerts.ProvisionStore.
type ProvisionStore struct {
	db *store.DB
}

// NewProvisionStore builds the store.
func NewProvisionStore(db *store.DB) *ProvisionStore {
	return &ProvisionStore{db: db}
}

// ProvisionMissing implements alerts.ProvisionStore and returns how many rules were created.
func (s *ProvisionStore) ProvisionMissing(ctx context.Context, rule alerts.DefaultRule) (int, error) {
	tag, err := s.db.Q(ctx).Exec(ctx, provisionSQL, string(rule.Kind), rule.Name, rule.Threshold, int(rule.Window))
	if err != nil {
		return 0, s.db.Translate(err)
	}
	return int(tag.RowsAffected()), nil
}
