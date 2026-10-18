// Exactly-once integration tests (RR-201, RR-207) on an in-process kfake cluster: transactional
// batches, worker kill mid-batch, abort on halt and the start-time same-connection guard.
package runner_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/runtime/clients"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/runner"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/kafka"
)

const exactlyOnceJSON = `{"apiVersion":"router/v1","name":"orders","delivery":{"mode":"exactly_once"},
"source":{"connectionId":"c1","topics":["in"],"format":{"type":"json"},"startFrom":{"mode":"earliest"}},
"errorPolicy":{"onError":"%s"},
"branches":[{"id":"main","filter":{"type":"cond","field":"value.keep","op":"eq","value":true},"destination":{"connectionId":"%s","topic":"out"}}]}`

// eosTimings keep transactions and batches short so a killed worker's open transaction expires fast.
var eosTimings = struct{ Txn, Batch, Idle, Rebalance time.Duration }{Txn: 4 * time.Second, Batch: 3 * time.Second, Idle: 2 * time.Second,
	Rebalance: 6 * time.Second}

// Session gives chaos workers transact sessions through their faultNetwork (worker kill mid-batch).
func (o chaosOpener) Session(ctx context.Context, spec kafka.ConnectionSpec, purpose kafka.ClientPurpose, extra ...kgo.Opt) (*kgo.GroupTransactSession, error) {
	opts := append(extra, kgo.Dialer(o.network.Dial), kgo.SessionTimeout(chaosTimings.Session),
		kgo.HeartbeatInterval(chaosTimings.Heartbeat), kgo.TransactionTimeout(eosTimings.Txn), kgo.RebalanceTimeout(eosTimings.Rebalance))
	if os.Getenv("EOS_DEBUG") != "" {
		opts = append(opts, kgo.WithLogger(kgo.BasicLogger(os.Stderr, kgo.LogLevelInfo, func() string { return time.Now().Format("15:04:05.000 ") })))
	}
	return o.FranzOpener.Session(ctx, spec, purpose, opts...)
}

func eosWorker(name string) (chaosWorker, *haltRecorder) {
	network := newFaultNetwork()
	opener := clients.NewOpenerManager(chaosOpener{FranzOpener: clients.NewFranzOpener(), network: network})
	deps := testDeps(opener, name, nil)
	deps.Timings.Batch = eosTimings.Batch
	if os.Getenv("EOS_DEBUG") != "" {
		deps.Log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})).With("worker", name)
	}
	halts := &haltRecorder{}
	return chaosWorker{manager: runner.NewManager(runner.RouteRunnerFactory{Deps: deps}, halts, deps.Log), network: network}, halts
}

func eosVersion(t testing.TB, c *kfake.Cluster, onError, destination string) runner.CompiledVersion {
	t.Helper()
	doc, err := config.Parse([]byte(fmt.Sprintf(exactlyOnceJSON, onError, destination)))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := doc.Config()
	if err != nil {
		t.Fatal(err)
	}
	p, err := compile.NewDefaultCompiler().Compile(context.Background(), doc, engine.CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	conns := map[string]kafka.ConnectionSpec{}
	for _, id := range []string{"c1", destination} {
		conns[id] = kafka.ConnectionSpec{ID: id, BootstrapServers: c.ListenAddrs(), SecurityMode: kafka.SecurityPlaintext}
	}
	return runner.CompiledVersion{Ref: domain.VersionRef{RouteID: "r1", VersionID: "v1", VersionNo: 1}, RouteName: "orders", Config: cfg,
		Pipeline: p, Connections: conns}
}

// committedAudit reads a topic with read_committed until it is idle and counts records per key.
type committedAudit struct{ c *kfake.Cluster }

func (a committedAudit) Keys(t *testing.T, topic string) map[string]int {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(a.c.ListenAddrs()...), kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()), kgo.FetchIsolationLevel(kgo.ReadCommitted()))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	keys := map[string]int{}
	for {
		ctx, cancel := context.WithTimeout(context.Background(), eosTimings.Idle)
		fetches := cl.PollFetches(ctx)
		cancel()
		if fetches.NumRecords() == 0 {
			return keys
		}
		fetches.EachRecord(func(r *kgo.Record) { keys[string(r.Key)]++ })
	}
}

// ExactlyOnce fails unless every key in want appears exactly once and nothing else appears.
func (a committedAudit) ExactlyOnce(t *testing.T, topic string, want int) {
	t.Helper()
	keys := a.Keys(t, topic)
	for k, n := range keys {
		if n != 1 {
			t.Errorf("%s: key %s seen %d times by a read_committed consumer", topic, k, n)
		}
	}
	if len(keys) != want {
		t.Fatalf("%s: %d distinct keys committed, want %d", topic, len(keys), want)
	}
}

// mixedLoad produces keyed batches where every tenth record is not JSON (dead-lettered in the transaction).
func mixedLoad(t *testing.T, c *kfake.Cluster, batches, n int) (good, bad int, done <-chan struct{}) {
	ch := make(chan struct{})
	go func() {
		defer close(ch)
		for b := 0; b < batches; b++ {
			recs := make([]*kgo.Record, 0, n)
			for i := 0; i < n; i++ {
				v, _ := json.Marshal(map[string]any{"keep": true, "n": i})
				if i%10 == 0 {
					v = []byte("not-json")
				}
				recs = append(recs, &kgo.Record{Key: []byte(fmt.Sprintf("b%d-%d", b, i)), Value: v})
			}
			produceRetrying(t, c, recs)
			time.Sleep(chaosTimings.Interval)
		}
	}()
	bad = batches * ((n + 9) / 10)
	return batches*n - bad, bad, ch
}

func produceRetrying(t *testing.T, c *kfake.Cluster, recs []*kgo.Record) {
	deadline := time.Now().Add(30 * time.Second)
	err := keyedLoad{c: c}.attempt(recs)
	for err != nil && time.Now().Before(deadline) {
		time.Sleep(chaosTimings.Interval)
		err = keyedLoad{c: c}.attempt(recs)
	}
	if err != nil {
		t.Error(err)
	}
}

func TestExactlyOnceWorkerKillMidBatchDeliversEachKeyOnce(t *testing.T) {
	c := newChaosCluster(t)
	victim, _ := eosWorker("w1")
	survivor, survivorHalts := eosWorker("w2")
	v := eosVersion(t, c.Cluster, "dlq", "c1")
	for _, w := range []chaosWorker{victim, survivor} {
		if err := w.manager.Start(context.Background(), v); err != nil {
			t.Fatal(err)
		}
	}
	defer survivor.manager.StopAll(context.Background())
	adm := kadm.NewClient(mustClient(t, c.Cluster))
	eventually(t, "two members sharing the partitions", func() bool { return stableAssignments(t, adm) == 2 })
	good, bad, done := mixedLoad(t, c.Cluster, 60, 25)
	eventually(t, "first transactional outputs", func() bool { return countRecords(t, c.Cluster, "out") > 0 })
	victim.network.Cut()
	<-done
	deadline := time.Now().Add(30 * time.Second)
	for committed(t, c.Cluster, "router.r1") != int64(good+bad) {
		if survivorHalts.count() != 0 || time.Now().After(deadline) {
			t.Fatalf("survivor did not commit everything: committed %d of %d, halts %d, members %d", committed(t, c.Cluster, "router.r1"),
				good+bad, survivorHalts.count(), groupSize(t, adm))
		}
		time.Sleep(chaosTimings.Interval)
	}
	committedAudit{c: c.Cluster}.ExactlyOnce(t, "out", good)
	committedAudit{c: c.Cluster}.ExactlyOnce(t, "router.dlq.orders", bad)
	t.Logf("read_uncommitted sees %d output records for %d keys (aborted batches)", countRecords(t, c.Cluster, "out"), good)
	stopQuietly(victim)
}

func TestExactlyOnceHaltAbortsTheOpenBatch(t *testing.T) {
	c := cluster(t)
	halts := &haltRecorder{}
	deps := testDeps(clients.NewOpenerManager(clients.NewFranzOpener()), "w1", nil)
	m := runner.NewManager(runner.RouteRunnerFactory{Deps: deps}, halts, deps.Log)
	recs := []*kgo.Record{}
	for i, v := range []string{`{"keep":true}`, `{"keep":true}`, `not-json`, `{"keep":true}`} {
		recs = append(recs, &kgo.Record{Key: []byte(fmt.Sprintf("k%d", i)), Value: []byte(v), Partition: 0})
	}
	cl, err := kgo.NewClient(kgo.SeedBrokers(c.ListenAddrs()...), kgo.DefaultProduceTopic("in"), kgo.RecordPartitioner(kgo.ManualPartitioner()))
	if err != nil {
		t.Fatal(err)
	}
	if err := cl.ProduceSync(context.Background(), recs...).FirstErr(); err != nil {
		t.Fatal(err)
	}
	cl.Close()
	if err := m.Start(context.Background(), eosVersion(t, c, "pause", "c1")); err != nil {
		t.Fatal(err)
	}
	defer m.StopAll(context.Background())
	eventually(t, "pause policy halts the route", func() bool { return halts.count() == 1 })
	if n := len(committedAudit{c: c}.Keys(t, "out")); n != 0 {
		t.Fatalf("read_committed saw %d outputs of an aborted batch", n)
	}
	if off := committed(t, c, "router.r1"); off != 0 {
		t.Fatalf("aborted batch committed offsets: %d", off)
	}
}

func TestExactlyOnceRefusesCrossConnectionRoutes(t *testing.T) {
	c := cluster(t)
	halts := &haltRecorder{}
	m := newManager(halts)
	err := m.Start(context.Background(), eosVersion(t, c, "dlq", "c2"))
	if err == nil || halts.count() != 1 || halts.halts[0].Reason != domain.HaltDeliveryModeForbidden {
		t.Fatalf("cross-connection exactly_once must halt as delivery_mode_forbidden, err=%v halts=%d", err, halts.count())
	}
}

// stableAssignments counts members holding partitions of a Stable group.
func stableAssignments(t *testing.T, adm *kadm.Client) int {
	t.Helper()
	groups, err := adm.DescribeGroups(context.Background(), "router.r1")
	g := groups["router.r1"]
	if err != nil || g.State != "Stable" {
		return -1
	}
	n := 0
	for _, m := range g.Members {
		if a, ok := m.Assigned.AsConsumer(); ok && len(a.Topics) > 0 {
			n++
		}
	}
	return n
}
