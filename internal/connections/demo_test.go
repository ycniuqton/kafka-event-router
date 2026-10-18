package connections_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/connections/connectionstest"
	"github.com/eventrouter/router/pkg/kafka"
)

// recordingTeardown remembers the connections released after delete.
type recordingTeardown struct {
	released []uuid.UUID
}

func (r *recordingTeardown) Release(_ context.Context, c connections.Connection) {
	r.released = append(r.released, c.ID)
}

func TestCreateDemoIsIdempotentPerTeam(t *testing.T) {
	f := newServiceFixture()
	teardown := &recordingTeardown{}
	svc := connections.NewService(connections.ServiceDeps{Repo: f.repo, Usage: f.usage, Vault: f.vault, Tx: connectionstest.DirectTx{}, Teardown: teardown,
		Validator: connections.DefaultValidator(kafka.DefaultSecurityRegistry(), kafka.DefaultPropertyCatalog())})
	ctx := context.Background()
	c, err := svc.CreateDemo(ctx, f.team)
	if err != nil {
		t.Fatal(err)
	}
	if c.Kind != kafka.ClusterDemo || c.ProbeEnabled || !c.AllowTopicCreate || c.SecretRef != "" || c.Name != string(connections.DemoName) {
		t.Fatalf("demo connection: %+v", c)
	}
	var exists *connections.DemoExistsError
	if _, err := svc.CreateDemo(ctx, f.team); !errors.As(err, &exists) || exists.Connection.ID != c.ID {
		t.Fatalf("second create must conflict: %v", err)
	}
	if other, err := svc.CreateDemo(ctx, uuid.New()); err != nil || other.ID == c.ID {
		t.Fatalf("another team gets its own demo: %v", err)
	}
	f.usage.Routes[c.ID] = []connections.RouteRef{{ID: uuid.New(), Name: "demo-route"}}
	var inUse *connections.InUseError
	if err := svc.DeleteDemo(ctx, f.team); !errors.As(err, &inUse) {
		t.Fatalf("used demo must not be deleted: %v", err)
	}
	delete(f.usage.Routes, c.ID)
	if err := svc.DeleteDemo(ctx, f.team); err != nil || len(teardown.released) != 1 || teardown.released[0] != c.ID {
		t.Fatalf("delete demo: %v %v", err, teardown.released)
	}
	if _, found, _ := svc.DemoOf(ctx, f.team); found {
		t.Fatal("demo must be gone")
	}
	if err := svc.DeleteDemo(ctx, f.team); err != nil {
		t.Fatalf("deleting a missing demo is a no-op: %v", err)
	}
}

func TestDemoClusterTeardownReleasesPoolCluster(t *testing.T) {
	pool := kafka.NewDemoClusterPool(kafka.DefaultDemoSettings())
	t.Cleanup(pool.Close)
	ctx := context.Background()
	c := connections.Connection{ID: uuid.New(), Kind: kafka.ClusterDemo}
	key := kafka.DemoClusterKeyOf(c.Spec(kafka.SecretBundle{}))
	first, err := pool.Acquire(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	connections.DemoClusterTeardown{Pool: pool}.Release(ctx, c)
	second, err := pool.Acquire(ctx, key)
	if err != nil || second == first {
		t.Fatalf("teardown must stop the cluster: %v", err)
	}
}
