package routes

import (
	"context"
	"fmt"
	"log/slog"
)

// RolloutNoticeKind names the outcome a rollout notice reports.
type RolloutNoticeKind string

const (
	NoticeAutoRollback RolloutNoticeKind = "deployment.rolled_back"
	NoticePaused       RolloutNoticeKind = "deployment.failed_paused"
)

// RolloutNoticeText formats the bell texts of failed rollouts.
type RolloutNoticeText string

const (
	TextAutoRollback RolloutNoticeText = "Deploy v%d of %s failed, rolled back as v%d (= v%d)"
	TextPaused       RolloutNoticeText = "Deploy v%d of %s failed; the route was paused (no earlier version)"
)

// RolloutNotice tells people that a rollout failed and what the router did about it.
type RolloutNotice struct {
	Kind            RolloutNoticeKind `json:"kind"`
	RouteID         RouteID           `json:"routeId"`
	RouteName       string            `json:"routeName"`
	TeamID          TeamID            `json:"teamId"`
	FailedVersion   int               `json:"failedVersion"`
	NewVersion      int               `json:"newVersion,omitempty"`
	RestoredVersion int               `json:"restoredVersion,omitempty"`
	Reason          string            `json:"reason"`
	Message         string            `json:"message"`
}

// RolloutNotifierKind selects the notifier through ROUTER_ROUTES_ROLLOUT_NOTIFIER.
type RolloutNotifierKind string

const RolloutNotifierLog RolloutNotifierKind = "log"

// RolloutNotifier is the base of rollout notification channels (monitoring plugs the WS /events bus in).
type RolloutNotifier interface {
	Kind() RolloutNotifierKind
	Notify(ctx context.Context, n RolloutNotice) error
}

// LogRolloutNotifier writes notices to the service log (default until the event bus exists).
type LogRolloutNotifier struct {
	Log *slog.Logger
}

func (LogRolloutNotifier) Kind() RolloutNotifierKind { return RolloutNotifierLog }

// LogKey names log attributes of rollout notices.
type LogKey string

const (
	LogKeyRoute  LogKey = "route_id"
	LogKeyNotice LogKey = "notice"
)

func (n LogRolloutNotifier) Notify(_ context.Context, notice RolloutNotice) error {
	n.Log.Warn(notice.Message, string(LogKeyRoute), string(notice.RouteID), string(LogKeyNotice), string(notice.Kind))
	return nil
}

// RolloutNotifierManager sends notices through the configured notifier; failures are logged, never returned.
type RolloutNotifierManager struct {
	active RolloutNotifier
	log    *slog.Logger
}

// NewRolloutNotifierManager registers notifiers (log always) and selects kind.
func NewRolloutNotifierManager(kind RolloutNotifierKind, log *slog.Logger, ns ...RolloutNotifier) (*RolloutNotifierManager, error) {
	all := map[RolloutNotifierKind]RolloutNotifier{RolloutNotifierLog: LogRolloutNotifier{Log: log}}
	for _, n := range ns {
		all[n.Kind()] = n
	}
	active, ok := all[kind]
	if !ok {
		return nil, fmt.Errorf("routes: no rollout notifier %q registered", kind)
	}
	return &RolloutNotifierManager{active: active, log: log}, nil
}

// Send delivers n.
func (m *RolloutNotifierManager) Send(ctx context.Context, n RolloutNotice) {
	if m == nil {
		return
	}
	if err := m.active.Notify(ctx, n); err != nil {
		m.log.Warn("routes: rollout notice not delivered", string(LogKeyRoute), string(n.RouteID))
	}
}

// RolloutNoticeBuilder turns a failure outcome into its notice.
type RolloutNoticeBuilder struct{}

// From describes out for route r (the route after recovery).
func (RolloutNoticeBuilder) From(r Route, out FailureOutcome, reason string) RolloutNotice {
	n := RolloutNotice{RouteID: r.ID, RouteName: r.Name, TeamID: r.TeamID, FailedVersion: out.Failed.VersionNo, Reason: reason}
	if out.Rollback == nil {
		n.Kind = NoticePaused
		n.Message = fmt.Sprintf(string(TextPaused), n.FailedVersion, r.Name)
		return n
	}
	n.Kind, n.NewVersion = NoticeAutoRollback, out.Rollback.Version.No
	if out.Rollback.Version.BasedOnNo != nil {
		n.RestoredVersion = *out.Rollback.Version.BasedOnNo
	}
	n.Message = fmt.Sprintf(string(TextAutoRollback), n.FailedVersion, r.Name, n.NewVersion, n.RestoredVersion)
	return n
}
