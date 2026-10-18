// Replay integration tests (RR-203, RR-204, RR-208): ReplayService + ReplayCoordinator + planner on an
// in-memory state store and an in-process kfake cluster.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/state"
	"github.com/eventrouter/router/internal/runtime/state/statetest"
	"github.com/eventrouter/router/pkg/kafka"
)

const replayRouteJSON = `{"apiVersion":"router/v1","name":"orders","source":{"connectionId":"c1","topics":["in"],"format":{"type":"json"}},
"branches":[{"id":"main","destination":{"connectionId":"c1","topic":"out"}}]}`

// replayEvents is how many events each source partition holds, one second apart.
const replayEvents = 10

type kfakeClusters struct{ c *kfake.Cluster }

func (k kfakeClusters) Admin(ctx context.Context, _ string) (kafka.Client, error) {
	spec := kafka.ConnectionSpec{BootstrapServers: k.c.ListenAddrs(), SecurityMode: kafka.SecurityPlaintext}
	return kafka.DefaultClientFactoryRegistry().NewClient(ctx, spec, kafka.PurposeAdmin)
}

type replayFixture struct {
	t       *testing.T
	c       *kfake.Cluster
	mem     *statetest.Memory
	clock   *clock
	service *ReplayService
	coord   *ReplayCoordinator
	plan    *AssignmentPlanner
	base    time.Time
}

// newReplayFixture seeds a running route whose group has consumed every event of both partitions.
func newReplayFixture(t *testing.T) *replayFixture {
	t.Helper()
	c, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(2, "in", "out"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	mem, clk := statetest.New(), &clock{now: t0}
	mem.Demands["r1"] = state.RouteDemand{RouteID: "r1", Name: "orders", DesiredState: string(state.DesiredRunning), ActiveVersionID: "v1", Replicas: 1}
	mem.Configs["v1"] = []byte(replayRouteJSON)
	mem.Workers["w1"] = worker("w1", "a", t0)
	f := &replayFixture{t: t, c: c, mem: mem, clock: clk, base: time.Now().Add(-time.Hour).Truncate(time.Second)}
	f.service = &ReplayService{Stores: mem.Stores(), Now: clk.Now}
	timings := domain.DefaultTimings()
	f.coord = NewReplayCoordinator(mem.Stores(), kfakeClusters{c: c}, timings, slog.New(slog.NewTextHandler(io.Discard, nil)), clk.Now)
	f.plan = planner(t, mem, clk, domain.PlacementLeastLoaded)
	f.seed()
	return f
}

// seed produces timestamped events and commits the group at the end of both partitions.
func (f *replayFixture) seed() {
	cl, err := kgo.NewClient(kgo.SeedBrokers(f.c.ListenAddrs()...), kgo.RecordPartitioner(kgo.ManualPartitioner()))
	if err != nil {
		f.t.Fatal(err)
	}
	defer cl.Close()
	for p := int32(0); p < 2; p++ {
		for i := 0; i < replayEvents; i++ {
			rec := &kgo.Record{Topic: "in", Partition: p, Key: []byte(fmt.Sprintf("p%d-%d", p, i)), Value: []byte(`{}`), Timestamp: f.base.Add(time.Duration(i) * time.Second)}
			if err := cl.ProduceSync(context.Background(), rec).FirstErr(); err != nil {
				f.t.Fatal(err)
			}
		}
	}
	offsets := kadm.Offsets{}
	for p := int32(0); p < 2; p++ {
		offsets.Add(kadm.Offset{Topic: "in", Partition: p, At: replayEvents, LeaderEpoch: -1})
	}
	if err := kadm.NewClient(cl).CommitAllOffsets(context.Background(), "router.r1", offsets); err != nil {
		f.t.Fatal(err)
	}
}

// pass runs one scheduler pass: replays first, then planning.
func (f *replayFixture) pass() {
	if err := f.coord.Advance(context.Background()); err != nil {
		f.t.Fatal(err)
	}
	if err := f.plan.Plan(context.Background()); err != nil {
		f.t.Fatal(err)
	}
}

func (f *replayFixture) replay(id string) state.Replay {
	rp, err := f.service.Status(context.Background(), "r1", id)
	if err != nil {
		f.t.Fatal(err)
	}
	return rp
}

func (f *replayFixture) committed() map[int32]int64 {
	cl := mustAdmin(f.t, f.c)
	offs, err := cl.FetchOffsets(context.Background(), "router.r1")
	if err != nil {
		f.t.Fatal(err)
	}
	out := map[int32]int64{}
	offs.Each(func(o kadm.OffsetResponse) { out[o.Partition] = o.At })
	return out
}

func mustAdmin(t *testing.T, c *kfake.Cluster) *kadm.Client {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(c.ListenAddrs()...))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cl.Close)
	return kadm.NewClient(cl)
}

// redelivered reads the route's group from its committed offsets, like a restarted runner would.
func (f *replayFixture) redelivered() []*kgo.Record {
	cl, err := kgo.NewClient(kgo.SeedBrokers(f.c.ListenAddrs()...), kgo.ConsumerGroup("router.r1"), kgo.ConsumeTopics("in"), kgo.DisableAutoCommit())
	if err != nil {
		f.t.Fatal(err)
	}
	defer cl.Close()
	out := []*kgo.Record{}
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		fetches := cl.PollFetches(ctx)
		cancel()
		if fetches.NumRecords() == 0 {
			return out
		}
		out = append(out, fetches.Records()...)
	}
}

func TestReplayFromTimestampRedeliversEventsAfterFrom(t *testing.T) {
	f := newReplayFixture(t)
	f.pass()
	if len(leasesOf(f.mem, "r1")) != 1 {
		t.Fatal("running route must hold a lease before the replay")
	}
	from := f.base.Add(6 * time.Second)
	rp, err := f.service.Reset(context.Background(), ReplayRequest{RouteID: "r1", From: from, RequestedBy: "u1"})
	if err != nil || rp.Status != domain.ReplayPending {
		t.Fatalf("reset: %+v %v", rp, err)
	}
	f.pass()
	if got := f.replay(rp.ID).Status; got != domain.ReplayStopping || len(leasesOf(f.mem, "r1")) != 0 {
		t.Fatalf("after first pass: status %s, leases %d; want stopping with leases suspended", got, len(leasesOf(f.mem, "r1")))
	}
	f.pass()
	f.pass()
	done := f.replay(rp.ID)
	if done.Status != domain.ReplayDone || done.Partitions != 2 || done.ConsumerGroup != "router.r1" || done.FinishedAt == nil {
		t.Fatalf("replay = %+v, want done over 2 partitions of router.r1", done)
	}
	if len(leasesOf(f.mem, "r1")) != 1 {
		t.Fatal("the route must get its lease back once the replay is done")
	}
	for p, off := range f.committed() {
		if off != replayEvents-6 && off != 6 {
			t.Fatalf("partition %d committed at %d, want 6 (first event at or after from)", p, off)
		}
	}
	recs := f.redelivered()
	if len(recs) != 2*(replayEvents-6) {
		t.Fatalf("redelivered %d events, want %d", len(recs), 2*(replayEvents-6))
	}
	for _, r := range recs {
		if r.Timestamp.Before(from) {
			t.Fatalf("event %s at %s is before from %s", r.Key, r.Timestamp, from)
		}
	}
}

func TestReplayRefusedWhileGroupHasForeignMembers(t *testing.T) {
	f := newReplayFixture(t)
	foreign, err := kgo.NewClient(kgo.SeedBrokers(f.c.ListenAddrs()...), kgo.ConsumerGroup("router.r1"), kgo.ConsumeTopics("in"), kgo.DisableAutoCommit())
	if err != nil {
		t.Fatal(err)
	}
	defer foreign.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	foreign.PollFetches(ctx)
	cancel()
	before := f.committed()
	rp, err := f.service.Reset(context.Background(), ReplayRequest{RouteID: "r1", From: f.base})
	if err != nil {
		t.Fatal(err)
	}
	f.pass()
	f.pass()
	if got := f.replay(rp.ID).Status; got != domain.ReplayStopping {
		t.Fatalf("status %s, want stopping while a foreign member holds the group", got)
	}
	f.clock.now = f.clock.now.Add(domain.ReplayGroupWait.D() + time.Second)
	f.pass()
	failed := f.replay(rp.ID)
	if failed.Status != domain.ReplayFailed || failed.Error == "" {
		t.Fatalf("replay = %+v, want failed with the member message", failed)
	}
	t.Logf("refused: %s", failed.Error)
	if after := f.committed(); fmt.Sprint(after) != fmt.Sprint(before) {
		t.Fatalf("a refused replay must not move offsets: before %v after %v", before, after)
	}
	f.mem.Workers["w1"] = worker("w1", "a", f.clock.now)
	f.pass()
	if len(leasesOf(f.mem, "r1")) != 1 {
		t.Fatal("a failed replay must restore the route's lease")
	}
}

func TestReplayServiceValidatesRequests(t *testing.T) {
	f := newReplayFixture(t)
	ctx := context.Background()
	if _, err := f.service.Reset(ctx, ReplayRequest{RouteID: "nope", From: f.base}); !errors.Is(err, ErrRouteNotFound) {
		t.Fatalf("unknown route: %v", err)
	}
	if _, err := f.service.Reset(ctx, ReplayRequest{RouteID: "r1", From: f.base}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Reset(ctx, ReplayRequest{RouteID: "r1", From: f.base}); !errors.Is(err, ErrReplayInProgress) {
		t.Fatalf("second open replay: %v", err)
	}
	f.mem.Demands["r2"] = state.RouteDemand{RouteID: "r2", DesiredState: string(state.DesiredPaused)}
	if _, err := f.service.Reset(ctx, ReplayRequest{RouteID: "r2", From: f.base}); !errors.Is(err, ErrNoActiveVersion) {
		t.Fatalf("route without version: %v", err)
	}
	if _, err := f.service.Status(ctx, "r1", "missing"); !errors.Is(err, ErrReplayNotFound) {
		t.Fatalf("unknown replay: %v", err)
	}
}
