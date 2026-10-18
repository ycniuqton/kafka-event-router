package runner_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
)

// backlogCase describes a source topic whose partitions hold a backlog while at least one stays idle.
type backlogCase struct {
	partitions int32
	loaded     []int32
	perLoaded  int
	deadline   time.Duration
}

// seed creates the cluster and writes the backlog into the loaded partitions only.
func (b backlogCase) seed(t *testing.T) *kfake.Cluster {
	t.Helper()
	c, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(b.partitions, "in", "out"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	cl, err := kgo.NewClient(kgo.SeedBrokers(c.ListenAddrs()...), kgo.DefaultProduceTopic("in"), kgo.RecordPartitioner(kgo.ManualPartitioner()))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	val, _ := json.Marshal(map[string]any{"keep": true})
	recs := make([]*kgo.Record, 0, len(b.loaded)*b.perLoaded)
	for _, p := range b.loaded {
		for i := 0; i < b.perLoaded; i++ {
			recs = append(recs, &kgo.Record{Partition: p, Value: val})
		}
	}
	if err := cl.ProduceSync(context.Background(), recs...).FirstErr(); err != nil {
		t.Fatal(err)
	}
	return c
}

// A backlog larger than the lane high-water mark must drain at processing speed, not at one poll per fetch
// long-poll: pausing a full lane strips its buffered records, and an idle partition must not hold the re-fetch
// of the resumed ones for the whole fetch max wait.
func TestRunnerDrainsBacklogBehindIdlePartition(t *testing.T) {
	b := backlogCase{partitions: 3, loaded: []int32{0, 1}, perLoaded: 15000, deadline: 6 * time.Second}
	c := b.seed(t)
	m := newManager(&haltRecorder{})
	if err := m.Start(context.Background(), compiled(t, c, "dlq")); err != nil {
		t.Fatal(err)
	}
	defer m.StopAll(context.Background())
	want := len(b.loaded) * b.perLoaded
	began := time.Now()
	for countRecords(t, c, "out") < want {
		if time.Since(began) > b.deadline {
			t.Fatalf("backlog of %d drained only %d records in %s", want, countRecords(t, c, "out"), b.deadline)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Logf("drained %d records in %s", want, time.Since(began).Round(time.Millisecond))
}
