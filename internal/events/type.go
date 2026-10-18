// Package events pushes control-plane changes to browsers over WS /events: a Bus carries PushEvents
// between API instances, sources publish route/deployment/connection/alert changes, the Hub fans them out.
package events

import (
	"context"
	"encoding/json"
	"time"
)

// PushEventType names one push frame type (web PushEventRouter switches on it).
type PushEventType string

const (
	TypeRouteStateChanged PushEventType = "route.state_changed"
	TypeDeploymentStatus  PushEventType = "deployment.status_changed"
	TypeConnectionHealth  PushEventType = "connection.health_changed"
	TypeAlertFired        PushEventType = "alert.fired"
	TypeAlertResolved     PushEventType = "alert.resolved"
	TypeHello             PushEventType = "hello"
	TypeResync            PushEventType = "resync"
)

// PushEvent is one event on the bus. TeamID scopes visibility ("" = workspace-wide) and never reaches clients.
type PushEvent struct {
	Type    PushEventType   `json:"type"`
	At      time.Time       `json:"at"`
	TeamID  string          `json:"teamId,omitempty"`
	Payload json.RawMessage `json:"payload"`
}

// Frame is what a socket receives.
type Frame struct {
	Type    PushEventType   `json:"type"`
	At      time.Time       `json:"at"`
	Payload json.RawMessage `json:"payload"`
}

// Frame strips the routing fields.
func (e PushEvent) Frame() Frame {
	return Frame{Type: e.Type, At: e.At, Payload: e.Payload}
}

// EventFactory builds push events with a marshalled payload.
type EventFactory struct {
	Now func() time.Time
}

// New returns an event of type t for team with payload p.
func (f EventFactory) New(t PushEventType, team string, p any) (PushEvent, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return PushEvent{}, err
	}
	return PushEvent{Type: t, At: f.Now().UTC(), TeamID: team, Payload: body}, nil
}

// Publisher accepts events for delivery.
type Publisher interface {
	Publish(ctx context.Context, e PushEvent) error
}

// Bus carries events between API instances.
type Bus interface {
	Publisher
	Kind() BusKind
	Subscribe(ctx context.Context) (<-chan PushEvent, error)
}

// BusKind selects the Bus implementation (ROUTER_EVENTS_BUS).
type BusKind string

const (
	BusRedis  BusKind = "redis"
	BusMemory BusKind = "memory"
)
