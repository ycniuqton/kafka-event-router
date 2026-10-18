package kafka

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

func demoFactory(t *testing.T) *DemoClientFactory {
	t.Helper()
	s := DefaultDemoSettings()
	s.Interval = 20 * time.Millisecond
	pool := NewDemoClusterPool(s)
	t.Cleanup(pool.Close)
	r := NewClientFactoryRegistry()
	f := NewDemoClientFactory(pool, DefaultPurposeDefaults())
	r.Register(f)
	return f
}

func TestDemoClusterSeedsOrderEvents(t *testing.T) {
	ctx := testCtx(t)
	f := demoFactory(t)
	spec := ConnectionSpec{ID: "demo-1", Kind: ClusterDemo, BootstrapServers: []string{"demo-cluster:9092"}}
	cl, err := f.NewClient(ctx, spec, PurposeAdmin)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	if err := (SeedDialer{}).Reach(ctx, cl.Seeds()); err != nil {
		t.Fatalf("seeds %v: %v", cl.Seeds(), err)
	}
	topics, err := cl.Admin().DescribeTopics(ctx, string(DemoOrderEvents))
	if err != nil || len(topics) != 1 || topics[0].Partitions != int(DemoPartitions) {
		t.Fatalf("topics %+v %v", topics, err)
	}
	first, _ := cl.Admin().EndOffsets(ctx, string(DemoOrderEvents))
	if first[string(DemoOrderEvents)] < int64(DemoBacklog) {
		t.Fatalf("backlog %v", first)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		now, _ := cl.Admin().EndOffsets(ctx, string(DemoOrderEvents))
		if now[string(DemoOrderEvents)] > first[string(DemoOrderEvents)] {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("live feed produced nothing")
}

func TestDemoPeekReturnsOrderEvents(t *testing.T) {
	ctx := testCtx(t)
	f := demoFactory(t)
	cl, err := f.NewClient(ctx, ConnectionSpec{ID: "demo-2", Kind: ClusterDemo}, PurposeSampler)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	marks, err := NewFranzMarksReader(cl.Raw()).Marks(ctx, []string{string(DemoOrderEvents)}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	windows := LatestOffsetPlanner{}.Plan(SamplingPlan{Count: 20}, marks)
	records, _, err := NewFranzPartitionSampler(cl.Raw()).Read(ctx, windows, NewSamplingBudget(20, 10<<20, time.Now().Add(5*time.Second)))
	if err != nil || len(records) != 20 {
		t.Fatalf("peek %d %v", len(records), err)
	}
	var ev DemoOrderEvent
	if err := json.Unmarshal(records[0].Value, &ev); err != nil || ev.OrderID == "" || len(ev.Items) == 0 || ev.Amount <= 0 {
		t.Fatalf("event %s %v", records[0].Value, err)
	}
	if string(records[0].Key) != ev.OrderID {
		t.Fatalf("key %q != order id %q", records[0].Key, ev.OrderID)
	}
}

func TestDemoPoolSharesClusterPerKey(t *testing.T) {
	ctx := testCtx(t)
	f := demoFactory(t)
	a, err := f.Pool().Acquire(ctx, "x")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := f.Pool().Acquire(ctx, "x")
	c, _ := f.Pool().Acquire(ctx, DemoClusterKeyOf(ConnectionSpec{}))
	if a != b || a == c || c != mustAcquire(t, f, DemoDraftKey) {
		t.Fatal("pool must return one cluster per key")
	}
	f.Pool().Release("x")
	d, _ := f.Pool().Acquire(ctx, "x")
	if d == a {
		t.Fatal("released cluster must be replaced")
	}
}

func mustAcquire(t *testing.T, f *DemoClientFactory, key DemoClusterKey) *DemoCluster {
	t.Helper()
	c, err := f.Pool().Acquire(testCtx(t), key)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestDemoOrderGeneratorIsDeterministic(t *testing.T) {
	at := time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC)
	a, b := NewDemoOrderGenerator(1, 5).Event(7, at), NewDemoOrderGenerator(1, 0).Event(7, at)
	if a.OrderID != b.OrderID || a.Amount != b.Amount || a.Email != b.Email || a.OrderID != "ord_000008" {
		t.Fatalf("%+v != %+v", a, b)
	}
	rec := NewDemoOrderGenerator(1, 0).Next(at)
	if rec.Topic != string(DemoOrderEvents) || len(rec.Headers) != 2 || !rec.Timestamp.Equal(at) {
		t.Fatalf("record %+v", rec)
	}
}

func TestDemoOpenAddsExtraOptions(t *testing.T) {
	ctx := testCtx(t)
	f := demoFactory(t)
	cl, err := f.Open(ctx, ConnectionSpec{ID: "demo-3", Kind: ClusterDemo}, PurposeConsumer,
		kgo.ConsumerGroup("g1"), kgo.ConsumeTopics(string(DemoOrderEvents)), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	fetches := cl.PollRecords(ctx, 5)
	if err := fetches.Err(); err != nil || fetches.NumRecords() == 0 {
		t.Fatalf("group consume: %d %v", fetches.NumRecords(), err)
	}
}

// TestDemoProducerCreatesMissingTopics: each process runs its own demo cluster, so a route's destination
// and DLQ topics created by the API do not exist on the worker's cluster; producing must create them.
func TestDemoProducerCreatesMissingTopics(t *testing.T) {
	f := demoFactory(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spec := ConnectionSpec{ID: "demo-autocreate", Kind: ClusterDemo}
	cl, err := f.Open(ctx, spec, PurposeProducer)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	if err := cl.ProduceSync(ctx, &kgo.Record{Topic: "order-totals", Value: []byte(`{}`)}).FirstErr(); err != nil {
		t.Fatalf("produce to a missing topic: %v", err)
	}
}
