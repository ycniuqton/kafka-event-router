package connections_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/connections/connectionstest"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/pkg/kafka"
)

type serviceFixture struct {
	svc     *connections.Service
	repo    *connectionstest.MemRepo
	usage   *connectionstest.MemUsage
	secrets *connectionstest.MemSecrets
	vault   *connections.SecretVault
	team    uuid.UUID
}

func newServiceFixture() serviceFixture {
	f := serviceFixture{repo: connectionstest.NewMemRepo(), usage: &connectionstest.MemUsage{Routes: map[uuid.UUID][]connections.RouteRef{}}, secrets: connectionstest.NewMemSecrets(), team: uuid.New()}
	f.vault = connections.NewSecretVault(f.secrets)
	f.svc = connections.NewService(connections.ServiceDeps{
		Repo: f.repo, Usage: f.usage, Vault: f.vault, Tx: connectionstest.DirectTx{},
		Validator: connections.DefaultValidator(kafka.DefaultSecurityRegistry(), kafka.DefaultPropertyCatalog()),
	})
	return f
}

func TestServiceCreateStoresSecretWriteOnly(t *testing.T) {
	f := newServiceFixture()
	ctx := context.Background()
	d := validDraft()
	d.TeamID = f.team
	c, err := f.svc.Create(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if c.SecretRef == "" || !c.HasSecret(kafka.SecretPassword) || c.Health.Status != connections.HealthUnknown || c.Kind != kafka.ClusterKafka {
		t.Fatalf("created: %+v", c)
	}
	bundle, err := f.vault.Load(ctx, c.SecretRef)
	if err != nil || bundle.Password != "pw" {
		t.Fatalf("vault: %v", err)
	}
	if _, err := f.svc.Create(ctx, d); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("duplicate name: %v", err)
	}
	bad := d
	bad.Secrets = kafka.SecretBundle{}
	if _, err := f.svc.Create(ctx, bad); fieldPaths(t, err)[0] != "/secrets/password" {
		t.Fatalf("missing password: %v", err)
	}
}

func TestServiceUpdateMergesSecretsAndChecksVersion(t *testing.T) {
	f := newServiceFixture()
	ctx := context.Background()
	d := validDraft()
	d.TeamID = f.team
	c, _ := f.svc.Create(ctx, d)
	_ = f.repo.UpdateHealth(ctx, c.ID, connections.Health{Status: connections.HealthHealthy})

	edit := d
	edit.Secrets = kafka.SecretBundle{}
	edit.Description = "orders"
	updated, err := f.svc.Update(ctx, c.ID, c.Version, edit)
	if err != nil {
		t.Fatalf("keep stored password: %v", err)
	}
	if updated.Version != c.Version+1 || updated.Health.Status != connections.HealthHealthy || updated.SecretRef != c.SecretRef {
		t.Fatalf("updated: %+v", updated)
	}
	if _, err := f.svc.Update(ctx, c.ID, c.Version, edit); !errors.Is(err, store.ErrPreconditionFailed) {
		t.Fatalf("stale version: %v", err)
	}
	edit.Secrets = kafka.SecretBundle{Password: "new"}
	edit.BootstrapServers = []string{"kafka-9:9092"}
	rotated, err := f.svc.Update(ctx, c.ID, updated.Version, edit)
	if err != nil || rotated.Health.Status != connections.HealthUnknown {
		t.Fatalf("rotate: %+v %v", rotated, err)
	}
	if bundle, _ := f.vault.Load(ctx, rotated.SecretRef); bundle.Password != "new" {
		t.Fatal("password not replaced")
	}
}

func TestServiceDeleteGuardsUsage(t *testing.T) {
	f := newServiceFixture()
	ctx := context.Background()
	d := validDraft()
	d.TeamID = f.team
	c, _ := f.svc.Create(ctx, d)
	f.usage.Routes[c.ID] = []connections.RouteRef{{ID: uuid.New(), Name: "order-paid"}}
	var inUse *connections.InUseError
	if err := f.svc.Delete(ctx, c.ID); !errors.As(err, &inUse) || inUse.Routes[0].Name != "order-paid" {
		t.Fatalf("in use: %v", err)
	}
	delete(f.usage.Routes, c.ID)
	if err := f.svc.Delete(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if len(f.secrets.Data) != 0 {
		t.Fatal("secret must be dropped with the connection")
	}
	if _, _, err := f.svc.Get(ctx, c.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("get after delete: %v", err)
	}
}
