package preview_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/platform/controlplane"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/pkg/kafka"
)

// staticSpecs serves one kfake cluster as every connection.
type staticSpecs struct{ spec kafka.ConnectionSpec }

func (s staticSpecs) Spec(context.Context, string) (kafka.ConnectionSpec, error) { return s.spec, nil }

type kafkaFixture struct {
	cluster  *kfake.Cluster
	producer *kgo.Client
	factory  preview.FranzSamplingFactory
	base     time.Time
}

func newKafkaFixture(t *testing.T) kafkaFixture {
	t.Helper()
	c, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(2, "orders"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	prod, err := kgo.NewClient(kgo.SeedBrokers(c.ListenAddrs()...), kgo.RecordPartitioner(kgo.ManualPartitioner()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(prod.Close)
	spec := kafka.ConnectionSpec{BootstrapServers: c.ListenAddrs(), SecurityMode: kafka.SecurityPlaintext}
	fx := kafkaFixture{cluster: c, producer: prod, base: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		factory: preview.FranzSamplingFactory{Specs: staticSpecs{spec: spec}, Clients: kafka.DefaultClientFactoryRegistry()}}
	for i := 0; i < 30; i++ {
		rec := &kgo.Record{Topic: "orders", Partition: int32(i % 2), Value: []byte(`{"id":1}`), Timestamp: fx.base.Add(time.Duration(i) * time.Second)}
		if err := prod.ProduceSync(context.Background(), rec).FirstErr(); err != nil {
			t.Fatal(err)
		}
	}
	return fx
}

func integrationCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestKafkaSamplingIntegrationNoGroupNoCommit(t *testing.T) {
	fx, ctx := newKafkaFixture(t), integrationCtx(t)
	latest := preview.KafkaLatestSource{Sampling: preview.KafkaSampling{Clients: fx.factory}, Planner: kafka.LatestOffsetPlanner{}}
	req := preview.LoadRequest{ConnectionID: "c1", Topics: []string{"orders"}, Count: 10}
	res, err := latest.Load(ctx, req, kafka.NewSamplingBudget(1000, 10<<20, time.Now().Add(10*time.Second)))
	if err != nil || len(res.Records) != 10 || res.StopReason != kafka.StopComplete {
		t.Fatalf("latest: %d %s %v", len(res.Records), res.StopReason, err)
	}
	if last := res.Records[9]; !last.Timestamp.Equal(fx.base.Add(29 * time.Second)) {
		t.Fatalf("newest record = %+v", last)
	}
	ranged := preview.KafkaTimeRangeSource{Sampling: preview.KafkaSampling{Clients: fx.factory}, Planner: kafka.TimeRangeOffsetPlanner{}}
	from, to := fx.base.Add(10*time.Second), fx.base.Add(14*time.Second)
	res, err = ranged.Load(ctx, preview.LoadRequest{ConnectionID: "c1", Topics: []string{"orders"}, From: &from, To: &to}, kafka.NewSamplingBudget(1000, 10<<20, time.Now().Add(10*time.Second)))
	if err != nil || len(res.Records) != 5 {
		t.Fatalf("time range: %d %v", len(res.Records), err)
	}
	groups, err := kadm.NewClient(fx.producer).ListGroups(ctx)
	if err != nil || len(groups) != 0 {
		t.Fatalf("sampling created consumer groups (and so commits): %v %v", groups.Groups(), err)
	}
}

func TestKafkaSamplingIntegrationLimitsTruncate(t *testing.T) {
	fx, ctx := newKafkaFixture(t), integrationCtx(t)
	latest := preview.KafkaLatestSource{Sampling: preview.KafkaSampling{Clients: fx.factory}, Planner: kafka.LatestOffsetPlanner{}}
	req := preview.LoadRequest{ConnectionID: "c1", Topics: []string{"orders"}, Count: 30}
	res, err := latest.Load(ctx, req, kafka.NewSamplingBudget(7, 10<<20, time.Time{}))
	if err != nil || len(res.Records) != 7 || res.StopReason != kafka.StopMaxEvents || !res.StopReason.Truncated() {
		t.Fatalf("event cap: %d %s %v", len(res.Records), res.StopReason, err)
	}
	res, err = latest.Load(ctx, req, kafka.NewSamplingBudget(1000, 40, time.Time{}))
	if err != nil || len(res.Records) != 5 || res.StopReason != kafka.StopMaxBytes {
		t.Fatalf("byte cap: %d %s %v", len(res.Records), res.StopReason, err)
	}
	_, err = latest.Load(ctx, preview.LoadRequest{ConnectionID: "c1", Topics: []string{"nope"}}, kafka.NewSamplingBudget(10, 1<<20, time.Time{}))
	if !errors.Is(err, preview.ErrTopicNotFound) {
		t.Fatalf("missing topic: %v", err)
	}
}

func TestKafkaSamplingIntegrationRedisSessionTTL(t *testing.T) {
	fx, ctx := newKafkaFixture(t), integrationCtx(t)
	latest := preview.KafkaLatestSource{Sampling: preview.KafkaSampling{Clients: fx.factory}, Planner: kafka.LatestOffsetPlanner{}}
	res, err := latest.Load(ctx, preview.LoadRequest{ConnectionID: "c1", Topics: []string{"orders"}, Count: 4}, kafka.NewSamplingBudget(1000, 10<<20, time.Time{}))
	if err != nil {
		t.Fatal(err)
	}
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	sessions := preview.NewSessionStoreManager(preview.NewRedisSessionStore(client), time.Now)
	s, err := sessions.Create(ctx, preview.Session{ID: "ps_int", OwnerID: "u1", Topics: []string{"orders"}}, res.Records)
	if err != nil {
		t.Fatal(err)
	}
	if recs, err := sessions.All(ctx, s.ID); err != nil || len(recs) != 4 {
		t.Fatalf("stored records: %d %v", len(recs), err)
	}
	mr.FastForward(preview.SessionTTL.Duration() + time.Minute)
	if _, err := sessions.Owned(ctx, s.ID, preview.Caller{UserID: "u1"}); !errors.Is(err, preview.ErrSessionExpired) && !errors.Is(err, preview.ErrSessionNotFound) {
		t.Fatalf("session must expire after the TTL: %v", err)
	}
}

func TestControlPlaneSpecs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/connections/c1/spec" || r.Header.Get("Authorization") != "Bearer tok" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(kafka.ConnectionSpec{ID: "c1", BootstrapServers: []string{"b:9092"}, Secret: kafka.SecretBundle{Password: "p"}})
	}))
	t.Cleanup(srv.Close)
	specs := preview.ControlPlaneSpecs{Fetcher: controlplane.NewClientWith(srv.URL, srv.Client(), controlplane.StaticCredential("tok"))}
	spec, err := specs.Spec(context.Background(), "c1")
	if err != nil || spec.BootstrapServers[0] != "b:9092" || spec.Secret.Password != "p" {
		t.Fatalf("spec: %+v %v", spec, err)
	}
	_, err = specs.Spec(context.Background(), "gone")
	var pe *problem.Error
	if !errors.As(err, &pe) || pe.Kind != problem.KindNotFound || pe.Path != "/connectionId" {
		t.Fatalf("unknown connection: %v", err)
	}
	if _, err := (preview.UnreachableSpecs{Err: errors.New("no token")}).Spec(context.Background(), "c1"); !errors.Is(err, preview.ErrSourceUnavailable) {
		t.Fatalf("unreachable: %v", err)
	}
}
