package connections

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/pkg/kafka"
)

// DemoText holds the fixed texts of the demo connection.
type DemoText string

const (
	DemoName        DemoText = "demo-orders"
	DemoDescription DemoText = "In-memory demo cluster with live sample order events on order-events."
	DemoBootstrap   DemoText = "demo-cluster:9092"
	DetailDemoExist DemoText = "The demo connection already exists for this team."
)

// DemoLookupLimit bounds the name search for a team's demo connection.
type DemoLookupLimit int

const DemoLookupPage DemoLookupLimit = 50

// DemoExistsError is returned by CreateDemo when the team already has a demo connection (409).
type DemoExistsError struct {
	Connection Connection
}

func (e *DemoExistsError) Error() string             { return string(DetailDemoExist) }
func (e *DemoExistsError) ProblemKind() problem.Kind { return problem.KindConflict }

// DemoDraft is the fixed shape of a team's demo connection: no secrets, topic creation on, never probed.
type DemoDraft struct{}

// For returns the demo draft of team.
func (DemoDraft) For(team uuid.UUID) Draft {
	return Draft{
		TeamID: team, Name: string(DemoName), Description: string(DemoDescription), EnvTag: EnvDev,
		BootstrapServers: []string{string(DemoBootstrap)}, Security: Security{Mode: kafka.SecurityPlaintext},
		AllowTopicCreate: true, ProbeEnabled: false, Kind: kafka.ClusterDemo,
	}
}

// ClusterTeardown releases runtime resources of a deleted connection (the in-process demo cluster).
type ClusterTeardown interface {
	Release(ctx context.Context, c Connection)
}

// DemoClusterTeardown stops the demo cluster of a deleted connection; other kinds have none, so it is a no-op.
type DemoClusterTeardown struct {
	Pool *kafka.DemoClusterPool
}

// Release implements ClusterTeardown.
func (t DemoClusterTeardown) Release(_ context.Context, c Connection) {
	t.Pool.Release(kafka.DemoClusterKeyOf(c.Spec(kafka.SecretBundle{})))
}

// NoTeardown is the default when no runtime resource is kept per connection.
type NoTeardown struct{}

// Release implements ClusterTeardown.
func (NoTeardown) Release(context.Context, Connection) {}

// CreateDemo creates the team's `kind=demo` connection ("Try with demo data"); a second call yields DemoExistsError.
func (s *Service) CreateDemo(ctx context.Context, team uuid.UUID) (Connection, error) {
	existing, found, err := s.DemoOf(ctx, team)
	if err != nil {
		return Connection{}, err
	}
	if found {
		return Connection{}, &DemoExistsError{Connection: existing}
	}
	c := s.builder.New(DemoDraft{}.For(team), "")
	var out Connection
	err = s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if out, err = s.d.Repo.Create(ctx, c); err != nil {
			return err
		}
		return s.auditor.Record(ctx, ActionCreated, out, nil, ViewOf(out))
	})
	if errors.Is(err, store.ErrConflict) {
		return Connection{}, &DemoExistsError{Connection: c}
	}
	return out, err
}

// DemoOf returns the team's demo connection, if any.
func (s *Service) DemoOf(ctx context.Context, team uuid.UUID) (Connection, bool, error) {
	page, err := s.d.Repo.List(ctx, ListQuery{Text: string(DemoName), Limit: int(DemoLookupPage)})
	if err != nil {
		return Connection{}, false, err
	}
	for _, c := range page.Items {
		if c.TeamID == team && c.Kind == kafka.ClusterDemo {
			return c, true, nil
		}
	}
	return Connection{}, false, nil
}

// DeleteDemo removes the team's demo connection (same usage guard as Delete); none is a no-op.
func (s *Service) DeleteDemo(ctx context.Context, team uuid.UUID) error {
	c, found, err := s.DemoOf(ctx, team)
	if err != nil || !found {
		return err
	}
	return s.Delete(ctx, c.ID)
}
