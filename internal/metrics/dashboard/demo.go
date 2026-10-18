package dashboard

import (
	"context"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/pkg/kafka"
)

// DemoConnections is connections' demo entry point ("Try with demo data", connections#CN-202).
type DemoConnections interface {
	CreateDemo(ctx context.Context, team uuid.UUID) (connections.Connection, error)
	DemoOf(ctx context.Context, team uuid.UUID) (connections.Connection, bool, error)
	DeleteDemo(ctx context.Context, team uuid.UUID) error
}

// DemoSeed is POST /onboarding/demo and the `demo` field of GET /onboarding.
type DemoSeed struct {
	ConnectionID string `json:"connectionId"`
	Topic        string `json:"topic"`
}

// DemoWorkspaceSeeder creates and removes a team's in-memory demo connection whose generator feeds
// `order-events` (the cluster and generator are owned by connections); a second create is 409.
type DemoWorkspaceSeeder struct {
	Connections DemoConnections
}

// Create seeds the team's demo connection.
func (s DemoWorkspaceSeeder) Create(ctx context.Context, team uuid.UUID) (DemoSeed, error) {
	c, err := s.Connections.CreateDemo(ctx, team)
	if err != nil {
		return DemoSeed{}, err
	}
	return s.seedOf(c), nil
}

// Remove deletes the team's demo connection (none is a no-op; routes still using it block removal).
func (s DemoWorkspaceSeeder) Remove(ctx context.Context, team uuid.UUID) error {
	return s.Connections.DeleteDemo(ctx, team)
}

// Status returns the team's demo seed, or nil when there is none.
func (s DemoWorkspaceSeeder) Status(ctx context.Context, team uuid.UUID) (*DemoSeed, error) {
	c, found, err := s.Connections.DemoOf(ctx, team)
	if err != nil || !found {
		return nil, err
	}
	seed := s.seedOf(c)
	return &seed, nil
}

func (DemoWorkspaceSeeder) seedOf(c connections.Connection) DemoSeed {
	return DemoSeed{ConnectionID: c.ID.String(), Topic: string(kafka.DemoOrderEvents)}
}
