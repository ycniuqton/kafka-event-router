// Package alertstest provides in-memory fakes of the alerts repositories for unit tests.
package alertstest

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/alerts"
	"github.com/eventrouter/router/internal/store"
)

// Events is an in-memory alerts.EventRepository.
type Events struct {
	mu     sync.Mutex
	Rows   []alerts.AlertEvent
	Acks   map[alerts.EventID]map[uuid.UUID]bool
	nextID alerts.EventID
}

// Firing implements alerts.EventRepository.
func (e *Events) Firing(_ context.Context, rule alerts.RuleID) (*alerts.AlertEvent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.Rows {
		if e.Rows[i].RuleID == rule && e.Rows[i].State == alerts.StateFiring {
			c := e.Rows[i]
			return &c, nil
		}
	}
	return nil, nil
}

// Fire implements alerts.EventRepository (one firing per rule).
func (e *Events) Fire(ctx context.Context, ev alerts.AlertEvent) (alerts.AlertEvent, bool, error) {
	if f, _ := e.Firing(ctx, ev.RuleID); f != nil {
		return alerts.AlertEvent{}, false, nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.nextID++
	pending := alerts.StateFiring
	ev.ID, ev.State, ev.NotifyPending = e.nextID, alerts.StateFiring, &pending
	e.Rows = append(e.Rows, ev)
	return ev, true, nil
}

// Resolve implements alerts.EventRepository.
func (e *Events) Resolve(_ context.Context, id alerts.EventID, at time.Time) (alerts.AlertEvent, error) {
	return e.update(id, func(ev *alerts.AlertEvent) {
		pending := alerts.StateResolved
		ev.State, ev.ResolvedAt, ev.NotifyPending, ev.NotifyAttempts = alerts.StateResolved, &at, &pending, 0
	})
}

// Pending implements alerts.EventRepository.
func (e *Events) Pending(_ context.Context, max int) ([]alerts.AlertEvent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []alerts.AlertEvent
	for _, ev := range e.Rows {
		if ev.NotifyPending != nil && ev.NotifyAttempts < max {
			out = append(out, ev)
		}
	}
	return out, nil
}

// MarkNotified implements alerts.EventRepository.
func (e *Events) MarkNotified(_ context.Context, id alerts.EventID, at time.Time) error {
	_, err := e.update(id, func(ev *alerts.AlertEvent) {
		ev.NotifyPending, ev.NotifiedAt, ev.NotifyError = nil, &at, ""
		ev.NotifyAttempts++
	})
	return err
}

// MarkFailed implements alerts.EventRepository.
func (e *Events) MarkFailed(_ context.Context, id alerts.EventID, reason string, giveUp bool) error {
	_, err := e.update(id, func(ev *alerts.AlertEvent) {
		ev.NotifyAttempts++
		ev.NotifyError = reason
		if giveUp {
			ev.NotifyPending = nil
		}
	})
	return err
}

// Ack implements alerts.EventRepository.
func (e *Events) Ack(_ context.Context, id alerts.EventID, user uuid.UUID, _ time.Time) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.Acks == nil {
		e.Acks = map[alerts.EventID]map[uuid.UUID]bool{}
	}
	if e.Acks[id] == nil {
		e.Acks[id] = map[uuid.UUID]bool{}
	}
	e.Acks[id][user] = true
	return nil
}

// Feed implements alerts.EventRepository (newest first, no paging).
func (e *Events) Feed(_ context.Context, q alerts.FeedQuery) (alerts.FeedPage, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	page := alerts.FeedPage{Items: []alerts.FeedItem{}}
	for _, ev := range e.Rows {
		if q.State == nil || *q.State == ev.State {
			acked := e.Acks[ev.ID][q.User]
			page.Items = append(page.Items, alerts.FeedItem{ID: ev.ID, RuleID: ev.RuleID, RouteID: ev.RouteID, State: ev.State,
				Value: ev.Value, FiredAt: ev.FiredAt, ResolvedAt: ev.ResolvedAt, Acked: acked, Kind: alerts.KindLagAbove,
				Intent: alerts.Intent{Type: alerts.IntentRouteDetail, RouteID: ev.RouteID.String(), Tab: alerts.TabOverview}})
		}
	}
	sort.Slice(page.Items, func(i, j int) bool { return page.Items[i].ID > page.Items[j].ID })
	for _, it := range page.Items {
		if !it.Acked {
			page.Unread++
		}
	}
	return page, nil
}

// FiringItems implements alerts.EventRepository.
func (e *Events) FiringItems(ctx context.Context) ([]alerts.FeedItem, error) {
	state := alerts.StateFiring
	p, err := e.Feed(ctx, alerts.FeedQuery{State: &state})
	return p.Items, err
}

func (e *Events) update(id alerts.EventID, fn func(*alerts.AlertEvent)) (alerts.AlertEvent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.Rows {
		if e.Rows[i].ID == id {
			fn(&e.Rows[i])
			return e.Rows[i], nil
		}
	}
	return alerts.AlertEvent{}, alerts.ErrAlertNotFound
}

// Rules is an in-memory alerts.RuleRepository.
type Rules struct {
	mu   sync.Mutex
	Rows map[alerts.RuleID]alerts.AlertRule
}

// List implements alerts.RuleRepository.
func (r *Rules) List(_ context.Context, routeID *uuid.UUID) ([]alerts.AlertRule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []alerts.AlertRule{}
	for _, x := range r.Rows {
		if routeID == nil || x.RouteID == *routeID {
			out = append(out, x)
		}
	}
	return out, nil
}

// ListEnabled implements alerts.RuleRepository.
func (r *Rules) ListEnabled(ctx context.Context) ([]alerts.AlertRule, error) {
	all, _ := r.List(ctx, nil)
	out := []alerts.AlertRule{}
	for _, x := range all {
		if x.Enabled {
			out = append(out, x)
		}
	}
	return out, nil
}

// Get implements alerts.RuleRepository.
func (r *Rules) Get(_ context.Context, id alerts.RuleID) (alerts.AlertRule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	x, ok := r.Rows[id]
	if !ok {
		return alerts.AlertRule{}, alerts.ErrRuleNotFound
	}
	return x, nil
}

// Insert implements alerts.RuleRepository.
func (r *Rules) Insert(_ context.Context, x alerts.AlertRule) (alerts.AlertRule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Rows == nil {
		r.Rows = map[alerts.RuleID]alerts.AlertRule{}
	}
	x.ID, x.RowVersion = uuid.New(), 1
	r.Rows[x.ID] = x
	return x, nil
}

// Update implements alerts.RuleRepository with row-version checks.
func (r *Rules) Update(_ context.Context, x alerts.AlertRule, expected store.RowVersion) (alerts.AlertRule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, ok := r.Rows[x.ID]
	if !ok {
		return alerts.AlertRule{}, alerts.ErrRuleNotFound
	}
	if cur.RowVersion != expected {
		return alerts.AlertRule{}, &store.PreconditionFailedError{Current: cur.RowVersion}
	}
	x.RowVersion, x.CreatedBy, x.CreatedAt = cur.RowVersion+1, cur.CreatedBy, cur.CreatedAt
	r.Rows[x.ID] = x
	return x, nil
}

// Delete implements alerts.RuleRepository.
func (r *Rules) Delete(_ context.Context, id alerts.RuleID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.Rows[id]; !ok {
		return alerts.ErrRuleNotFound
	}
	delete(r.Rows, id)
	return nil
}

// Routes is an in-memory alerts.RouteDirectory.
type Routes map[uuid.UUID]alerts.RouteInfo

// Route implements alerts.RouteDirectory.
func (r Routes) Route(_ context.Context, id uuid.UUID) (alerts.RouteInfo, error) {
	x, ok := r[id]
	if !ok {
		return alerts.RouteInfo{}, alerts.ErrRouteUnknown
	}
	return x, nil
}

// Routes implements alerts.RouteDirectory.
func (r Routes) Routes(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]alerts.RouteInfo, error) {
	out := map[uuid.UUID]alerts.RouteInfo{}
	for _, id := range ids {
		if x, ok := r[id]; ok {
			out[id] = x
		}
	}
	return out, nil
}

// NoTx runs fn directly.
type NoTx struct{}

// WithinTx implements the TxRunner ports.
func (NoTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// Clock is a settable test clock.
type Clock struct {
	mu  sync.Mutex
	now time.Time
}

// NewClock starts at t.
func NewClock(t time.Time) *Clock {
	return &Clock{now: t}
}

// Now returns the current fake time.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
