// Package eventshttp serves WS /api/v1/events: one socket per browser tab receiving push frames.
package eventshttp

import (
	"context"
	"net/http"
	"time"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/events"
	"github.com/eventrouter/router/internal/platform/api"
)

// Path names the push endpoint.
type Path string

const PathEvents Path = "/api/v1/events"

// PermEvents is the permission of WS /events (any viewer).
var PermEvents = auth.Permission{Key: "metrics.read", Access: auth.AccessRead}

// HelloPayload is the first frame: the client knows the stream is live.
type HelloPayload struct {
	Visibility events.VisibilityKind `json:"visibility"`
}

// EventsHandler streams hub events to one socket; a client that fell behind gets a resync frame.
type EventsHandler struct {
	Hub        *events.Hub
	Visibility events.VisibilityKind
	Now        func() time.Time
}

// Route implements api.WSHandler.
func (h *EventsHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathEvents), Permission: PermEvents}
}

// Stream implements api.WSHandler.
func (h *EventsHandler) Stream(ctx context.Context, conn *api.WSConn) error {
	client := h.Hub.Register(conn.Principal())
	defer h.Hub.Unregister(client)
	factory := events.EventFactory{Now: h.Now}
	if err := h.send(ctx, conn, factory, events.TypeHello, HelloPayload{Visibility: h.Visibility}); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case e := <-client.Queue():
			if err := h.deliver(ctx, conn, factory, client, e); err != nil {
				return err
			}
		}
	}
}

// deliver writes e, preceded by a resync frame when events were dropped for this client.
func (h *EventsHandler) deliver(ctx context.Context, conn *api.WSConn, f events.EventFactory, c *events.Client, e events.PushEvent) error {
	if c.TakeDropped() {
		if err := h.send(ctx, conn, f, events.TypeResync, struct{}{}); err != nil {
			return err
		}
	}
	return conn.WriteJSON(ctx, e.Frame())
}

func (h *EventsHandler) send(ctx context.Context, conn *api.WSConn, f events.EventFactory, t events.PushEventType, payload any) error {
	e, err := f.New(t, "", payload)
	if err != nil {
		return err
	}
	return conn.WriteJSON(ctx, e.Frame())
}
