package dlq

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/dlq/record"
)

const testDLQTopic = "router.dlq.orders"

var baseTime = time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)

func newCluster(t *testing.T, topics ...string) (*kfake.Cluster, SeedClientOpener) {
	t.Helper()
	c, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(2, topics...))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c, SeedClientOpener{Seeds: c.ListenAddrs()}
}

func failure(i int, et record.ErrorType) record.Record {
	return record.Record{
		Key: []byte(fmt.Sprintf("k%d", i)), Value: []byte(fmt.Sprintf(`{"n":%d}`, i)),
		Failure: record.Failure{RouteID: "r1", VersionNo: 1, Branch: "main", Step: "s", ErrorType: et, Message: "m", FailedAt: baseTime.Add(time.Duration(i) * time.Minute)},
		Source:  record.SourcePosition{Topic: "orders", Partition: 0, Offset: int64(i)},
	}
}

func produceDLQ(t *testing.T, opener SeedClientOpener, n int, partitionOf func(int) int32, typeOf func(int) record.ErrorType) {
	t.Helper()
	cl, err := opener.Open(context.Background(), "c1", "", kgo.RecordPartitioner(kgo.ManualPartitioner()))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	for i := 0; i < n; i++ {
		raw := record.Codec{}.Encode(failure(i, typeOf(i)))
		hs := []kgo.RecordHeader{}
		for _, h := range raw.Headers {
			hs = append(hs, kgo.RecordHeader{Key: h.Key, Value: h.Value})
		}
		rec := &kgo.Record{Topic: testDLQTopic, Partition: partitionOf(i), Key: raw.Key, Value: raw.Value, Headers: hs, Timestamp: failure(i, "").Failure.FailedAt}
		if err := cl.ProduceSync(context.Background(), rec).FirstErr(); err != nil {
			t.Fatal(err)
		}
	}
}

func scanAll(t *testing.T, reader KafkaReader, q Query, filter RecordFilter, want int) ([]record.Record, int) {
	t.Helper()
	targets := []Target{{ConnectionID: "c1", Topics: []string{testDLQTopic}}}
	cur, out, pages := NewCursor(), []record.Record{}, 0
	for {
		page, err := reader.Scan(context.Background(), targets, ScanRequest{Query: q, Cursor: cur, Want: want, Filter: filter, Budget: NewScanBudget(time.Now)})
		if err != nil {
			t.Fatal(err)
		}
		out, pages = append(out, page.Records...), pages+1
		if page.Next.Done || pages > 50 {
			return out, pages
		}
		cur = page.Next
	}
}

func TestKafkaReaderScanPagesNewestFirst(t *testing.T) {
	_, opener := newCluster(t, testDLQTopic)
	produceDLQ(t, opener, 25, func(i int) int32 { return int32(i % 2) }, func(i int) record.ErrorType {
		if i%3 == 0 {
			return record.ErrorTimeout
		}
		return record.ErrorStep
	})
	reader := KafkaReader{Source: FranzSource{Clients: opener}}
	got, pages := scanAll(t, reader, Query{}, KeepAll{}, 7)
	if len(got) != 25 || pages < 4 {
		t.Fatalf("got %d records in %d pages", len(got), pages)
	}
	for i := 1; i < len(got); i++ {
		if got[i].Failure.FailedAt.After(got[i-1].Failure.FailedAt) {
			t.Fatalf("not newest first at %d", i)
		}
	}
	timeouts, _ := scanAll(t, reader, Query{ErrorTypes: []record.ErrorType{record.ErrorTimeout}}, KeepAll{}, 50)
	if len(timeouts) != 9 {
		t.Fatalf("timeouts %d", len(timeouts))
	}
	ranged, _ := scanAll(t, reader, Query{Since: baseTime.Add(10 * time.Minute), Until: baseTime.Add(14 * time.Minute)}, KeepAll{}, 50)
	if len(ranged) != 5 {
		t.Fatalf("time range %d", len(ranged))
	}
}

func TestKafkaReaderFetchAndMissingTopic(t *testing.T) {
	_, opener := newCluster(t, testDLQTopic)
	produceDLQ(t, opener, 5, func(int) int32 { return 0 }, func(int) record.ErrorType { return record.ErrorStep })
	reader := KafkaReader{Source: FranzSource{Clients: opener}}
	targets := []Target{{ConnectionID: "c1", Topics: []string{testDLQTopic, "nope"}}}
	refs := []Ref{{Topic: testDLQTopic, Partition: 0, Offset: 3}, {Topic: testDLQTopic, Partition: 0, Offset: 1}, {Topic: testDLQTopic, Partition: 0, Offset: 99}}
	res, err := reader.Fetch(context.Background(), targets, refs)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Records) != 2 || string(res.Records[refs[0]].Key) != "k3" || len(res.Missing) != 1 {
		t.Fatalf("fetch %+v", res)
	}
	set, err := reader.Ranges(context.Background(), targets)
	if err != nil || len(set.Missing) != 1 || set.Missing[0] != "nope" {
		t.Fatalf("ranges %+v %v", set, err)
	}
}

func TestCompactedTopicStoreCrossInstance(t *testing.T) {
	_, opener := newCluster(t, testDLQTopic)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	a, b := NewCompactedTopicStore(ctx, opener, log), NewCompactedTopicStore(ctx, opener, log)
	ref := Ref{Topic: testDLQTopic, Partition: 1, Offset: 4}
	if err := a.Mark(ctx, "c1", []Resolution{{Ref: ref, RouteID: "r1", State: StateDiscarded, At: baseTime}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.Lookup(ctx, "c1", "r1", []Ref{ref}); got[ref].State != StateDiscarded {
		t.Fatal("read your writes")
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		got, err := b.Lookup(ctx, "c1", "r1", []Ref{ref})
		if err == nil && got[ref].State == StateDiscarded {
			n, _ := b.CountResolved(ctx, "c1", "r1", []PartitionRange{{Topic: testDLQTopic, Partition: 1, Low: 0, High: 10}})
			if n != 1 {
				t.Fatalf("count %d", n)
			}
			if err := b.Clear(ctx, "c1", "r1", []Ref{ref}); err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("second instance never saw the resolution")
}
