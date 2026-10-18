// Package notify delivers alert notifications: one Notifier per channel type behind the NotifierManager
// (enabled set from config), and the NotificationDispatcher that resolves channel secrets and records results.
package notify

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/alerts"
)

// EventType is the notification event (same names as the WS /events push types).
type EventType string

const (
	EventFired    EventType = "alert.fired"
	EventResolved EventType = "alert.resolved"
	EventTest     EventType = "alert.test"
)

// Notification is what every channel renders.
type Notification struct {
	Type       EventType
	RuleID     uuid.UUID
	RuleName   string
	Kind       alerts.AlertRuleKind
	Threshold  float64
	Window     alerts.Window
	RouteID    uuid.UUID
	RouteName  string
	Value      *float64
	FiredAt    time.Time
	ResolvedAt *time.Time
	Link       string
}

// Destination is a resolved channel target: email recipients or a plaintext URL from the secrets Manager.
type Destination struct {
	Recipients []string
	URL        string
}

// Notifier is the base of every channel implementation.
type Notifier interface {
	Type() alerts.ChannelType
	Send(ctx context.Context, n Notification, d Destination) error
}

// ErrChannelDisabled is reported for channel types not enabled in config.
type ErrChannelDisabled struct {
	Type alerts.ChannelType
}

func (e ErrChannelDisabled) Error() string {
	return fmt.Sprintf("channel %s disabled", e.Type)
}

// NotifierManager is the registry ChannelType → Notifier restricted to the enabled types.
type NotifierManager struct {
	byType  map[alerts.ChannelType]Notifier
	enabled map[alerts.ChannelType]bool
}

// NewNotifierManager registers notifiers and enables the listed types (empty list enables every registered type).
func NewNotifierManager(enabled []alerts.ChannelType, notifiers ...Notifier) *NotifierManager {
	m := &NotifierManager{byType: map[alerts.ChannelType]Notifier{}, enabled: map[alerts.ChannelType]bool{}}
	for _, n := range notifiers {
		m.byType[n.Type()] = n
	}
	if len(enabled) == 0 {
		for t := range m.byType {
			enabled = append(enabled, t)
		}
	}
	for _, t := range enabled {
		m.enabled[t] = true
	}
	return m
}

// For returns the notifier of t, or ErrChannelDisabled.
func (m *NotifierManager) For(t alerts.ChannelType) (Notifier, error) {
	n, ok := m.byType[t]
	if !ok || !m.enabled[t] {
		return nil, ErrChannelDisabled{Type: t}
	}
	return n, nil
}
