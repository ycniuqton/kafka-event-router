package events

import (
	"context"

	"github.com/eventrouter/router/internal/connections"
)

// ConnectionHealthPublisher implements connections' HealthEventPublisher on the bus (connection.health_changed).
type ConnectionHealthPublisher struct {
	Out     Publisher
	Factory EventFactory
}

// Publish implements connections.HealthEventPublisher.
func (p ConnectionHealthPublisher) Publish(ctx context.Context, e connections.HealthEvent) error {
	ev, err := p.Factory.New(TypeConnectionHealth, e.TeamID, e)
	if err != nil {
		return err
	}
	return p.Out.Publish(ctx, ev)
}
