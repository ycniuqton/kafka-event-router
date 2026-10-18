package kafka

import (
	"context"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

func samplingClient(t *testing.T, ctx context.Context, addrs []string) *FranzSamplingClient {
	t.Helper()
	spec := ConnectionSpec{BootstrapServers: addrs, SecurityMode: SecurityPlaintext}
	cl, err := DefaultClientFactoryRegistry().NewClient(ctx, spec, PurposeSampler)
	if err != nil {
		t.Fatal(err)
	}
	sc := NewFranzSamplingClient(cl)
	sc.sampler = sc.sampler.WithIdleWait(500 * time.Millisecond)
	t.Cleanup(sc.Close)
	return sc
}

func produceAt(t *testing.T, ctx context.Context, cl *kgo.Client, topic string, partition int32, ts time.Time, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		rec := &kgo.Record{Topic: topic, Partition: partition, Value: []byte(`{"n":1}`), Timestamp: ts.Add(time.Duration(i) * time.Second), Headers: []kgo.RecordHeader{{Key: "h", Value: []byte("v")}}}
		if err := cl.ProduceSync(ctx, rec).FirstErr(); err != nil {
			t.Fatal(err)
		}
	}
}

func manualProducer(t *testing.T, addrs []string) *kgo.Client {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(addrs...), kgo.RecordPartitioner(kgo.ManualPartitioner()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cl.Close)
	return cl
}

func TestFranzMarksReader(t *testing.T) {
	c, ctx := fakeCluster(t), testCtx(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	prod := manualProducer(t, c.ListenAddrs())
	produceAt(t, ctx, prod, "orders", 0, base, 5)
	produceAt(t, ctx, prod, "orders", 2, base, 2)
	sc := samplingClient(t, ctx, c.ListenAddrs())

	marks, err := sc.Marks(ctx, []string{"orders", "missing"}, time.Time{})
	if err != nil || len(marks) != 3 {
		t.Fatalf("marks: %+v %v", marks, err)
	}
	if marks[0].High != 5 || marks[1].High != 0 || marks[2].High != 2 || marks[0].FromOffset != int64(OffsetUnresolved) {
		t.Fatalf("watermarks: %+v", marks)
	}
	timed, err := sc.Marks(ctx, []string{"orders"}, base.Add(3*time.Second))
	if err != nil || timed[0].FromOffset != 3 || timed[2].FromOffset != 2 {
		t.Fatalf("after time: %+v %v", timed, err)
	}
}

func TestFranzPartitionSamplerReadsWindowsWithoutGroup(t *testing.T) {
	c, ctx := fakeCluster(t), testCtx(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	prod := manualProducer(t, c.ListenAddrs())
	produceAt(t, ctx, prod, "orders", 0, base, 6)
	produceAt(t, ctx, prod, "orders", 1, base, 3)
	sc := samplingClient(t, ctx, c.ListenAddrs())

	marks, err := sc.Marks(ctx, []string{"orders"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	windows := LatestOffsetPlanner{}.Plan(SamplingPlan{Count: 4}, marks)
	recs, reason, err := sc.Read(ctx, windows, NewSamplingBudget(100, 1<<20, time.Now().Add(5*time.Second)))
	if err != nil || reason != StopComplete || len(recs) != 4 {
		t.Fatalf("read: %d %s %v", len(recs), reason, err)
	}
	if recs[0].Headers[0].Key != "h" || recs[0].Timestamp.IsZero() {
		t.Fatalf("record: %+v", recs[0])
	}
	again, _, err := sc.Read(ctx, windows, NewSamplingBudget(100, 1<<20, time.Now().Add(5*time.Second)))
	if err != nil || len(again) != 4 {
		t.Fatalf("second read on the same client: %d %v", len(again), err)
	}
	groups, err := kadm.NewClient(prod).ListGroups(ctx)
	if err != nil || len(groups) != 0 {
		t.Fatalf("sampling must not create consumer groups: %v %v", groups.Groups(), err)
	}
}

func TestFranzPartitionSamplerBudgetAndTimeRange(t *testing.T) {
	c, ctx := fakeCluster(t), testCtx(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	prod := manualProducer(t, c.ListenAddrs())
	produceAt(t, ctx, prod, "orders", 0, base, 10)
	sc := samplingClient(t, ctx, c.ListenAddrs())
	marks, _ := sc.Marks(ctx, []string{"orders"}, base.Add(2*time.Second))

	all := LatestOffsetPlanner{}.Plan(SamplingPlan{Count: 10}, marks)
	recs, reason, err := sc.Read(ctx, all, NewSamplingBudget(3, 1<<20, time.Time{}))
	if err != nil || reason != StopMaxEvents || len(recs) != 3 {
		t.Fatalf("max events: %d %s %v", len(recs), reason, err)
	}
	ranged := TimeRangeOffsetPlanner{}.Plan(SamplingPlan{From: base.Add(2 * time.Second), To: base.Add(5 * time.Second)}, marks)
	recs, reason, err = sc.Read(ctx, ranged, NewSamplingBudget(100, 1<<20, time.Time{}))
	if err != nil || reason != StopComplete || len(recs) != 4 || recs[0].Offset != 2 {
		t.Fatalf("time range: %d %s %v", len(recs), reason, err)
	}
	recs, reason, err = sc.Read(ctx, []PartitionWindow{}, NewSamplingBudget(1, 1, time.Time{}))
	if err != nil || reason != StopComplete || len(recs) != 0 {
		t.Fatalf("no windows: %d %s %v", len(recs), reason, err)
	}
}

func TestFranzPartitionSamplerCallerCancel(t *testing.T) {
	c := fakeCluster(t)
	ctx, cancel := context.WithCancel(testCtx(t))
	prod := manualProducer(t, c.ListenAddrs())
	produceAt(t, ctx, prod, "orders", 0, time.Now(), 1)
	sc := samplingClient(t, ctx, c.ListenAddrs())
	cancel()
	if _, _, err := sc.Read(ctx, []PartitionWindow{{Topic: "orders", Partition: 0, Start: 0, End: 5}}, NewSamplingBudget(10, 1<<20, time.Time{})); err == nil {
		t.Fatal("a cancelled caller context must fail the read")
	}
}

func TestFranzPartitionSamplerIdleEndsWindowsWithGaps(t *testing.T) {
	c, ctx := fakeCluster(t), testCtx(t)
	prod := manualProducer(t, c.ListenAddrs())
	produceAt(t, ctx, prod, "orders", 0, time.Now(), 3)
	sc := samplingClient(t, ctx, c.ListenAddrs())
	recs, reason, err := sc.Read(ctx, []PartitionWindow{{Topic: "orders", Partition: 0, Start: 0, End: 10}}, NewSamplingBudget(100, 1<<20, time.Now().Add(5*time.Second)))
	if err != nil || reason != StopComplete || len(recs) != 3 {
		t.Fatalf("idle end: %d %s %v", len(recs), reason, err)
	}
}
