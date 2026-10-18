// Package pgstore implements the alerts repositories on Postgres (alert_rules, alert_events,
// alert_event_acks) and the route directory alerting reads.
package pgstore

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/alerts"
	"github.com/eventrouter/router/internal/store"
)

// Table names the versioned table of rules.
type Table string

const (
	TableRules Table = "alert_rules"
	ColumnID   Table = "id"
)

const ruleColumns = `id, route_id, name, kind::text, threshold, (extract(epoch FROM "window") / 60)::int, channels, enabled,
       created_by, row_version, created_at, updated_at`

const (
	listRulesSQL  = `SELECT ` + ruleColumns + ` FROM alert_rules WHERE ($1::uuid IS NULL OR route_id = $1) ORDER BY created_at, id`
	getRuleSQL    = `SELECT ` + ruleColumns + ` FROM alert_rules WHERE id = $1`
	enabledSQL    = `SELECT ` + ruleColumns + ` FROM alert_rules WHERE enabled ORDER BY id`
	insertRuleSQL = `INSERT INTO alert_rules (route_id, name, kind, threshold, "window", channels, enabled, created_by)
VALUES ($1, $2, $3::alert_rule_kind, $4, make_interval(mins => $5), $6, $7, $8) RETURNING ` + ruleColumns
	updateRuleSet = `name = $1, kind = $2::alert_rule_kind, threshold = $3, "window" = make_interval(mins => $4), channels = $5,
       enabled = $6, updated_at = now()`
	deleteRuleSQL = `DELETE FROM alert_rules WHERE id = $1`
)

// RuleStore implements alerts.RuleRepository.
type RuleStore struct {
	db *store.DB
}

// NewRuleStore builds the store.
func NewRuleStore(db *store.DB) *RuleStore {
	return &RuleStore{db: db}
}

// List returns the rules of one route, or all rules when routeID is nil.
func (s *RuleStore) List(ctx context.Context, routeID *uuid.UUID) ([]alerts.AlertRule, error) {
	return s.query(ctx, listRulesSQL, routeID)
}

// ListEnabled returns every enabled rule.
func (s *RuleStore) ListEnabled(ctx context.Context) ([]alerts.AlertRule, error) {
	return s.query(ctx, enabledSQL)
}

// Get returns one rule or alerts.ErrRuleNotFound.
func (s *RuleStore) Get(ctx context.Context, id alerts.RuleID) (alerts.AlertRule, error) {
	r, err := RuleScanner{}.Scan(s.db.Q(ctx).QueryRow(ctx, getRuleSQL, id))
	if err == pgx.ErrNoRows {
		return alerts.AlertRule{}, alerts.ErrRuleNotFound
	}
	return r, s.db.Translate(err)
}

// Insert stores a new rule and returns it with generated fields.
func (s *RuleStore) Insert(ctx context.Context, r alerts.AlertRule) (alerts.AlertRule, error) {
	ch, err := json.Marshal(r.Channels)
	if err != nil {
		return alerts.AlertRule{}, err
	}
	out, err := RuleScanner{}.Scan(s.db.Q(ctx).QueryRow(ctx, insertRuleSQL, r.RouteID, r.Name, string(r.Kind), r.Threshold,
		int(r.Window), ch, r.Enabled, r.CreatedBy))
	return out, s.db.Translate(err)
}

// Update replaces the editable fields when the row version still matches.
func (s *RuleStore) Update(ctx context.Context, r alerts.AlertRule, expected store.RowVersion) (alerts.AlertRule, error) {
	ch, err := json.Marshal(r.Channels)
	if err != nil {
		return alerts.AlertRule{}, err
	}
	args := []any{r.Name, string(r.Kind), r.Threshold, int(r.Window), ch, r.Enabled}
	upd := store.VersionedUpdate{Table: string(TableRules), IDColumn: string(ColumnID)}
	if _, err := upd.Exec(ctx, s.db, updateRuleSet, args, r.ID, expected); err != nil {
		return alerts.AlertRule{}, err
	}
	return s.Get(ctx, r.ID)
}

// Delete removes a rule (its history cascades).
func (s *RuleStore) Delete(ctx context.Context, id alerts.RuleID) error {
	tag, err := s.db.Q(ctx).Exec(ctx, deleteRuleSQL, id)
	if err != nil {
		return s.db.Translate(err)
	}
	if tag.RowsAffected() == 0 {
		return alerts.ErrRuleNotFound
	}
	return nil
}

func (s *RuleStore) query(ctx context.Context, sql string, args ...any) ([]alerts.AlertRule, error) {
	rows, err := s.db.Q(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, s.db.Translate(err)
	}
	defer rows.Close()
	out := []alerts.AlertRule{}
	for rows.Next() {
		r, err := RuleScanner{}.Scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, s.db.Translate(rows.Err())
}

// RuleScanner decodes one rule row.
type RuleScanner struct{}

// Scan reads the ruleColumns projection.
func (RuleScanner) Scan(row pgx.Row) (alerts.AlertRule, error) {
	var r alerts.AlertRule
	var kind string
	var window int
	var channels []byte
	var version int64
	var created, updated time.Time
	if err := row.Scan(&r.ID, &r.RouteID, &r.Name, &kind, &r.Threshold, &window, &channels, &r.Enabled, &r.CreatedBy,
		&version, &created, &updated); err != nil {
		return alerts.AlertRule{}, err
	}
	r.Kind, r.Window, r.RowVersion, r.CreatedAt, r.UpdatedAt = alerts.AlertRuleKind(kind), alerts.Window(window), store.RowVersion(version), created, updated
	r.Channels = []alerts.ChannelRef{}
	if len(channels) > 0 {
		if err := json.Unmarshal(channels, &r.Channels); err != nil {
			return alerts.AlertRule{}, err
		}
	}
	return r, nil
}
