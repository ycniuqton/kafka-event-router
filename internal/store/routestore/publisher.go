package routestore

import (
	"context"

	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/store"
)

// PgNotifyPublisher sends route_changed notifications; callers publish after commit.
type PgNotifyPublisher struct {
	Notifier *store.Notifier
}

// Publish implements routes.RouteEventPublisher.
func (p PgNotifyPublisher) Publish(ctx context.Context, ev routes.RouteChanged) error {
	return p.Notifier.Notify(ctx, store.ChannelRouteChanged, ev)
}
