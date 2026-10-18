package runner_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/internal/runtime/clients"
	"github.com/eventrouter/router/internal/runtime/delivery"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/failures"
	"github.com/eventrouter/router/internal/runtime/producers"
	"github.com/eventrouter/router/internal/runtime/retry"
	"github.com/eventrouter/router/internal/runtime/runner"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/kafka"
)

const routeJSON = `{"apiVersion":"router/v1","name":"orders","source":{"connectionId":"c1","topics":["in"],"format":{"type":"json"},"startFrom":{"mode":"earliest"}},
"errorPolicy":{"onError":"%s"},
"branches":[{"id":"main","filter":{"type":"cond","field":"value.keep","op":"eq","value":true},"destination":{"connectionId":"c1","topic":"out"}}]}`

type haltRecorder struct {
	mu    sync.Mutex
	halts []*failures.HaltError
}

func (h *haltRecorder) Halted(_ string, _ domain.VersionRef, e *failures.HaltError) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.halts = append(h.halts, e)
}

func (h *haltRecorder) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.halts)
}

func cluster(t testing.TB) *kfake.Cluster {
	t.Helper()
	c, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(2, "in", "out"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func compiled(t testing.TB, c *kfake.Cluster, onError string) runner.CompiledVersion {
	t.Helper()
	doc, err := config.Parse([]byte(fmt.Sprintf(routeJSON, onError)))
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
	spec := kafka.ConnectionSpec{ID: "c1", BootstrapServers: c.ListenAddrs(), SecurityMode: kafka.SecurityPlaintext}
	return runner.CompiledVersion{Ref: domain.VersionRef{RouteID: "r1", VersionID: "v1", VersionNo: 1}, RouteName: "orders", Config: cfg,
		Pipeline: p, Connections: map[string]kafka.ConnectionSpec{"c1": spec}}
}

func newManager(halts runner.HaltSink) *runner.Manager {
	return newManagerWith(halts, clients.NewOpenerManager(clients.NewFranzOpener()), "w1")
}

func newManagerWith(halts runner.HaltSink, opener *clients.OpenerManager, worker string) *runner.Manager {
	return newManagerOn(halts, opener, worker, nil)
}

func newManagerOn(halts runner.HaltSink, opener *clients.OpenerManager, worker string, reg prometheus.Registerer) *runner.Manager {
	deps := testDeps(opener, worker, reg)
	return runner.NewManager(runner.RouteRunnerFactory{Deps: deps}, halts, deps.Log)
}

func testDeps(opener *clients.OpenerManager, worker string, reg prometheus.Registerer) runner.Deps {
	timings := domain.DefaultTimings()
	timings.Commit, timings.Drain, timings.LagProbe = 50*time.Millisecond, 5*time.Second, 200*time.Millisecond
	pol := retry.Policy{Min: 10 * time.Millisecond, Max: 100 * time.Millisecond, Factor: 2}
	return runner.Deps{Opener: opener, Producers: producers.NewPool(opener, domain.SharingOn, pol), Delivery: delivery.NewDefaultManager(),
		Classifier: failures.DefaultClassifier{}, DLQTargets: failures.DLQTargetResolver{Default: domain.DLQTargetSource}, Encoder: record.Codec{},
		Metrics: runner.NewMetrics(reg, worker), Timings: timings, Retry: pol, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: time.Now}
}

func produce(t testing.TB, c *kfake.Cluster, values ...string) {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(c.ListenAddrs()...), kgo.DefaultProduceTopic("in"))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	recs := make([]*kgo.Record, 0, len(values))
	for i, v := range values {
		recs = append(recs, &kgo.Record{Key: []byte(fmt.Sprintf("k%d", i)), Value: []byte(v)})
	}
	if err := cl.ProduceSync(context.Background(), recs...).FirstErr(); err != nil {
		t.Fatal(err)
	}
}

func countRecords(t testing.TB, c *kfake.Cluster, topic string) int {
	t.Helper()
	adm := kadm.NewClient(mustClient(t, c))
	ends, err := adm.ListEndOffsets(context.Background(), topic)
	if err != nil {
		return -1
	}
	total := 0
	ends.Each(func(o kadm.ListedOffset) { total += int(o.Offset) })
	return total
}

func committed(t testing.TB, c *kfake.Cluster, group string) int64 {
	t.Helper()
	adm := kadm.NewClient(mustClient(t, c))
	offs, err := adm.FetchOffsets(context.Background(), group)
	if err != nil {
		return 0
	}
	var total int64
	offs.Each(func(o kadm.OffsetResponse) { total += o.At })
	return total
}

func mustClient(t testing.TB, c *kfake.Cluster) *kgo.Client {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(c.ListenAddrs()...))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cl.Close)
	return cl
}

func eventually(t testing.TB, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func values(keep, drop, broken int) []string {
	out := []string{}
	for i := 0; i < keep; i++ {
		b, _ := json.Marshal(map[string]any{"keep": true, "n": i})
		out = append(out, string(b))
	}
	for i := 0; i < drop; i++ {
		out = append(out, `{"keep":false}`)
	}
	for i := 0; i < broken; i++ {
		out = append(out, `{not json`)
	}
	return out
}

func TestRunnerEndToEndDeliversFiltersAndDeadLetters(t *testing.T) {
	c := cluster(t)
	produce(t, c, values(40, 30, 5)...)
	halts := &haltRecorder{}
	m := newManager(halts)
	v := compiled(t, c, "dlq")
	if err := m.Start(context.Background(), v); err != nil {
		t.Fatal(err)
	}
	eventually(t, "outputs", func() bool { return countRecords(t, c, "out") == 40 })
	eventually(t, "dead letters", func() bool { return countRecords(t, c, "router.dlq.orders") == 5 })
	eventually(t, "commit of every record", func() bool { return committed(t, c, "router.r1") == 75 })
	m.StopAll(context.Background())
	if halts.count() != 0 {
		t.Fatalf("unexpected halts: %d", halts.count())
	}
	snap := m.Snapshot()
	if len(snap) != 0 {
		t.Fatalf("runners left after StopAll: %+v", snap)
	}
}

func TestRunnerDeadLetterCarriesRouterHeaders(t *testing.T) {
	c := cluster(t)
	produce(t, c, `{broken`)
	m := newManager(&haltRecorder{})
	if err := m.Start(context.Background(), compiled(t, c, "dlq")); err != nil {
		t.Fatal(err)
	}
	defer m.StopAll(context.Background())
	cl, err := kgo.NewClient(kgo.SeedBrokers(c.ListenAddrs()...), kgo.ConsumeTopics("router.dlq.orders"), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var got *kgo.Record
	for got == nil && ctx.Err() == nil {
		cl.PollFetches(ctx).EachRecord(func(r *kgo.Record) { got = r })
	}
	if got == nil {
		t.Fatal("no dead letter")
	}
	rec, err := record.Codec{}.Decode(delivery.RecordMapper{}.FromKafka(got))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Failure.RouteID != "r1" || rec.Failure.ErrorType != record.ErrorDecode || rec.Source.Topic != "in" || string(rec.Value) != `{broken` {
		t.Fatalf("unexpected dead letter: %+v", rec)
	}
}

func TestRunnerSkipPolicyCommitsWithoutDeadLetters(t *testing.T) {
	c := cluster(t)
	produce(t, c, values(3, 0, 2)...)
	m := newManager(&haltRecorder{})
	if err := m.Start(context.Background(), compiled(t, c, "skip")); err != nil {
		t.Fatal(err)
	}
	eventually(t, "commit", func() bool { return committed(t, c, "router.r1") == 5 })
	m.StopAll(context.Background())
	if n := countRecords(t, c, "out"); n != 3 {
		t.Fatalf("outputs = %d", n)
	}
}

func TestRunnerPausePolicyStopsProcessingAfterTheFailingRecord(t *testing.T) {
	c := cluster(t)
	produceKeyed(t, c, "same", append([]string{`{broken`}, values(5, 0, 0)...)...)
	halts := &haltRecorder{}
	m := newManager(halts)
	if err := m.Start(context.Background(), compiled(t, c, "pause")); err != nil {
		t.Fatal(err)
	}
	eventually(t, "halt", func() bool { return halts.count() == 1 })
	time.Sleep(500 * time.Millisecond)
	if n := countRecords(t, c, "out"); n != 0 {
		t.Fatalf("%d records after the failing one were delivered although the route halted", n)
	}
	m.StopAll(context.Background())
}

// produceKeyed writes values with one key, so they share a partition and keep their order.
func produceKeyed(t testing.TB, c *kfake.Cluster, key string, values ...string) {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(c.ListenAddrs()...), kgo.DefaultProduceTopic("in"))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	recs := make([]*kgo.Record, 0, len(values))
	for _, v := range values {
		recs = append(recs, &kgo.Record{Key: []byte(key), Value: []byte(v)})
	}
	if err := cl.ProduceSync(context.Background(), recs...).FirstErr(); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerPausePolicyHaltsWithoutCommittingPastFailure(t *testing.T) {
	c := cluster(t)
	produce(t, c, `{broken`)
	halts := &haltRecorder{}
	m := newManager(halts)
	if err := m.Start(context.Background(), compiled(t, c, "pause")); err != nil {
		t.Fatal(err)
	}
	eventually(t, "halt", func() bool { return halts.count() == 1 })
	eventually(t, "error state", func() bool {
		cur, ok := m.Current("r1")
		return ok && cur.Failed
	})
	if got := committed(t, c, "router.r1"); got > 0 {
		t.Fatalf("committed %d past the failed record", got)
	}
	snap := m.Snapshot()
	if len(snap) != 1 || snap[0].State != domain.RunnerError {
		t.Fatalf("snapshot = %+v", snap)
	}
	m.StopAll(context.Background())
}
