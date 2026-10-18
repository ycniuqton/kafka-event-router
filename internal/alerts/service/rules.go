// Package service holds the alert rule and alert feed use cases behind /alert-rules and /alerts.
package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/alerts"
	"github.com/eventrouter/router/internal/alerts/notify"
	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/store"
)

// RuleAccess decides whether the caller in ctx may change alert rules of a team (alert.write).
type RuleAccess interface {
	CanWriteAlerts(ctx context.Context, team uuid.UUID) bool
}

// TxRunner runs fn in one transaction.
type TxRunner interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Deliverer sends one notification to channels (notify.NotificationDispatcher).
type Deliverer interface {
	Deliver(ctx context.Context, n notify.Notification, channels []alerts.ChannelRef) []notify.ChannelResult
}

// RuleService implements alert rule CRUD and the test button.
type RuleService struct {
	Rules    alerts.RuleRepository
	Routes   alerts.RouteDirectory
	Secrets  ChannelSecretStore
	Access   RuleAccess
	Tx       TxRunner
	Deliver  Deliverer
	Links    notify.LinkBuilder
	Recorder AuditRecorder
}

// AuditRecorder writes rule changes to the request's audit scope.
type AuditRecorder struct{}

// Record writes c when the request has an audit scope.
func (AuditRecorder) Record(ctx context.Context, c audit.Change) error {
	if scope, ok := audit.FromContext(ctx); ok {
		return scope.Record(ctx, c)
	}
	return nil
}

// List returns the rules of a route (all rules when routeID is nil).
func (s *RuleService) List(ctx context.Context, routeID *uuid.UUID) ([]alerts.AlertRule, error) {
	return s.Rules.List(ctx, routeID)
}

// Get returns one rule.
func (s *RuleService) Get(ctx context.Context, id alerts.RuleID) (alerts.AlertRule, error) {
	return s.Rules.Get(ctx, id)
}

// Create validates the draft, stores channel secrets and inserts the rule.
func (s *RuleService) Create(ctx context.Context, actor *uuid.UUID, d alerts.RuleDraft) (alerts.AlertRule, error) {
	if err := (alerts.RuleValidator{}).Validate(d); err != nil {
		return alerts.AlertRule{}, err
	}
	route, err := s.writableRoute(ctx, d.RouteID)
	if err != nil {
		return alerts.AlertRule{}, err
	}
	res, err := s.Secrets.Resolve(ctx, d.Channels, nil)
	if err != nil {
		return alerts.AlertRule{}, err
	}
	rule := RuleBuilder{}.From(d, res.Channels)
	rule.CreatedBy = actor
	var out alerts.AlertRule
	err = s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if out, err = s.Rules.Insert(ctx, rule); err != nil {
			return err
		}
		return s.audit(ctx, route, out.ID, nil, out.View())
	})
	return out, err
}

// Patch applies a partial update when the row version matches.
func (s *RuleService) Patch(ctx context.Context, id alerts.RuleID, expected store.RowVersion, p alerts.RulePatch) (alerts.AlertRule, error) {
	before, err := s.Rules.Get(ctx, id)
	if err != nil {
		return alerts.AlertRule{}, err
	}
	route, err := s.writableRoute(ctx, before.RouteID)
	if err != nil {
		return alerts.AlertRule{}, err
	}
	d := p.Apply(before)
	d.Channels = s.Secrets.Inputs(before.Channels)
	if p.Channels != nil {
		d.Channels = *p.Channels
	}
	if err := (alerts.RuleValidator{}).Validate(d); err != nil {
		return alerts.AlertRule{}, err
	}
	res, err := s.Secrets.Resolve(ctx, d.Channels, before.Channels)
	if err != nil {
		return alerts.AlertRule{}, err
	}
	next := RuleBuilder{}.From(d, res.Channels)
	next.ID = id
	var out alerts.AlertRule
	err = s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if out, err = s.Rules.Update(ctx, next, expected); err != nil {
			return err
		}
		return s.audit(ctx, route, id, before.View(), out.View())
	})
	if err == nil {
		s.Secrets.Forget(ctx, res.Orphaned)
	}
	return out, err
}

// Delete removes a rule, its history and its channel secrets.
func (s *RuleService) Delete(ctx context.Context, id alerts.RuleID) error {
	before, err := s.Rules.Get(ctx, id)
	if err != nil {
		return err
	}
	route, err := s.accessibleRoute(ctx, before.RouteID)
	if err != nil {
		return err
	}
	err = s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.Rules.Delete(ctx, id); err != nil {
			return err
		}
		return s.audit(ctx, route, id, before.View(), nil)
	})
	if err == nil {
		s.Secrets.Forget(ctx, ChannelRefs(before.Channels).Secrets())
	}
	return err
}

// Test sends a test notification to every channel of the rule.
func (s *RuleService) Test(ctx context.Context, id alerts.RuleID) ([]notify.ChannelResult, error) {
	rule, err := s.Rules.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	route, err := s.writableRoute(ctx, rule.RouteID)
	if err != nil {
		return nil, err
	}
	n := notify.NotificationFactory{Links: s.Links}.ForRule(rule, route, notify.EventTest)
	n.FiredAt = time.Now().UTC()
	return s.Deliver.Deliver(ctx, n, rule.Channels), nil
}

// writableRoute loads the rule's route and checks it is live (not archived) and writable by the caller.
func (s *RuleService) writableRoute(ctx context.Context, id uuid.UUID) (alerts.RouteInfo, error) {
	route, err := s.accessibleRoute(ctx, id)
	if err != nil {
		return alerts.RouteInfo{}, err
	}
	if route.Archived {
		return alerts.RouteInfo{}, alerts.ErrRouteArchived.At(string(alerts.PathRouteID))
	}
	return route, nil
}

// accessibleRoute loads the rule's route, archived or not, and checks the caller may change its alert rules.
// Removing a rule only needs this: rules of an archived route must stay deletable.
func (s *RuleService) accessibleRoute(ctx context.Context, id uuid.UUID) (alerts.RouteInfo, error) {
	route, err := s.Routes.Route(ctx, id)
	if errors.Is(err, alerts.ErrRouteUnknown) {
		return alerts.RouteInfo{}, alerts.ErrRouteUnknown.At(string(alerts.PathRouteID))
	}
	if err != nil {
		return alerts.RouteInfo{}, err
	}
	if !s.Access.CanWriteAlerts(ctx, route.TeamID) {
		return alerts.RouteInfo{}, alerts.ErrWriteForbidden
	}
	return route, nil
}

func (s *RuleService) audit(ctx context.Context, route alerts.RouteInfo, id alerts.RuleID, before, after any) error {
	team := route.TeamID
	return s.Recorder.Record(ctx, audit.Change{Action: audit.ActionAlertRuleSaved, Object: audit.ObjectAlertRule,
		ObjectID: id.String(), TeamID: &team, Before: before, After: after})
}

// RuleBuilder assembles a rule from a validated draft.
type RuleBuilder struct{}

// From returns the rule (ID and audit fields unset); a missing enabled flag means enabled.
func (RuleBuilder) From(d alerts.RuleDraft, channels []alerts.ChannelRef) alerts.AlertRule {
	enabled := d.Enabled == nil || *d.Enabled
	return alerts.AlertRule{RouteID: d.RouteID, Name: d.Name, Kind: d.Kind, Threshold: d.Threshold,
		Window: alerts.Window(d.WindowMinutes), Channels: channels, Enabled: enabled}
}

// ChannelRefs is a rule's channel list.
type ChannelRefs []alerts.ChannelRef

// Secrets returns the secret refs in use.
func (c ChannelRefs) Secrets() []string {
	var out []string
	for _, r := range c {
		if r.SecretRef != "" {
			out = append(out, r.SecretRef)
		}
	}
	return out
}
