package runner_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/twmb/franz-go/pkg/kadm"

	"github.com/eventrouter/router/internal/runtime/clients"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/runner"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compile"
)

func compiledJSON(t *testing.T, base runner.CompiledVersion, raw string, version int) runner.CompiledVersion {
	t.Helper()
	doc, err := config.Parse([]byte(raw))
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
	v := base
	v.Ref = domain.VersionRef{RouteID: "r1", VersionID: fmt.Sprintf("v%d", version), VersionNo: version}
	v.Config, v.Pipeline = cfg, p
	return v
}

func groupMember(t *testing.T, adm *kadm.Client) string {
	t.Helper()
	groups, err := adm.DescribeGroups(context.Background(), "router.r1")
	if err != nil {
		t.Fatal(err)
	}
	members := groups["router.r1"].Members
	if len(members) != 1 {
		return ""
	}
	return members[0].MemberID
}

func TestRunnerSwapUnderLoadKeepsGroupMembershipAndLosesNothing(t *testing.T) {
	c := cluster(t)
	m := newManager(&haltRecorder{})
	v1 := compiled(t, c, "dlq")
	if err := m.Start(context.Background(), v1); err != nil {
		t.Fatal(err)
	}
	defer m.StopAll(context.Background())
	produce(t, c, values(200, 0, 0)...)
	eventually(t, "first batch", func() bool { return countRecords(t, c, "out") == 200 })
	adm := kadm.NewClient(mustClient(t, c))
	eventually(t, "stable group", func() bool { return groupMember(t, adm) != "" })
	before := groupMember(t, adm)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 10; i++ {
			produce(t, c, values(50, 10, 0)...)
			time.Sleep(10 * time.Millisecond)
		}
	}()
	raw := strings.Replace(fmt.Sprintf(routeJSON, "dlq"), `"value":true`, `"value":true},"transform":{"steps":[]`, 1)
	v2 := compiledJSON(t, v1, raw, 2)
	if err := m.Swap(context.Background(), v2); err != nil {
		t.Fatal(err)
	}
	<-done
	eventually(t, "all outputs", func() bool { return countRecords(t, c, "out") == 700 })
	eventually(t, "all commits", func() bool { return committed(t, c, "router.r1") == 800 })
	if after := groupMember(t, adm); after != before {
		t.Fatalf("group member changed %q → %q: swap left the group", before, after)
	}
	if cur, _ := m.Current("r1"); cur.Ref.VersionNo != 2 {
		t.Fatalf("running version %d", cur.Ref.VersionNo)
	}
}

func TestRunnerHaltsOnMissingDestinationTopic(t *testing.T) {
	c := cluster(t)
	produce(t, c, values(1, 0, 0)...)
	halts := &haltRecorder{}
	m := newManager(halts)
	v := compiled(t, c, "dlq")
	raw := strings.Replace(fmt.Sprintf(routeJSON, "dlq"), `"topic":"out"`, `"topic":"missing"`, 1)
	v = compiledJSON(t, v, raw, 1)
	if err := m.Start(context.Background(), v); err != nil {
		t.Fatal(err)
	}
	defer m.StopAll(context.Background())
	eventually(t, "halt", func() bool { return halts.count() == 1 })
	if h := halts.halts[0]; h.Reason != domain.HaltDestinationPermanent || h.Category != domain.CategoryDestinationPermanent {
		t.Fatalf("halt %+v", h)
	}
	if got := committed(t, c, "router.r1"); got != 0 {
		t.Fatalf("committed %d past an undelivered record", got)
	}
}

// runnerStateSeries returns the router_runner_state gauge values keyed by version label.
func runnerStateSeries(t *testing.T, reg *prometheus.Registry) map[string]float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]float64{}
	for _, f := range families {
		if f.GetName() != "router_runner_state" {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "version" {
					out[l.GetValue()] = m.GetGauge().GetValue()
				}
			}
		}
	}
	return out
}

func TestRunnerSwapMovesStateSeriesToNewVersion(t *testing.T) {
	c := cluster(t)
	reg := prometheus.NewRegistry()
	m := newManagerOn(&haltRecorder{}, clients.NewOpenerManager(clients.NewFranzOpener()), "w1", reg)
	v1 := compiled(t, c, "dlq")
	if err := m.Start(context.Background(), v1); err != nil {
		t.Fatal(err)
	}
	defer m.StopAll(context.Background())
	eventually(t, "v1 running", func() bool { return runnerStateSeries(t, reg)["1"] == 1 })
	if err := m.Swap(context.Background(), compiledJSON(t, v1, fmt.Sprintf(routeJSON, "dlq"), 2)); err != nil {
		t.Fatal(err)
	}
	got := runnerStateSeries(t, reg)
	if _, stale := got["1"]; stale || got["2"] != 1 || len(got) != 1 {
		t.Fatalf("runner_state series after swap: %v", got)
	}
}
