// Alert inputs (RR-104): the series monitoring's alert rules read (design 1.2 PromQL of lag_above,
// error_rate_above, throughput_zero, match_rate_change) are checked on a real runner, including lag that
// keeps growing while the destination is down and fetching is paused.
package runner_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/twmb/franz-go/pkg/kfake"

	"github.com/eventrouter/router/internal/runtime/clients"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/runner"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/kafka"
)

const outageRoute = `{"apiVersion":"router/v1","name":"lagging","source":{"connectionId":"src","topics":["in"],"format":{"type":"json"},"startFrom":{"mode":"earliest"}},
"limits":{"maxInFlight":%d},
"branches":[{"id":"main","filter":null,"destination":{"connectionId":"dst","topic":"out"}}]}`

// outage is the shape of the destination-outage scenario.
var outage = struct {
	MaxInFlight, Before, Burst, Late int
}{MaxInFlight: 16, Before: 50, Burst: 900, Late: 600}

func outageVersion(t *testing.T, src *kfake.Cluster, dst chaosCluster) runner.CompiledVersion {
	t.Helper()
	doc, err := config.Parse([]byte(fmt.Sprintf(outageRoute, outage.MaxInFlight)))
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
	conns := map[string]kafka.ConnectionSpec{
		"src": {ID: "src", BootstrapServers: src.ListenAddrs(), SecurityMode: kafka.SecurityPlaintext},
		"dst": {ID: "dst", BootstrapServers: dst.ListenAddrs(), SecurityMode: kafka.SecurityPlaintext},
	}
	return runner.CompiledVersion{Ref: domain.VersionRef{RouteID: "lag", VersionID: "v1", VersionNo: 1}, RouteName: "lagging", Config: cfg, Pipeline: p, Connections: conns}
}

func TestLagKeepsGrowingWhileTheDestinationIsDown(t *testing.T) {
	src := cluster(t)
	dst := newChaosCluster(t)
	reg := prometheus.NewRegistry()
	m := newManagerOn(&haltRecorder{}, clients.NewOpenerManager(clients.NewFranzOpener()), "w1", reg)
	if err := m.Start(context.Background(), outageVersion(t, src, dst)); err != nil {
		t.Fatal(err)
	}
	defer m.StopAll(context.Background())
	produce(t, src, values(outage.Before, 0, 0)...)
	eventually(t, "first records committed", func() bool { return committed(t, src, "router.lag") == int64(outage.Before) })
	dst.listener.Stop()
	produce(t, src, values(outage.Burst, 0, 0)...)
	eventually(t, "burst visible as lag", func() bool { return gaugeSum(t, reg, "router_consumer_lag") >= float64(outage.Burst) })
	produce(t, src, values(outage.Late, 0, 0)...)
	want := float64(outage.Burst + outage.Late)
	eventually(t, "records produced after fetching paused visible as lag", func() bool { return gaugeSum(t, reg, "router_consumer_lag") >= want })
	if got := committed(t, src, "router.lag"); got != int64(outage.Before) {
		t.Fatalf("committed %d past the outage", got)
	}
	dst.listener.Start()
	total := int64(outage.Before + outage.Burst + outage.Late)
	eventually(t, "every record committed after recovery", func() bool { return committed(t, src, "router.lag") == total })
	eventually(t, "lag back to zero", func() bool { return gaugeSum(t, reg, "router_consumer_lag") == 0 })
}

func TestAlertRuleInputsMatchTheProcessedRecords(t *testing.T) {
	c := cluster(t)
	keep, drop, broken := 40, 30, 10
	produce(t, c, values(keep, drop, broken)...)
	reg := prometheus.NewRegistry()
	m := newManagerOn(&haltRecorder{}, clients.NewOpenerManager(clients.NewFranzOpener()), "w1", reg)
	if err := m.Start(context.Background(), compiled(t, c, "dlq")); err != nil {
		t.Fatal(err)
	}
	defer m.StopAll(context.Background())
	total := keep + drop + broken
	eventually(t, "every record committed", func() bool { return committed(t, c, "router.r1") == int64(total) })
	eventually(t, "counters settled", func() bool { return counterSum(t, reg, "router_events_consumed_total") == float64(total) })
	inputs := AlertInputs{
		Consumed: counterSum(t, reg, "router_events_consumed_total"), Filtered: counterSum(t, reg, "router_events_filtered_total"),
		Produced: counterSum(t, reg, "router_events_produced_total"), DLQ: counterSum(t, reg, "router_events_dlq_total"),
	}
	if inputs.Produced != float64(keep) || inputs.DLQ != float64(broken) || inputs.Filtered != float64(drop) {
		t.Fatalf("alert inputs %+v", inputs)
	}
	if got, want := inputs.ErrorRatePercent(), 100*float64(broken)/float64(total); got != want {
		t.Fatalf("error_rate_above input %v want %v", got, want)
	}
	if got, want := inputs.MatchRatePercent(), 100*float64(keep+broken)/float64(total); got != want {
		t.Fatalf("match_rate_change input %v want %v", got, want)
	}
}

// AlertInputs are the route counters the alert rules combine (monitoring design 1.2).
type AlertInputs struct {
	Consumed, Filtered, Produced, DLQ float64
}

// ErrorRatePercent is error_rate_above: 100 × dlq ÷ consumed.
func (a AlertInputs) ErrorRatePercent() float64 { return 100 * a.DLQ / a.Consumed }

// MatchRatePercent is the match rate behind match_rate_change: share of records not filtered.
func (a AlertInputs) MatchRatePercent() float64 { return 100 * (a.Consumed - a.Filtered) / a.Consumed }

func gaugeSum(t *testing.T, reg *prometheus.Registry, name string) float64 {
	return metricSum(t, reg, name)
}

func counterSum(t *testing.T, reg *prometheus.Registry, name string) float64 {
	return metricSum(t, reg, name)
}

// metricSum sums every series of a counter or gauge family.
func metricSum(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	var sum float64
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			sum += m.GetCounter().GetValue() + m.GetGauge().GetValue()
		}
	}
	return sum
}

func TestStopReturnsWhileTheDestinationIsDown(t *testing.T) {
	src := cluster(t)
	dst := newChaosCluster(t)
	m := newManagerOn(&haltRecorder{}, clients.NewOpenerManager(clients.NewFranzOpener()), "w1", nil)
	if err := m.Start(context.Background(), outageVersion(t, src, dst)); err != nil {
		t.Fatal(err)
	}
	produce(t, src, values(outage.Before, 0, 0)...)
	eventually(t, "first records committed", func() bool { return committed(t, src, "router.lag") == int64(outage.Before) })
	dst.listener.Stop()
	produce(t, src, values(outage.Burst, 0, 0)...)
	done := make(chan struct{})
	go func() {
		m.StopAll(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(stopBound):
		t.Fatal("stop blocked on flushing to an unreachable destination")
	}
	if got := committed(t, src, "router.lag"); got != int64(outage.Before) {
		t.Fatalf("committed %d past undelivered records", got)
	}
}

// stopBound is drain (5 s) for in-flight records plus drain for the producer flush, with slack.
const stopBound = 15 * time.Second

const latestRoute = `{"apiVersion":"router/v1","name":"fresh","source":{"connectionId":"c1","topics":["in"],"format":{"type":"json"},"startFrom":{"mode":"latest"}},
"branches":[{"id":"main","filter":null,"destination":{"connectionId":"c1","topic":"out"}}]}`

func TestFreshLatestRouteReportsNoLagForTheBacklogItSkips(t *testing.T) {
	c := cluster(t)
	produce(t, c, values(100, 0, 0)...)
	doc, err := config.Parse([]byte(latestRoute))
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := doc.Config()
	p, err := compile.NewDefaultCompiler().Compile(context.Background(), doc, engine.CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	v := runner.CompiledVersion{Ref: domain.VersionRef{RouteID: "fresh", VersionID: "v1", VersionNo: 1}, RouteName: "fresh", Config: cfg, Pipeline: p,
		Connections: map[string]kafka.ConnectionSpec{"c1": {ID: "c1", BootstrapServers: c.ListenAddrs(), SecurityMode: kafka.SecurityPlaintext}}}
	reg := prometheus.NewRegistry()
	m := newManagerOn(&haltRecorder{}, clients.NewOpenerManager(clients.NewFranzOpener()), "w1", reg)
	if err := m.Start(context.Background(), v); err != nil {
		t.Fatal(err)
	}
	defer m.StopAll(context.Background())
	time.Sleep(time.Second)
	if lag := gaugeSum(t, reg, "router_consumer_lag"); lag != 0 {
		t.Fatalf("a latest route without commits reported lag %v for a backlog it never reads", lag)
	}
	produce(t, c, values(5, 0, 0)...)
	eventually(t, "new records delivered", func() bool { return countRecords(t, c, "out") == 5 })
	eventually(t, "lag settles at zero", func() bool { return gaugeSum(t, reg, "router_consumer_lag") == 0 })
}
