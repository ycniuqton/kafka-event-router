package checks_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/connections/checks"
	"github.com/eventrouter/router/internal/connections/connectionstest"
	"github.com/eventrouter/router/pkg/kafka"
)

type env struct {
	repo    *connectionstest.MemRepo
	tester  *connections.Tester
	topics  *connections.TopicBrowser
	creator *connections.TopicCreator
	vault   *connections.SecretVault
}

func newEnv() env {
	repo := connectionstest.NewMemRepo()
	vault := connections.NewSecretVault(connectionstest.NewMemSecrets())
	resolver := connections.NewSpecResolver(repo, vault)
	clients := kafka.DefaultClientFactoryRegistry()
	failures := connections.ClusterFailures{Classifier: kafka.DefaultFailureClassifier(), Hints: connections.DefaultHintCatalog()}
	runner := connections.NewCheckRunner(checks.DefaultRegistry(), failures.Classifier, failures.Hints, nil)
	validator := connections.DefaultValidator(kafka.DefaultSecurityRegistry(), kafka.DefaultPropertyCatalog())
	return env{
		repo: repo, vault: vault,
		tester:  connections.NewTester(connections.TesterDeps{Repo: repo, Resolver: resolver, Vault: vault, Validator: validator, Clients: clients, Runner: runner, Plan: connections.DefaultTestPlan}),
		topics:  connections.NewTopicBrowser(resolver, clients, connectionstest.MemRates{Fixed: map[string]float64{"orders": 2.5}}, failures),
		creator: connections.NewTopicCreator(resolver, clients, failures),
	}
}

func (e env) save(t *testing.T, seeds []string, mode kafka.SecurityMode, user, pass string, allowCreate bool) connections.Connection {
	t.Helper()
	ctx := context.Background()
	ref, err := e.vault.Store(ctx, kafka.SecretBundle{Password: pass})
	if err != nil {
		t.Fatal(err)
	}
	c, err := e.repo.Create(ctx, connections.Connection{
		TeamID: uuid.New(), Name: "c-" + uuid.NewString(), EnvTag: connections.EnvDev, BootstrapServers: seeds, Kind: kafka.ClusterKafka,
		Security: connections.Security{Mode: mode, Auth: kafka.AuthSpec{Username: user}}, SecretRef: ref, AllowTopicCreate: allowCreate,
		Health: connections.Health{Status: connections.HealthUnknown},
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func cluster(t *testing.T, opts ...kfake.Opt) *kfake.Cluster {
	t.Helper()
	c, err := kfake.NewCluster(append([]kfake.Opt{kfake.NumBrokers(1), kfake.SeedTopics(2, "orders", "__consumer_offsets", "router.dlq.r1")}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func (e env) run(t *testing.T, id uuid.UUID, opts connections.TestOptions) (*connections.CollectingSink, connections.Connection) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	p, err := e.tester.PrepareSaved(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	sink := &connections.CollectingSink{}
	if err := e.tester.Execute(ctx, p, opts, sink); err != nil {
		t.Fatal(err)
	}
	c, _ := e.repo.Get(ctx, id)
	return sink, c
}

func TestFullChecklistPasses(t *testing.T) {
	kc := cluster(t)
	produceOne(t, kc.ListenAddrs(), "orders")
	e := newEnv()
	c := e.save(t, kc.ListenAddrs(), kafka.SecurityPlaintext, "", "", false)
	sink, saved := e.run(t, c.ID, connections.TestOptions{ReadTopic: "orders", WriteTopic: "orders"})
	if !sink.Result.OK || saved.Health.Status != connections.HealthHealthy || saved.Health.CheckedAt == nil {
		t.Fatalf("result %+v events %+v", sink.Result, sink.Events)
	}
	if saved.Version != c.Version {
		t.Fatal("health updates must not bump the version")
	}
}

func produceOne(t *testing.T, seeds []string, topic string) {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(seeds...))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	if err := cl.ProduceSync(context.Background(), &kgo.Record{Topic: topic, Value: []byte("{}")}).FirstErr(); err != nil {
		t.Fatal(err)
	}
}

func TestFailuresMapToHealth(t *testing.T) {
	kc := cluster(t, kfake.EnableSASL(), kfake.Superuser("SCRAM-SHA-512", "admin", "secret"))
	e := newEnv()
	cases := []struct {
		name   string
		conn   connections.Connection
		opts   connections.TestOptions
		health connections.HealthStatus
		reason kafka.FailureReason
	}{
		{"stopped broker", e.save(t, []string{"127.0.0.1:1"}, kafka.SecurityPlaintext, "", "", false), connections.TestOptions{}, connections.HealthUnreachable, kafka.ReasonUnreachable},
		{"wrong password", e.save(t, kc.ListenAddrs(), kafka.SecuritySASLScram512, "admin", "nope", false), connections.TestOptions{}, connections.HealthAuthFailed, kafka.ReasonAuthFailed},
		{"missing write topic", e.save(t, kc.ListenAddrs(), kafka.SecuritySASLScram512, "admin", "secret", false), connections.TestOptions{WriteTopic: "nope"}, connections.HealthDegraded, kafka.ReasonTopicNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sink, saved := e.run(t, tc.conn.ID, tc.opts)
			if sink.Result.OK || saved.Health.Status != tc.health || saved.Health.LastFailure.Reason != tc.reason {
				t.Fatalf("health %+v events %+v", saved.Health, sink.Events)
			}
		})
	}
}

func TestTopicBrowserAndCreator(t *testing.T) {
	kc := cluster(t)
	e := newEnv()
	ctx := context.Background()
	c := e.save(t, kc.ListenAddrs(), kafka.SecurityPlaintext, "", "", true)
	topics, err := e.topics.List(ctx, c.ID, connections.TopicQuery{})
	if err != nil || len(topics) != 2 || topics[0].Name != "orders" || *topics[0].RatePerSec != 2.5 || topics[1].Name != "router.dlq.r1" {
		t.Fatalf("topics: %+v %v", topics, err)
	}
	all, _ := e.topics.List(ctx, c.ID, connections.TopicQuery{IncludeInternal: true, Text: "CONSUMER"})
	if len(all) != 1 || !all[0].Internal {
		t.Fatalf("internal: %+v", all)
	}
	created, err := e.creator.Create(ctx, c.ID, connections.TopicRequest{Name: "payments"})
	if err != nil || created.Partitions != 6 {
		t.Fatalf("create: %+v %v", created, err)
	}
	var exists *connections.TopicExistsError
	if _, err := e.creator.Create(ctx, c.ID, connections.TopicRequest{Name: "payments"}); !errors.As(err, &exists) {
		t.Fatalf("duplicate: %v", err)
	}
	locked := e.save(t, kc.ListenAddrs(), kafka.SecurityPlaintext, "", "", false)
	var disabled *connections.TopicCreateDisabledError
	if _, err := e.creator.Create(ctx, locked.ID, connections.TopicRequest{Name: "x"}); !errors.As(err, &disabled) {
		t.Fatalf("disabled: %v", err)
	}
	down := e.save(t, []string{"127.0.0.1:1"}, kafka.SecurityPlaintext, "", "", false)
	var unavailable *connections.ClusterUnavailableError
	short, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if _, err := e.topics.List(short, down.ID, connections.TopicQuery{}); !errors.As(err, &unavailable) || unavailable.Hint == "" {
		t.Fatalf("unavailable: %v", err)
	}
}
