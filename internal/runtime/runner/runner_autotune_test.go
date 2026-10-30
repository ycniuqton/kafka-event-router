package runner_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
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

const autotuneRoute = `{"apiVersion":"router/v1","name":"tuned","source":{"connectionId":"c1","topics":["in"],"format":{"type":"json"},"startFrom":{"mode":"earliest"}},
"limits":{"minInFlight":1,"maxInFlight":64},
"branches":[{"id":"main","destination":{"connectionId":"c1","topic":"out"}}]}`

// autotuneLoad is the keyed sequence load of the ordering test.
var autotuneLoad = struct{ Keys, PerKey int }{Keys: 16, PerKey: 250}

type keyedSeq struct {
	K   string `json:"k"`
	Seq int    `json:"seq"`
}

// flappingPressure reports memory pressure on 1 of every 25 checks, forcing shrinks between growths.
type flappingPressure struct{ n atomic.Int64 }

func (p *flappingPressure) UnderPressure() bool { return p.n.Add(1)%25 == 0 }

// eagerTuning reacts on tiny windows so the limit moves many times during the test.
func eagerTuning() *runner.InFlightControllers {
	t := runner.DefaultGradientTuning
	t.MinAcks, t.HoldWindows = 1, 1
	return runner.NewInFlightControllers(t)
}

func autotuneVersion(t *testing.T, c *kfake.Cluster) runner.CompiledVersion {
	t.Helper()
	doc, err := config.Parse([]byte(autotuneRoute))
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := doc.Config()
	p, err := compile.NewDefaultCompiler().Compile(context.Background(), doc, engine.CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	spec := kafka.ConnectionSpec{ID: "c1", BootstrapServers: c.ListenAddrs(), SecurityMode: kafka.SecurityPlaintext}
	return runner.CompiledVersion{Ref: domain.VersionRef{RouteID: "r1", VersionID: "v1", VersionNo: 3}, RouteName: "tuned", Config: cfg,
		Pipeline: p, Connections: map[string]kafka.ConnectionSpec{"c1": spec}}
}

func produceTuningLoad(t *testing.T, c *kfake.Cluster) {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(c.ListenAddrs()...), kgo.DefaultProduceTopic("in"))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	var recs []*kgo.Record
	for seq := 0; seq < autotuneLoad.PerKey; seq++ {
		for k := 0; k < autotuneLoad.Keys; k++ {
			v, _ := json.Marshal(keyedSeq{K: fmt.Sprintf("key-%d", k), Seq: seq})
			recs = append(recs, &kgo.Record{Key: []byte(fmt.Sprintf("key-%d", k)), Value: v})
		}
	}
	if err := cl.ProduceSync(context.Background(), recs...).FirstErr(); err != nil {
		t.Fatal(err)
	}
}

// readKeyed consumes want outputs and fails on any per-key order break.
func readKeyed(t *testing.T, c *kfake.Cluster, want int) map[string]int {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(c.ListenAddrs()...), kgo.ConsumeTopics("out"), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	next := map[string]int{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for n := 0; n < want && ctx.Err() == nil; {
		cl.PollFetches(ctx).EachRecord(func(r *kgo.Record) {
			var ev keyedSeq
			_ = json.Unmarshal(r.Value, &ev)
			if ev.Seq != next[ev.K] {
				t.Errorf("%s: seq %d, want %d (order broken or lost)", ev.K, ev.Seq, next[ev.K])
			}
			next[ev.K] = ev.Seq + 1
			n++
		})
	}
	return next
}

// The limit moves every 5 ms between 1 and 64 (growth plus forced shrinks); every key must still arrive
// complete and in order, and the runner exports router_route_inflight_limit.
func TestRunnerAutotuneKeepsOrderAndLosesNothing(t *testing.T) {
	c, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(4, "in", "out"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	produceTuningLoad(t, c)
	reg := prometheus.NewRegistry()
	deps := testDeps(clients.NewOpenerManager(clients.NewFranzOpener()), "w1", reg)
	deps.InFlight = runner.InFlightTuning{Controllers: eagerTuning(), Pressure: &flappingPressure{}, Interval: 5 * time.Millisecond}
	m := runner.NewManager(runner.RouteRunnerFactory{Deps: deps}, &haltRecorder{}, deps.Log)
	if err := m.Start(context.Background(), autotuneVersion(t, c)); err != nil {
		t.Fatal(err)
	}
	defer m.StopAll(context.Background())
	next := readKeyed(t, c, autotuneLoad.Keys*autotuneLoad.PerKey)
	for k := 0; k < autotuneLoad.Keys; k++ {
		if got := next[fmt.Sprintf("key-%d", k)]; got != autotuneLoad.PerKey {
			t.Fatalf("key-%d delivered %d of %d", k, got, autotuneLoad.PerKey)
		}
	}
	if v, ok := inflightGauge(t, reg); !ok || v < 1 || v > 64 {
		t.Fatalf("router_route_inflight_limit = %v (present %v)", v, ok)
	}
}

// inflightGauge returns router_route_inflight_limit of route r1 version 3.
func inflightGauge(t *testing.T, reg *prometheus.Registry) (float64, bool) {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() != string(runner.MetricInFlightLimit) {
			continue
		}
		for _, m := range f.GetMetric() {
			labels := map[string]string{}
			for _, l := range m.GetLabel() {
				labels[l.GetName()] = l.GetValue()
			}
			if labels["route_id"] == "r1" && labels["version"] == "3" && labels["worker"] == "w1" {
				return m.GetGauge().GetValue(), true
			}
		}
	}
	return 0, false
}
