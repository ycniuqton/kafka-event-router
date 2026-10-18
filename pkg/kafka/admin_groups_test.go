package kafka

import (
	"context"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

func TestReplayAdminOperations(t *testing.T) {
	c := fakeCluster(t)
	ctx := testCtx(t)
	spec := ConnectionSpec{BootstrapServers: c.ListenAddrs(), SecurityMode: SecurityPlaintext}
	admin, err := DefaultClientFactoryRegistry().NewClient(ctx, spec, PurposeAdmin)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	producer, err := kgo.NewClient(kgo.SeedBrokers(c.ListenAddrs()...), kgo.RecordPartitioner(kgo.ManualPartitioner()))
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()
	base := time.Now().Add(-time.Hour).Truncate(time.Millisecond)
	produceAt(t, ctx, producer, "orders", 0, base, 4)
	offsets, err := admin.Admin().OffsetsForTimes(ctx, base.Add(2*time.Second), "orders")
	if err != nil || offsets["orders"][0] != 2 || offsets["orders"][1] != 0 {
		t.Fatalf("offsets for times: %v %v", offsets, err)
	}
	group, err := admin.Admin().DescribeGroup(ctx, "replay-g")
	if err != nil || !group.Empty() {
		t.Fatalf("unknown group: %+v %v", group, err)
	}
	if err := admin.Admin().CommitOffsets(ctx, "replay-g", offsets); err != nil {
		t.Fatalf("commit: %v", err)
	}
	consumer, err := kgo.NewClient(kgo.SeedBrokers(c.ListenAddrs()...), kgo.ConsumerGroup("replay-g"), kgo.ConsumeTopics("orders"))
	if err != nil {
		t.Fatal(err)
	}
	defer consumer.Close()
	fetches := consumer.PollRecords(ctx, 10)
	if fetches.NumRecords() == 0 || fetches.Records()[0].Offset != 2 {
		t.Fatalf("consumer must resume at the committed offset: %+v", fetches.Records())
	}
	waitForMember(t, ctx, admin.Admin(), "replay-g")
}

func waitForMember(t *testing.T, ctx context.Context, admin Admin, group string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		info, err := admin.DescribeGroup(ctx, group)
		if err == nil && !info.Empty() && info.State == GroupStable && len(info.Members[0].Assigned["orders"]) == 3 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("group never became stable with an assigned member")
}
