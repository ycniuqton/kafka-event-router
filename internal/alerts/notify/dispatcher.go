package notify

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/eventrouter/router/internal/alerts"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/secrets"
)

// SecretResolver reads channel secrets (the secrets Manager).
type SecretResolver interface {
	Get(ctx context.Context, ref secrets.Ref) ([]byte, error)
}

// DeliveryResult is the outcome label of router_alert_notifications_total.
type DeliveryResult string

const (
	ResultOK       DeliveryResult = "ok"
	ResultError    DeliveryResult = "error"
	ResultDisabled DeliveryResult = "disabled"
)

// DeliveryCounter counts notification attempts per channel and result.
type DeliveryCounter interface {
	Delivered(channel alerts.ChannelType, result DeliveryResult)
}

// ChannelResult is the outcome of one channel (also the POST /alert-rules/{id}/test response row).
type ChannelResult struct {
	Channel alerts.ChannelType `json:"channel"`
	OK      bool               `json:"ok"`
	Error   string             `json:"error,omitempty"`
}

// ErrorJoin separates channel errors in notify_error.
type ErrorJoin string

const ErrorSeparator ErrorJoin = "; "

// ErrSecretMissing is reported for URL channels without a stored secret.
var ErrSecretMissing = errors.New("webhook URL not set")

// LinkBuilder renders the UI link of a route (ROUTER_PUBLIC_URL + /routes/{id}).
type LinkBuilder struct {
	PublicURL string
}

// RoutePath is the UI path prefix of route detail pages.
type RoutePath string

const RouteDetailPath RoutePath = "/routes/"

// Route returns the absolute link, or "" without a public URL.
func (l LinkBuilder) Route(id string) string {
	if l.PublicURL == "" {
		return ""
	}
	return strings.TrimRight(l.PublicURL, "/") + string(RouteDetailPath) + id
}

// NotificationDispatcher sends notifications to a rule's channels and records the outcome on alert events.
type NotificationDispatcher struct {
	Notifiers   *NotifierManager
	Secrets     SecretResolver
	Events      alerts.EventRepository
	Rules       alerts.RuleRepository
	Routes      alerts.RouteDirectory
	Links       LinkBuilder
	Counter     DeliveryCounter
	MaxAttempts int
	Now         func() time.Time
	Log         *slog.Logger
}

// Deliver sends n to every channel and reports each outcome.
func (d *NotificationDispatcher) Deliver(ctx context.Context, n Notification, channels []alerts.ChannelRef) []ChannelResult {
	out := make([]ChannelResult, 0, len(channels))
	for _, ch := range channels {
		err := d.send(ctx, n, ch)
		res := ChannelResult{Channel: ch.Type, OK: err == nil}
		if err != nil {
			res.Error = err.Error()
		}
		d.Counter.Delivered(ch.Type, d.result(err))
		out = append(out, res)
	}
	return out
}

func (d *NotificationDispatcher) send(ctx context.Context, n Notification, ch alerts.ChannelRef) error {
	notifier, err := d.Notifiers.For(ch.Type)
	if err != nil {
		return err
	}
	dest := Destination{Recipients: alerts.EmailList(ch.Target).Addresses()}
	if ch.Type.UsesSecret() {
		if ch.SecretRef == "" {
			return ErrSecretMissing
		}
		url, err := d.Secrets.Get(ctx, secrets.Ref(ch.SecretRef))
		if err != nil {
			return err
		}
		dest.URL = string(url)
	}
	return notifier.Send(ctx, n, dest)
}

func (d *NotificationDispatcher) result(err error) DeliveryResult {
	var disabled ErrChannelDisabled
	switch {
	case err == nil:
		return ResultOK
	case errors.As(err, &disabled):
		return ResultDisabled
	default:
		return ResultError
	}
}

// DispatchPending delivers every queued fired/resolved notification; failures are retried on later ticks.
func (d *NotificationDispatcher) DispatchPending(ctx context.Context) error {
	pending, err := d.Events.Pending(ctx, d.MaxAttempts)
	if err != nil {
		return err
	}
	for _, e := range pending {
		if err := d.dispatchOne(ctx, e); err != nil {
			d.Log.Warn("alerts: notification dispatch failed", string(logging.AttrError), err)
		}
	}
	return nil
}

func (d *NotificationDispatcher) dispatchOne(ctx context.Context, e alerts.AlertEvent) error {
	rule, err := d.Rules.Get(ctx, e.RuleID)
	if err != nil {
		return err
	}
	route, err := d.Routes.Route(ctx, e.RouteID)
	if err != nil {
		return err
	}
	results := d.Deliver(ctx, NotificationFactory{Links: d.Links}.ForEvent(e, rule, route), rule.Channels)
	failures := ResultSummary{}.Failures(results)
	if failures == "" {
		return d.Events.MarkNotified(ctx, e.ID, d.Now())
	}
	giveUp := e.NotifyAttempts+1 >= d.MaxAttempts
	if giveUp {
		d.Log.Error("alerts: giving up notification", string(AttrEventID), int64(e.ID), string(logging.AttrError), failures)
	}
	return d.Events.MarkFailed(ctx, e.ID, failures, giveUp)
}

// LogAttr names dispatcher log attributes.
type LogAttr string

const AttrEventID LogAttr = "alert_event_id"

// ResultSummary condenses channel results.
type ResultSummary struct{}

// Failures returns "type: error; …" for failed channels, or "".
func (ResultSummary) Failures(results []ChannelResult) string {
	var parts []string
	for _, r := range results {
		if !r.OK {
			parts = append(parts, string(r.Channel)+": "+r.Error)
		}
	}
	return strings.Join(parts, string(ErrorSeparator))
}

// NotificationFactory builds notifications from alert events and rules.
type NotificationFactory struct {
	Links LinkBuilder
}

// ForEvent returns the fired or resolved notification of e.
func (f NotificationFactory) ForEvent(e alerts.AlertEvent, rule alerts.AlertRule, route alerts.RouteInfo) Notification {
	n := f.ForRule(rule, route, EventFired)
	if e.State == alerts.StateResolved {
		n.Type = EventResolved
	}
	n.Value, n.FiredAt, n.ResolvedAt = e.Value, e.FiredAt, e.ResolvedAt
	return n
}

// ForRule returns a notification of type t without event data (used by the test button).
func (f NotificationFactory) ForRule(rule alerts.AlertRule, route alerts.RouteInfo, t EventType) Notification {
	return Notification{Type: t, RuleID: rule.ID, RuleName: rule.Name, Kind: rule.Kind, Threshold: rule.Threshold,
		Window: rule.Window, RouteID: route.ID, RouteName: route.Name, Link: f.Links.Route(route.ID.String())}
}

// MetricsCounter adapts alerts.Metrics to DeliveryCounter.
type MetricsCounter struct {
	Metrics *alerts.Metrics
}

// Delivered implements DeliveryCounter.
func (c MetricsCounter) Delivered(channel alerts.ChannelType, result DeliveryResult) {
	c.Metrics.Notified(channel, string(result))
}
