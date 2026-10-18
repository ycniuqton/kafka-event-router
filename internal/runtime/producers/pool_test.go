package producers_test

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/twmb/franz-go/pkg/kfake"

	"github.com/eventrouter/router/internal/runtime/clients"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/producers"
	"github.com/eventrouter/router/internal/runtime/retry"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/kafka"
)

func newPool(t *testing.T, sharing domain.ProducerSharing) (*producers.Pool, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	opener := clients.NewOpenerManager(clients.NewFranzOpener())
	p := producers.NewPool(opener, sharing, retry.DefaultPolicy())
	if err := p.Register(reg); err != nil {
		t.Fatal(err)
	}
	return p, reg
}

func specOn(t *testing.T, id, user, password string) kafka.ConnectionSpec {
	t.Helper()
	c, err := kfake.NewCluster(kfake.NumBrokers(1))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return kafka.ConnectionSpec{ID: id, BootstrapServers: c.ListenAddrs(), SecurityMode: kafka.SecurityPlaintext,
		Auth: kafka.AuthSpec{Username: user}, Secret: kafka.SecretBundle{Password: password}}
}

func TestSharedPoolHandsOneClientToRoutesWithMatchingCredentials(t *testing.T) {
	p, reg := newPool(t, domain.SharingOn)
	ctx := context.Background()
	spec := specOn(t, "c1", "svc", "pw")
	a, err := p.Acquire(ctx, spec, config.DeliveryModeAtLeastOnce, "route-a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.Acquire(ctx, spec, config.DeliveryModeAtLeastOnce, "route-b")
	if err != nil {
		t.Fatal(err)
	}
	if a.Client != b.Client || p.Size() != 1 {
		t.Fatalf("routes on the same connection and credentials must share: size %d", p.Size())
	}
	rotated := spec
	rotated.Secret.Password = "rotated"
	c, err := p.Acquire(ctx, rotated, config.DeliveryModeAtLeastOnce, "route-c")
	if err != nil {
		t.Fatal(err)
	}
	if c.Client == a.Client || p.Size() != 2 {
		t.Fatalf("different credentials must not share: size %d", p.Size())
	}
	if got := clientsGauge(t, reg); got != 2 {
		t.Fatalf("%s = %v", producers.MetricProducerClients, got)
	}
	p.Release(ctx, a)
	if p.Size() != 2 {
		t.Fatal("releasing one of two shared leases must keep the client open")
	}
	p.Release(ctx, b)
	p.Release(ctx, c)
	if p.Size() != 0 {
		t.Fatalf("last release must close the client: size %d", p.Size())
	}
}

func TestUnsharedPoolGivesEveryRouteItsOwnClient(t *testing.T) {
	p, _ := newPool(t, domain.SharingOff)
	ctx := context.Background()
	spec := specOn(t, "c1", "svc", "pw")
	a, _ := p.Acquire(ctx, spec, config.DeliveryModeAtLeastOnce, "route-a")
	b, _ := p.Acquire(ctx, spec, config.DeliveryModeAtLeastOnce, "route-b")
	if a == nil || b == nil || a.Client == b.Client || p.Size() != 2 {
		t.Fatalf("sharing off must isolate routes: size %d", p.Size())
	}
	p.Release(ctx, a)
	p.Release(ctx, b)
}

func TestProducerSharingValues(t *testing.T) {
	for s, want := range map[domain.ProducerSharing]bool{domain.SharingOn: true, domain.SharingOff: true, "maybe": false} {
		if s.Valid() != want {
			t.Errorf("%q valid = %v", s, !want)
		}
	}
}

func clientsGauge(t *testing.T, reg *prometheus.Registry) float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() == string(producers.MetricProducerClients) {
			return f.GetMetric()[0].GetGauge().GetValue()
		}
	}
	t.Fatal("gauge not registered")
	return 0
}
