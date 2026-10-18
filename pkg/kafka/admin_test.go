package kafka

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
)

func fakeCluster(t *testing.T, opts ...kfake.Opt) *kfake.Cluster {
	t.Helper()
	c, err := kfake.NewCluster(append([]kfake.Opt{kfake.NumBrokers(1), kfake.SeedTopics(3, "orders")}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func testCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestFranzAdminAgainstFakeCluster(t *testing.T) {
	c := fakeCluster(t)
	ctx := testCtx(t)
	spec := ConnectionSpec{BootstrapServers: c.ListenAddrs(), SecurityMode: SecurityPlaintext, Properties: map[PropertyKey]string{PropClientID: "test"}}
	cl, err := DefaultClientFactoryRegistry().NewClient(ctx, spec, PurposeAdmin)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	if err := (SeedDialer{}).Reach(ctx, spec.BootstrapServers); err != nil {
		t.Fatalf("reach: %v", err)
	}
	if err := cl.Admin().Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	created, err := cl.Admin().CreateTopic(ctx, TopicSpec{Name: "payments", Partitions: 2, ReplicationFactor: -1, RetentionMs: 1000})
	if err != nil || created.Name != "payments" {
		t.Fatalf("create: %+v %v", created, err)
	}
	if _, err := cl.Admin().CreateTopic(ctx, TopicSpec{Name: "payments", Partitions: 1, ReplicationFactor: -1}); !errors.Is(err, kerr.TopicAlreadyExists) {
		t.Fatalf("duplicate: %v", err)
	}
	topics, err := cl.Admin().ListTopics(ctx)
	if err != nil || len(topics) != 2 || topics[0].Name != "orders" || topics[0].Partitions != 3 {
		t.Fatalf("list: %+v %v", topics, err)
	}
	produce(t, ctx, cl.Raw(), "orders", 4)
	ends, err := cl.Admin().EndOffsets(ctx, "orders")
	if err != nil || ends["orders"] != 4 {
		t.Fatalf("end offsets: %v %v", ends, err)
	}
}

func produce(t *testing.T, ctx context.Context, cl *kgo.Client, topic string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := cl.ProduceSync(ctx, &kgo.Record{Topic: topic, Value: []byte("x")}).FirstErr(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestScramCredentials(t *testing.T) {
	c := fakeCluster(t, kfake.EnableSASL(), kfake.Superuser("SCRAM-SHA-512", "admin", "secret"))
	ctx := testCtx(t)
	good := ConnectionSpec{BootstrapServers: c.ListenAddrs(), SecurityMode: SecuritySASLScram512, Auth: AuthSpec{Username: "admin"}, Secret: SecretBundle{Password: "secret"}}
	bad := good
	bad.Secret = SecretBundle{Password: "wrong"}
	reg := DefaultClientFactoryRegistry()
	if err := pingWith(ctx, reg, good); err != nil {
		t.Fatalf("good creds: %v", err)
	}
	if err := pingWith(ctx, reg, bad); err == nil {
		t.Fatal("bad credentials must fail (kfake closes the connection instead of returning a SASL code)")
	}
}

func pingWith(ctx context.Context, reg *ClientFactoryRegistry, spec ConnectionSpec) error {
	cl, err := reg.NewClient(ctx, spec, PurposeAdmin)
	if err != nil {
		return err
	}
	defer cl.Close()
	return cl.Admin().Ping(ctx)
}

func TestSeedDialerUnreachable(t *testing.T) {
	err := SeedDialer{Timeout: time.Second}.Reach(testCtx(t), []string{"127.0.0.1:1"})
	if got := DefaultFailureClassifier().Classify(err); got != ReasonUnreachable {
		t.Fatalf("got %s (%v)", got, err)
	}
	if err := (SeedDialer{}).Reach(testCtx(t), nil); !errors.Is(err, ErrNoSeeds) {
		t.Fatal(err)
	}
}

func TestClientFactoryRegistryUnknownKind(t *testing.T) {
	if _, err := NewClientFactoryRegistry().NewClient(context.Background(), ConnectionSpec{Kind: ClusterDemo}, PurposeAdmin); err == nil {
		t.Fatal("an unregistered kind must fail")
	}
	if _, err := DefaultClientFactoryRegistry().For(ClusterDemo); err != nil {
		t.Fatalf("demo kind must be registered by default: %v", err)
	}
}
