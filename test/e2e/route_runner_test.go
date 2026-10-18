// Package e2e runs a compiled route through a real worker runner against an in-process kfake cluster:
// filter OrderPaid, mask email, and dead-letter a failing JS step, with no external infrastructure.
package e2e

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/internal/runtime/clients"
	"github.com/eventrouter/router/internal/runtime/delivery"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/failures"
	"github.com/eventrouter/router/internal/runtime/producers"
	"github.com/eventrouter/router/internal/runtime/reconcile"
	"github.com/eventrouter/router/internal/runtime/retry"
	"github.com/eventrouter/router/internal/runtime/runner"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/kafka"
	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/enginebind"
	"github.com/eventrouter/router/pkg/sandbox/engines"
)

// Fixture names the cluster, route and topics of the scenario.
type Fixture string

const (
	ConnectionID Fixture = "c1"
	RouteID      Fixture = "r-e2e"
	VersionID    Fixture = "v-e2e"
	TeamID       Fixture = "t-e2e"
	RouteName    Fixture = "orders-e2e"
	SourceTopic  Fixture = "orders"
	DestTopic    Fixture = "orders-public"
	DLQTopic     Fixture = "router.dlq.orders-e2e"
	WorkerID     Fixture = "w-e2e"
	MaskedEmail  Fixture = "j*******@example.com"
	PlainEmail   Fixture = "jane.doe@example.com"
	EventPaid    Fixture = "OrderPaid"
	EventCreated Fixture = "OrderCreated"
)

// Scenario sizes and timings.
const (
	Partitions   = 1
	PaidOK       = 3
	PaidFailing  = 1
	Created      = 2
	WaitTimeout  = 20 * time.Second
	PollInterval = 50 * time.Millisecond
	CommitEvery  = 50 * time.Millisecond
)

const routeDoc = `{"apiVersion":"router/v1","name":"orders-e2e",
"source":{"connectionId":"c1","topics":["orders"],"format":{"type":"json"},"startFrom":{"mode":"earliest"}},
"errorPolicy":{"onError":"dlq"},
"branches":[{"id":"main",
 "filter":{"type":"cond","field":"value.eventType","op":"eq","value":"OrderPaid"},
 "transform":{"steps":[
  {"id":"m1","kind":"mask","field":"value.email","style":"email"},
  {"id":"j1","kind":"js","code":"function transform(event, ctx) { if (event.value.fail) { throw new Error('boom'); } return event; }"}]},
 "destination":{"connectionId":"c1","topic":"orders-public"}}]}`

// haltFailer fails the test when a runner halts; the scenario must never halt.
type haltFailer struct{ t *testing.T }

// Halted implements runner.HaltSink.
func (f haltFailer) Halted(routeID string, _ domain.VersionRef, e *failures.HaltError) {
	f.t.Errorf("route %s halted: %v", routeID, e)
}

// harness owns the kfake cluster, the sandbox and the runner manager of one test.
type harness struct {
	t       *testing.T
	cluster *kfake.Cluster
	scripts reconcile.ScriptProvider
	runners *runner.Manager
}

func newHarness(t *testing.T) *harness {
	c, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(Partitions, string(SourceTopic), string(DestTopic)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	m, err := sandbox.NewManager(context.Background(), sandbox.DefaultConfig(), sandbox.ManagerDeps{Engines: engines.Registry(),
		Salts: reconcile.NewProcessSalts(), Binder: enginebind.Binder{Base: ctxhelpers.Set{}}})
	if err != nil {
		t.Skipf("sandbox unavailable: %v", err)
	}
	h := &harness{t: t, cluster: c, scripts: reconcile.SandboxScripts{Manager: m}}
	h.runners = runner.NewManager(runner.RouteRunnerFactory{Deps: h.deps()}, haltFailer{t: t}, h.log())
	return h
}

func (h *harness) log() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func (h *harness) deps() runner.Deps {
	opener := clients.NewOpenerManager(clients.NewFranzOpener())
	timings := domain.DefaultTimings()
	timings.Commit = CommitEvery
	pol := retry.Policy{Min: 10 * time.Millisecond, Max: 100 * time.Millisecond, Factor: 2}
	return runner.Deps{Opener: opener, Producers: producers.NewPool(opener, domain.SharingOff, pol), Delivery: delivery.NewDefaultManager(),
		Classifier: failures.DefaultClassifier{}, DLQTargets: failures.DLQTargetResolver{Default: domain.DLQTargetSource}, Encoder: record.Codec{},
		Metrics: runner.NewMetrics(nil, string(WorkerID)), Timings: timings, Retry: pol, Log: h.log(), Now: time.Now}
}

// compiled parses and compiles the route exactly like the worker's EngineVersionCompiler.
func (h *harness) compiled() runner.CompiledVersion {
	doc, err := config.Parse([]byte(routeDoc))
	if err != nil {
		h.t.Fatal(err)
	}
	cfg, err := doc.Config()
	if err != nil {
		h.t.Fatal(err)
	}
	scope := sandbox.RouteScope{RouteID: string(RouteID), VersionID: string(VersionID), TeamID: string(TeamID), Parallelism: 1, Mode: sandbox.ScopeWorker}
	opts := engine.CompileOptions{Scripts: h.scripts.Runtime(scope), Helpers: ctxhelpers.Set{}.WithDefaults()}
	p, err := compile.NewDefaultCompiler().Compile(context.Background(), doc, opts)
	if err != nil {
		h.t.Fatal(err)
	}
	spec := kafka.ConnectionSpec{ID: string(ConnectionID), BootstrapServers: h.cluster.ListenAddrs(), SecurityMode: kafka.SecurityPlaintext}
	return runner.CompiledVersion{Ref: domain.VersionRef{RouteID: string(RouteID), VersionID: string(VersionID), VersionNo: 1},
		RouteName: string(RouteName), TeamID: string(TeamID), Config: cfg, Pipeline: p,
		Connections: map[string]kafka.ConnectionSpec{string(ConnectionID): spec}}
}

// produce writes the sample events: paid ok, paid failing (JS throws) and created (filtered out).
func (h *harness) produce() {
	cl, err := kgo.NewClient(kgo.SeedBrokers(h.cluster.ListenAddrs()...), kgo.DefaultProduceTopic(string(SourceTopic)))
	if err != nil {
		h.t.Fatal(err)
	}
	defer cl.Close()
	recs := []*kgo.Record{}
	add := func(n int, typ Fixture, fail bool) {
		for i := 0; i < n; i++ {
			b, _ := json.Marshal(map[string]any{"eventType": string(typ), "email": string(PlainEmail), "n": i, "fail": fail})
			recs = append(recs, &kgo.Record{Value: b})
		}
	}
	add(PaidOK, EventPaid, false)
	add(PaidFailing, EventPaid, true)
	add(Created, EventCreated, false)
	if err := cl.ProduceSync(context.Background(), recs...).FirstErr(); err != nil {
		h.t.Fatal(err)
	}
}

// consume reads want records from topic or fails after WaitTimeout.
func (h *harness) consume(topic Fixture, want int) []*kgo.Record {
	cl, err := kgo.NewClient(kgo.SeedBrokers(h.cluster.ListenAddrs()...), kgo.ConsumeTopics(string(topic)),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		h.t.Fatal(err)
	}
	defer cl.Close()
	ctx, cancel := context.WithTimeout(context.Background(), WaitTimeout)
	defer cancel()
	out := []*kgo.Record{}
	for len(out) < want && ctx.Err() == nil {
		cl.PollFetches(ctx).EachRecord(func(r *kgo.Record) { out = append(out, r) })
	}
	if len(out) < want {
		h.t.Fatalf("topic %s: got %d records, want %d", topic, len(out), want)
	}
	return out
}

// noMore asserts the topic holds no records beyond want after a short settle period.
func (h *harness) noMore(topic Fixture, want int) {
	time.Sleep(10 * CommitEvery)
	if got := len(h.consume(topic, want)); got != want {
		h.t.Fatalf("topic %s: got %d records, want exactly %d", topic, got, want)
	}
}

func TestRouteFiltersMasksAndDeadLettersFailingJS(t *testing.T) {
	h := newHarness(t)
	h.produce()
	if err := h.runners.Start(context.Background(), h.compiled()); err != nil {
		t.Fatal(err)
	}
	defer h.runners.StopAll(context.Background())
	out := h.consume(DestTopic, PaidOK)
	for _, r := range out {
		v := map[string]any{}
		if err := json.Unmarshal(r.Value, &v); err != nil {
			t.Fatal(err)
		}
		if v["eventType"] != string(EventPaid) || v["email"] != string(MaskedEmail) || v["fail"] != false {
			t.Fatalf("unexpected output %s", r.Value)
		}
	}
	dlq := h.consume(DLQTopic, PaidFailing)
	if !strings.Contains(string(dlq[0].Value), string(PlainEmail)) {
		t.Fatalf("dead letter must carry the original event, got %s", dlq[0].Value)
	}
	h.assertDLQHeaders(dlq[0])
	h.noMore(DestTopic, PaidOK)
	h.noMore(DLQTopic, PaidFailing)
}

// assertDLQHeaders checks the router.* contract the dead-letters feature reads.
func (h *harness) assertDLQHeaders(r *kgo.Record) {
	got := map[string]string{}
	for _, hd := range r.Headers {
		got[hd.Key] = string(hd.Value)
	}
	if got[string(record.HeaderRouteID)] != string(RouteID) || got[string(record.HeaderSourceTopic)] != string(SourceTopic) {
		h.t.Fatalf("missing router headers: %v", got)
	}
	if !record.ErrorType(got[string(record.HeaderErrorType)]).Valid() {
		h.t.Fatalf("invalid error type header: %v", got)
	}
}
