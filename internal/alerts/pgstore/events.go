package pgstore

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/alerts"
	"github.com/eventrouter/router/internal/store"
)

const eventColumns = `id, rule_id, route_id, state::text, value, fired_at, resolved_at, notify_pending::text, notify_attempts,
       notified_at, COALESCE(notify_error, '')`

const (
	firingSQL = `SELECT ` + eventColumns + ` FROM alert_events WHERE rule_id = $1 AND state = 'firing'`
	fireSQL   = `INSERT INTO alert_events (rule_id, route_id, state, value, fired_at, notify_pending)
VALUES ($1, $2, 'firing', $3, $4, 'firing') ON CONFLICT (rule_id) WHERE state = 'firing' DO NOTHING RETURNING ` + eventColumns
	resolveSQL = `UPDATE alert_events SET state = 'resolved', resolved_at = $2, notify_pending = 'resolved', notify_attempts = 0,
       notify_error = NULL WHERE id = $1 AND state = 'firing' RETURNING ` + eventColumns
	pendingSQL = `SELECT ` + eventColumns + ` FROM alert_events WHERE notify_pending IS NOT NULL AND notify_attempts < $1
 ORDER BY id LIMIT 500`
	notifiedSQL = `UPDATE alert_events SET notify_pending = NULL, notified_at = $2, notify_error = NULL,
       notify_attempts = notify_attempts + 1 WHERE id = $1`
	failedSQL = `UPDATE alert_events SET notify_attempts = notify_attempts + 1, notify_error = $2,
       notify_pending = CASE WHEN $3 THEN NULL ELSE notify_pending END WHERE id = $1`
	ackSQL = `WITH upd AS (UPDATE alert_events SET acked_by = COALESCE(acked_by, $2), acked_at = COALESCE(acked_at, $3)
        WHERE id = $1 RETURNING id),
     ins AS (INSERT INTO alert_event_acks (event_id, user_id, acked_at) SELECT id, $2, $3 FROM upd ON CONFLICT DO NOTHING)
SELECT count(*) FROM upd`
	feedColumns = `e.id, e.rule_id, r.name, e.route_id, rt.name, rt.team_id, r.kind::text, r.threshold, (extract(epoch FROM r."window") / 60)::int, e.state::text, e.value,
       e.fired_at, e.resolved_at, (k.user_id IS NOT NULL)`
	feedFrom = ` FROM alert_events e JOIN alert_rules r ON r.id = e.rule_id JOIN routes rt ON rt.id = e.route_id
  LEFT JOIN alert_event_acks k ON k.event_id = e.id AND k.user_id = $1`
	feedSQL = `SELECT ` + feedColumns + feedFrom + `
 WHERE ($2::text IS NULL OR e.state::text = $2)
   AND ($3::timestamptz IS NULL OR (e.fired_at, e.id) < ($3, $4))
 ORDER BY e.fired_at DESC, e.id DESC LIMIT $5`
	unreadSQL      = `SELECT count(*)` + feedFrom + ` WHERE k.user_id IS NULL AND (e.state = 'firing' OR e.fired_at > now() - interval '24 hours')`
	firingItemsSQL = `SELECT ` + feedColumns + feedFrom + ` WHERE e.state = 'firing' ORDER BY e.fired_at DESC, e.id DESC`
)

// EventStore implements alerts.EventRepository.
type EventStore struct {
	db *store.DB
}

// NewEventStore builds the store.
func NewEventStore(db *store.DB) *EventStore {
	return &EventStore{db: db}
}

// Firing returns the open firing event of a rule, or nil.
func (s *EventStore) Firing(ctx context.Context, rule alerts.RuleID) (*alerts.AlertEvent, error) {
	e, err := EventScanner{}.Scan(s.db.Q(ctx).QueryRow(ctx, firingSQL, rule))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, s.db.Translate(err)
	}
	return &e, nil
}

// Fire inserts a firing event; false when the rule already fires (unique index alert_events_one_firing).
func (s *EventStore) Fire(ctx context.Context, e alerts.AlertEvent) (alerts.AlertEvent, bool, error) {
	out, err := EventScanner{}.Scan(s.db.Q(ctx).QueryRow(ctx, fireSQL, e.RuleID, e.RouteID, e.Value, e.FiredAt))
	if errors.Is(err, pgx.ErrNoRows) {
		return alerts.AlertEvent{}, false, nil
	}
	if err != nil {
		return alerts.AlertEvent{}, false, s.db.Translate(err)
	}
	return out, true, nil
}

// Resolve closes a firing event and queues its resolve notification.
func (s *EventStore) Resolve(ctx context.Context, id alerts.EventID, at time.Time) (alerts.AlertEvent, error) {
	e, err := EventScanner{}.Scan(s.db.Q(ctx).QueryRow(ctx, resolveSQL, int64(id), at))
	return e, s.db.Translate(err)
}

// Pending returns events with an undelivered notification and attempts left.
func (s *EventStore) Pending(ctx context.Context, maxAttempts int) ([]alerts.AlertEvent, error) {
	rows, err := s.db.Q(ctx).Query(ctx, pendingSQL, maxAttempts)
	if err != nil {
		return nil, s.db.Translate(err)
	}
	defer rows.Close()
	out := []alerts.AlertEvent{}
	for rows.Next() {
		e, err := EventScanner{}.Scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, s.db.Translate(rows.Err())
}

// MarkNotified records a delivered notification.
func (s *EventStore) MarkNotified(ctx context.Context, id alerts.EventID, at time.Time) error {
	_, err := s.db.Q(ctx).Exec(ctx, notifiedSQL, int64(id), at)
	return s.db.Translate(err)
}

// MarkFailed records a failed attempt; giveUp clears the pending flag.
func (s *EventStore) MarkFailed(ctx context.Context, id alerts.EventID, reason string, giveUp bool) error {
	_, err := s.db.Q(ctx).Exec(ctx, failedSQL, int64(id), reason, giveUp)
	return s.db.Translate(err)
}

// Ack records the user's acknowledgement (idempotent).
func (s *EventStore) Ack(ctx context.Context, id alerts.EventID, user uuid.UUID, at time.Time) error {
	var found int
	if err := s.db.Q(ctx).QueryRow(ctx, ackSQL, int64(id), user, at).Scan(&found); err != nil {
		return s.db.Translate(err)
	}
	if found == 0 {
		return alerts.ErrAlertNotFound
	}
	return nil
}

// Feed returns one page of history (newest first) and the user's unread count.
func (s *EventStore) Feed(ctx context.Context, q alerts.FeedQuery) (alerts.FeedPage, error) {
	var state *string
	if q.State != nil {
		v := string(*q.State)
		state = &v
	}
	at, id, err := FeedCursor{}.Decode(q.After)
	if err != nil {
		return alerts.FeedPage{}, err
	}
	items, err := s.items(ctx, feedSQL, q.User, state, at, id, q.Limit+1)
	if err != nil {
		return alerts.FeedPage{}, err
	}
	page := alerts.FeedPage{Items: items}
	if len(items) > q.Limit {
		page.Items = items[:q.Limit]
		page.Next = FeedCursor{}.Encode(page.Items[q.Limit-1])
	}
	err = s.db.Q(ctx).QueryRow(ctx, unreadSQL, q.User).Scan(&page.Unread)
	return page, s.db.Translate(err)
}

// FiringItems returns every firing alert (needs-attention rows).
func (s *EventStore) FiringItems(ctx context.Context) ([]alerts.FeedItem, error) {
	return s.items(ctx, firingItemsSQL, uuid.Nil)
}

func (s *EventStore) items(ctx context.Context, sql string, args ...any) ([]alerts.FeedItem, error) {
	rows, err := s.db.Q(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, s.db.Translate(err)
	}
	defer rows.Close()
	out := []alerts.FeedItem{}
	for rows.Next() {
		it, err := FeedScanner{}.Scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, s.db.Translate(rows.Err())
}

// FeedCursor encodes the (fired_at, id) keyset position.
type FeedCursor struct{}

// Encode returns the cursor after item.
func (FeedCursor) Encode(item alerts.FeedItem) *store.Cursor {
	return &store.Cursor{SortKey: []string{item.FiredAt.UTC().Format(time.RFC3339Nano)}, ID: strconv.FormatInt(int64(item.ID), 10)}
}

// Decode returns nil values for the first page.
func (FeedCursor) Decode(c *store.Cursor) (*time.Time, *int64, error) {
	if c == nil {
		return nil, nil, nil
	}
	if len(c.SortKey) != 1 {
		return nil, nil, store.ErrInvalidCursor
	}
	at, err := time.Parse(time.RFC3339Nano, c.SortKey[0])
	if err != nil {
		return nil, nil, store.ErrInvalidCursor
	}
	id, err := strconv.ParseInt(c.ID, 10, 64)
	if err != nil {
		return nil, nil, store.ErrInvalidCursor
	}
	return &at, &id, nil
}

// EventScanner decodes one eventColumns row.
type EventScanner struct{}

// Scan reads an event.
func (EventScanner) Scan(row pgx.Row) (alerts.AlertEvent, error) {
	var e alerts.AlertEvent
	var id int64
	var state string
	var pending *string
	if err := row.Scan(&id, &e.RuleID, &e.RouteID, &state, &e.Value, &e.FiredAt, &e.ResolvedAt, &pending, &e.NotifyAttempts,
		&e.NotifiedAt, &e.NotifyError); err != nil {
		return alerts.AlertEvent{}, err
	}
	e.ID, e.State = alerts.EventID(id), alerts.AlertState(state)
	if pending != nil {
		p := alerts.AlertState(*pending)
		e.NotifyPending = &p
	}
	return e, nil
}

// FeedScanner decodes one feedColumns row.
type FeedScanner struct{}

// Scan reads a feed item and derives its intent.
func (FeedScanner) Scan(row pgx.Row) (alerts.FeedItem, error) {
	var it alerts.FeedItem
	var id int64
	var kind, state string
	var window int
	if err := row.Scan(&id, &it.RuleID, &it.RuleName, &it.RouteID, &it.RouteName, &it.TeamID, &kind, &it.Threshold, &window, &state,
		&it.Value, &it.FiredAt, &it.ResolvedAt, &it.Acked); err != nil {
		return alerts.FeedItem{}, err
	}
	it.ID, it.Kind, it.State, it.Window = alerts.EventID(id), alerts.AlertRuleKind(kind), alerts.AlertState(state), alerts.Window(window)
	it.Intent = alerts.Intent{Type: alerts.IntentRouteDetail, RouteID: it.RouteID.String(), Tab: alerts.TabOverview}
	return it, nil
}
