package pgstore_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/connections/connectionstest"
	"github.com/eventrouter/router/internal/connections/pgstore"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/internal/testkit"
	"github.com/eventrouter/router/pkg/kafka"
)

func TestIntegrationDemoConnectionLifecycle(t *testing.T) {
	sb := testkit.NewManager().Postgres(t)
	sb.Migrate(t)
	ctx := context.Background()
	var team uuid.UUID
	if err := sb.DB.Pool().QueryRow(ctx, `INSERT INTO teams (name) VALUES ('demo-team') RETURNING id`).Scan(&team); err != nil {
		t.Fatal(err)
	}
	tx := store.NewTxManager(sb.DB)
	repo := pgstore.NewConnectionRepository(sb.DB, tx)
	svc := connections.NewService(connections.ServiceDeps{Repo: repo, Usage: pgstore.NewUsageResolver(sb.DB), Tx: tx,
		Vault:     connections.NewSecretVault(connectionstest.NewMemSecrets()),
		Validator: connections.DefaultValidator(kafka.DefaultSecurityRegistry(), kafka.DefaultPropertyCatalog())})
	c, err := svc.CreateDemo(ctx, team)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := repo.Get(ctx, c.ID)
	if err != nil || stored.Kind != kafka.ClusterDemo || stored.ProbeEnabled || stored.TeamName != "demo-team" {
		t.Fatalf("stored demo: %+v %v", stored, err)
	}
	var exists *connections.DemoExistsError
	if _, err := svc.CreateDemo(ctx, team); !errors.As(err, &exists) {
		t.Fatalf("second create: %v", err)
	}
	probe, err := repo.ListForProbe(ctx, stored.CreatedAt.AddDate(1, 0, 0), 10)
	if err != nil || len(probe) != 0 {
		t.Fatalf("the prober must skip demo connections: %+v %v", probe, err)
	}
	if err := svc.DeleteDemo(ctx, team); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, c.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted: %v", err)
	}
}
