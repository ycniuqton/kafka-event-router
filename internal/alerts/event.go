package alerts

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/store"
)

// EventID identifies one alert event (a firing and its resolve).
type EventID int64

// AlertEvent is one row of alert history.
type AlertEvent struct {
	ID             EventID
	RuleID         RuleID
	RouteID        uuid.UUID
	State          AlertState
	Value          *float64
	FiredAt        time.Time
	ResolvedAt     *time.Time
	NotifyPending  *AlertState
	NotifyAttempts int
	NotifiedAt     *time.Time
	NotifyError    string
}

// IntentType mirrors the web ActionIntent the bell resolves.
type IntentType string

const IntentRouteDetail IntentType = "routeDetail"

// IntentTab is the route-detail tab an alert opens.
type IntentTab string

const (
	TabOverview IntentTab = "overview"
	TabSettings IntentTab = "settings"
)

// Intent is the serialisable action of a bell row.
type Intent struct {
	Type    IntentType `json:"type"`
	RouteID string     `json:"routeId"`
	Tab     IntentTab  `json:"tab,omitempty"`
}

// FeedItem is one GET /alerts row.
type FeedItem struct {
	ID         EventID       `json:"id"`
	RuleID     RuleID        `json:"ruleId"`
	RuleName   string        `json:"ruleName"`
	RouteID    uuid.UUID     `json:"routeId"`
	RouteName  string        `json:"routeName"`
	TeamID     uuid.UUID     `json:"-"`
	Kind       AlertRuleKind `json:"kind"`
	Threshold  float64       `json:"threshold"`
	Window     Window        `json:"windowMinutes"`
	State      AlertState    `json:"state"`
	Value      *float64      `json:"value"`
	FiredAt    time.Time     `json:"firedAt"`
	ResolvedAt *time.Time    `json:"resolvedAt"`
	Acked      bool          `json:"acked"`
	Intent     Intent        `json:"intent"`
}

// FeedQuery selects a feed page for one user.
type FeedQuery struct {
	State *AlertState
	User  uuid.UUID
	Limit int
	After *store.Cursor
}

// FeedPage is one feed page plus the caller's unread count.
type FeedPage struct {
	Items  []FeedItem
	Next   *store.Cursor
	Unread int
}

// PendingNotification is an event whose fired/resolved notification is not yet delivered.
type PendingNotification struct {
	Event AlertEvent
	Rule  AlertRule
	Route RouteInfo
}

// RouteInfo is what alerting needs to know about a route.
type RouteInfo struct {
	ID       uuid.UUID
	Name     string
	TeamID   uuid.UUID
	Running  bool
	Archived bool
}

// RuleRepository persists alert rules (alert_rules).
type RuleRepository interface {
	List(ctx context.Context, routeID *uuid.UUID) ([]AlertRule, error)
	Get(ctx context.Context, id RuleID) (AlertRule, error)
	Insert(ctx context.Context, r AlertRule) (AlertRule, error)
	Update(ctx context.Context, r AlertRule, expected store.RowVersion) (AlertRule, error)
	Delete(ctx context.Context, id RuleID) error
	ListEnabled(ctx context.Context) ([]AlertRule, error)
}

// EventRepository persists alert history (alert_events, alert_event_acks).
type EventRepository interface {
	Firing(ctx context.Context, rule RuleID) (*AlertEvent, error)
	Fire(ctx context.Context, e AlertEvent) (AlertEvent, bool, error)
	Resolve(ctx context.Context, id EventID, at time.Time) (AlertEvent, error)
	Pending(ctx context.Context, maxAttempts int) ([]AlertEvent, error)
	MarkNotified(ctx context.Context, id EventID, at time.Time) error
	MarkFailed(ctx context.Context, id EventID, reason string, giveUp bool) error
	Feed(ctx context.Context, q FeedQuery) (FeedPage, error)
	Ack(ctx context.Context, id EventID, user uuid.UUID, at time.Time) error
	FiringItems(ctx context.Context) ([]FeedItem, error)
}

// RouteDirectory reads routes for alerting (Postgres routes ⨝ route_runtime).
type RouteDirectory interface {
	Route(ctx context.Context, id uuid.UUID) (RouteInfo, error)
	Routes(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]RouteInfo, error)
}
