package runner_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/internal/runtime/delivery"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/runner"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/trace"
	"github.com/eventrouter/router/pkg/kafka"
)

const retryRouteJSON = `{"apiVersion":"router/v1","name":"orders","source":{"connectionId":"c1","topics":["in"],"format":{"type":"json"},"startFrom":{"mode":"earliest"}},
"errorPolicy":{"onError":"dlq","retry":{"enabled":true,"maxAttempts":2,"baseDelayMs":50,"maxDelayMs":100}},
"branches":[{"id":"main","filter":{"type":"cond","field":"value.keep","op":"eq","value":true},"destination":{"connectionId":"c1","topic":"out"}}]}`

// flakyPipeline fails the first `fails` Process calls with an unavailable lookup (fails < 0: always).
type flakyPipeline struct {
	engine.Pipeline
	fails int64
	calls atomic.Int64
}

func (p *flakyPipeline) Process(ctx context.Context, ev engine.Event, tr trace.Tracer) ([]engine.Out, error) {
	n := p.calls.Add(1)
	if p.fails < 0 || n <= p.fails {
		dep := &engine.DependencyError{Kind: engine.DependencyLookup, Name: "customers", Cause: errors.New("connection refused")}
		return nil, &engine.ProcessError{Type: engine.ErrStep, Branch: "main", StepID: "lookup", Cause: dep}
	}
	return p.Pipeline.Process(ctx, ev, tr)
}

func retryVersion(t *testing.T, c *kfake.Cluster, fails int64) (runner.CompiledVersion, *flakyPipeline) {
	t.Helper()
	doc, err := config.Parse([]byte(retryRouteJSON))
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
	flaky := &flakyPipeline{Pipeline: p, fails: fails}
	spec := kafka.ConnectionSpec{ID: "c1", BootstrapServers: c.ListenAddrs(), SecurityMode: kafka.SecurityPlaintext}
	return runner.CompiledVersion{Ref: domain.VersionRef{RouteID: "r1", VersionID: "v1", VersionNo: 1}, RouteName: "orders", Config: cfg,
		Pipeline: flaky, Connections: map[string]kafka.ConnectionSpec{"c1": spec}}, flaky
}

func TestRetryTopicRecoversTransientFailuresWithoutDeadLetters(t *testing.T) {
	c := cluster(t)
	produce(t, c, values(3, 1, 0)...)
	m := newManager(&haltRecorder{})
	v, _ := retryVersion(t, c, 4)
	if err := m.Start(context.Background(), v); err != nil {
		t.Fatal(err)
	}
	defer m.StopAll(context.Background())
	eventually(t, "outputs after retry", func() bool { return countRecords(t, c, "out") == 3 })
	eventually(t, "source committed", func() bool { return committed(t, c, "router.r1") == 4 })
	eventually(t, "tier committed", func() bool { return committed(t, c, "router.r1.retry") == 4 })
	if n := countRecords(t, c, "router.retry.orders.1"); n != 4 {
		t.Fatalf("tier 1 records %d", n)
	}
	if n := countRecords(t, c, "router.dlq.orders"); n > 0 {
		t.Fatalf("dead letters %d", n)
	}
}

func TestRetryTopicParksExhaustedRecordsWithAttempts(t *testing.T) {
	c := cluster(t)
	produce(t, c, `{"keep":true}`)
	m := newManager(&haltRecorder{})
	v, _ := retryVersion(t, c, -1)
	if err := m.Start(context.Background(), v); err != nil {
		t.Fatal(err)
	}
	defer m.StopAll(context.Background())
	eventually(t, "dead letter after attempts", func() bool { return countRecords(t, c, "router.dlq.orders") == 1 })
	if countRecords(t, c, "router.retry.orders.1") != 1 || countRecords(t, c, "router.retry.orders.2") != 1 {
		t.Fatal("expected one record per tier")
	}
	rec := firstRecord(t, c, "router.dlq.orders")
	got, err := record.Codec{}.Decode(delivery.RecordMapper{}.FromKafka(rec))
	if err != nil {
		t.Fatal(err)
	}
	if got.Failure.ErrorType != record.ErrorLookupUnavailable || got.Failure.Attempts != 2 || got.Source.Topic != "in" || got.Source.Offset != 0 {
		t.Fatalf("dead letter %+v source %+v", got.Failure, got.Source)
	}
	eventually(t, "source committed", func() bool { return committed(t, c, "router.r1") == 1 })
}

func firstRecord(t *testing.T, c *kfake.Cluster, topic string) *kgo.Record {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(c.ListenAddrs()...), kgo.ConsumeTopics(topic), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
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
		t.Fatalf("no record on %s", topic)
	}
	return got
}
