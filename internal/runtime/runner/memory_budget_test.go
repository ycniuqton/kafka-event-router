package runner

import (
	"slices"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/runtime/membudget"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/kafka"
)

// fakeTopicFetcher records topic pauses.
type fakeTopicFetcher struct {
	consumed []string
	paused   []string
	resumed  []string
}

func (f *fakeTopicFetcher) PauseFetchTopics(topics ...string) []string {
	f.paused = append(f.paused, topics...)
	return f.paused
}
func (f *fakeTopicFetcher) ResumeFetchTopics(topics ...string) {
	f.resumed = append(f.resumed, topics...)
}
func (f *fakeTopicFetcher) GetConsumeTopics() []string { return f.consumed }

func TestSourcePauserPausesConfiguredOrMatchedTopics(t *testing.T) {
	f := &fakeTopicFetcher{consumed: []string{"x"}}
	p := NewSourcePauser(f, config.Source{Topics: []string{"a", "b"}})
	p.PauseFetch()
	p.ResumeFetch()
	p.ResumeFetch()
	if !slices.Equal(f.paused, []string{"a", "b"}) || !slices.Equal(f.resumed, []string{"a", "b"}) {
		t.Fatalf("paused=%v resumed=%v", f.paused, f.resumed)
	}
	pattern := "orders-.*"
	g := &fakeTopicFetcher{consumed: []string{"orders-1"}}
	q := NewSourcePauser(g, config.Source{TopicPattern: &pattern})
	q.PauseFetch()
	q.ResumeFetch()
	if !slices.Equal(g.paused, []string{"orders-1"}) || !slices.Equal(g.resumed, []string{"orders-1"}) {
		t.Fatalf("pattern paused=%v resumed=%v", g.paused, g.resumed)
	}
}

func TestPartitionLaneChargesAndReleasesItsAccount(t *testing.T) {
	acc := &countingAccount{}
	lane := NewPartitionLane(TopicPartition{Topic: "in"}, NewBackpressureGate(nil)).WithAccount(acc)
	lane.Enqueue(laneFixture{}.batch(3, 10))
	rec, _ := lane.next()
	lane.Discard(rec)
	lane.Stop()
	if acc.charged != acc.released || acc.charged == 0 {
		t.Fatalf("charged=%d released=%d, want equal and positive", acc.charged, acc.released)
	}
}

// countingAccount sums charges and releases; detach switches record detaching on.
type countingAccount struct {
	charged, released int64
	detach            bool
}

func (c *countingAccount) Charge(n int64)  { c.charged += n }
func (c *countingAccount) Release(n int64) { c.released += n }
func (c *countingAccount) Detaching() bool { return c.detach }

func TestSourceFetchSizesFromTheBudgetUnlessTheConnectionSetsIt(t *testing.T) {
	cfg := &config.RouteConfig{Source: config.Source{ConnectionID: "c1"}}
	plain := CompiledVersion{Config: cfg, Connections: map[string]kafka.ConnectionSpec{"c1": {ID: "c1"}}}
	if got := (SourceFetch{Budget: membudget.New(256 << 20)}).Options(plain); len(got) != 2 {
		t.Fatalf("want fetch max + partition max options, got %d", len(got))
	}
	set := CompiledVersion{Config: cfg, Connections: map[string]kafka.ConnectionSpec{"c1": {ID: "c1", Properties: map[kafka.PropertyKey]string{kafka.PropFetchMaxBytes: "1048576"}}}}
	if got := (SourceFetch{Budget: membudget.New(256 << 20)}).Options(set); got != nil {
		t.Fatal("an explicit fetch.max.bytes on the connection must win")
	}
}

func TestLaneDetachesRecordsOnlyWhenTheAccountAsks(t *testing.T) {
	resp := []byte("key-value")
	zeroCopy := &countingAccount{}
	lane := NewPartitionLane(TopicPartition{Topic: "in"}, NewBackpressureGate(nil)).WithAccount(zeroCopy)
	rec := &kgo.Record{Key: resp[:3], Value: resp[4:]}
	lane.Enqueue([]*kgo.Record{rec})
	if &rec.Value[0] != &resp[4] {
		t.Fatal("without budget pressure records keep franz-go's zero-copy slices")
	}
	detaching := &countingAccount{detach: true}
	lane = NewPartitionLane(TopicPartition{Topic: "in"}, NewBackpressureGate(nil)).WithAccount(detaching)
	rec = &kgo.Record{Key: resp[:3], Value: resp[4:]}
	lane.Enqueue([]*kgo.Record{rec})
	if &rec.Value[0] == &resp[4] || string(rec.Value) != "value" {
		t.Fatal("under budget pressure the record must own a copy of its value")
	}
}
