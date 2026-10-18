// Package engine evaluates alert rules on the scheduler leader every tick and keeps alert state:
// one firing event per rule, resolve only after a full clean window, notifications queued on each transition.
package engine

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/alerts"
	"github.com/eventrouter/router/internal/events"
)

// TxRunner runs fn in one transaction (store.PgTxManager).
type TxRunner interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Transition is what one evaluation did to a rule's state.
type Transition string

const (
	TransitionNone     Transition = "none"
	TransitionFired    Transition = "fired"
	TransitionResolved Transition = "resolved"
)

// AlertPayload is the payload of alert.fired / alert.resolved push events.
type AlertPayload struct {
	EventID    alerts.EventID       `json:"eventId"`
	RuleID     uuid.UUID            `json:"ruleId"`
	RuleName   string               `json:"ruleName"`
	RouteID    uuid.UUID            `json:"routeId"`
	RouteName  string               `json:"routeName"`
	Kind       alerts.AlertRuleKind `json:"kind"`
	Threshold  float64              `json:"threshold"`
	State      alerts.AlertState    `json:"state"`
	Value      *float64             `json:"value"`
	FiredAt    time.Time            `json:"firedAt"`
	ResolvedAt *time.Time           `json:"resolvedAt"`
}

// Observation is one evaluated rule.
type Observation struct {
	Rule     alerts.AlertRule
	Route    alerts.RouteInfo
	Value    *float64
	Breached bool
}

// StateTracker applies observations to alert history (one tx per rule) and announces transitions.
// A firing alert resolves only after the condition stayed clean for one full rule window (no flapping).
type StateTracker struct {
	Tx      TxRunner
	Events  alerts.EventRepository
	Out     events.Publisher
	Factory events.EventFactory
	Now     func() time.Time

	mu         sync.Mutex
	cleanSince map[alerts.RuleID]time.Time
}

// Apply records the observation and returns the transition it caused.
func (t *StateTracker) Apply(ctx context.Context, o Observation) (Transition, error) {
	var result Transition = TransitionNone
	var event alerts.AlertEvent
	err := t.Tx.WithinTx(ctx, func(ctx context.Context) error {
		firing, err := t.Events.Firing(ctx, o.Rule.ID)
		if err != nil {
			return err
		}
		result, event, err = t.step(ctx, o, firing)
		return err
	})
	if err != nil || result == TransitionNone {
		return result, err
	}
	return result, t.announce(ctx, o, event)
}

// step decides and writes the transition for the current firing event (nil when none).
func (t *StateTracker) step(ctx context.Context, o Observation, firing *alerts.AlertEvent) (Transition, alerts.AlertEvent, error) {
	now := t.Now()
	switch {
	case o.Breached && firing == nil:
		t.clearClean(o.Rule.ID)
		e, inserted, err := t.Events.Fire(ctx, alerts.AlertEvent{RuleID: o.Rule.ID, RouteID: o.Rule.RouteID, Value: o.Value, FiredAt: now})
		if err != nil || !inserted {
			return TransitionNone, alerts.AlertEvent{}, err
		}
		return TransitionFired, e, nil
	case o.Breached:
		t.clearClean(o.Rule.ID)
		return TransitionNone, alerts.AlertEvent{}, nil
	case firing == nil:
		return TransitionNone, alerts.AlertEvent{}, nil
	}
	if !t.cleanLongEnough(o.Rule, now) {
		return TransitionNone, alerts.AlertEvent{}, nil
	}
	t.clearClean(o.Rule.ID)
	e, err := t.Events.Resolve(ctx, firing.ID, now)
	return TransitionResolved, e, err
}

// cleanLongEnough starts or checks the clean streak of a firing rule.
func (t *StateTracker) cleanLongEnough(rule alerts.AlertRule, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cleanSince == nil {
		t.cleanSince = map[alerts.RuleID]time.Time{}
	}
	since, ok := t.cleanSince[rule.ID]
	if !ok {
		t.cleanSince[rule.ID] = now
		return false
	}
	return now.Sub(since) >= rule.Window.Duration()
}

func (t *StateTracker) clearClean(id alerts.RuleID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.cleanSince, id)
}

// announce publishes alert.fired / alert.resolved for the route's team.
func (t *StateTracker) announce(ctx context.Context, o Observation, e alerts.AlertEvent) error {
	typ := events.TypeAlertFired
	if e.State == alerts.StateResolved {
		typ = events.TypeAlertResolved
	}
	ev, err := t.Factory.New(typ, o.Route.TeamID.String(), AlertPayload{EventID: e.ID, RuleID: o.Rule.ID, RuleName: o.Rule.Name,
		RouteID: o.Route.ID, RouteName: o.Route.Name, Kind: o.Rule.Kind, Threshold: o.Rule.Threshold, State: e.State,
		Value: e.Value, FiredAt: e.FiredAt, ResolvedAt: e.ResolvedAt})
	if err != nil {
		return err
	}
	return t.Out.Publish(ctx, ev)
}
