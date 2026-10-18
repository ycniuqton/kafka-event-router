package runner

import (
	"strings"
	"testing"
	"time"

	"github.com/eventrouter/router/test/stress/kit/knee"
	"github.com/eventrouter/router/test/stress/kit/latency"
	"github.com/eventrouter/router/test/stress/kit/pacing"
	"github.com/eventrouter/router/test/stress/kit/sampling"
	"github.com/eventrouter/router/test/stress/kit/scenario"
	"github.com/eventrouter/router/test/stress/kit/seqtrack"
	"github.com/eventrouter/router/test/stress/kit/stream"
)

func TestNamingIsGuardedAndTopologyAware(t *testing.T) {
	n := NewNaming("s4-fanout")
	if !strings.HasPrefix(n.Base, "stress-s4-") || !strings.HasPrefix(n.Connection(), "stress-") {
		t.Fatalf("base = %s", n.Base)
	}
	shared := n.Routes("n3", scenario.TopologySharedInput, 0, 3)
	if shared[0].Input != shared[2].Input || shared[0].Output == shared[1].Output || !strings.HasPrefix(shared[1].Group, "stress-") {
		t.Errorf("shared = %+v", shared)
	}
	per := n.Routes("r", scenario.TopologyPerRoute, 10, 2)
	if per[0].Input == per[1].Input || !strings.HasSuffix(per[0].Name, "-r10") || !strings.HasSuffix(per[1].DLQ, "-r11-dlq") {
		t.Errorf("per-route = %+v", per)
	}
}

func TestDeliveryVerdict(t *testing.T) {
	sums := []stream.ProduceSummary{{Acked: 600}, {Acked: 400}}
	ok := DeliveryCheck{}.Verdict(sums, &stream.MeasureSummary{Delivery: seqtrack.Summary{Unique: 3000, Duplicates: 5}}, 3)
	if ok.Acked != 3000 || ok.Lost != 0 || !ok.ZeroLoss || ok.Duplicates != 5 {
		t.Errorf("fan-out verdict = %+v", ok)
	}
	lost := DeliveryCheck{}.Verdict(sums, &stream.MeasureSummary{Delivery: seqtrack.Summary{Unique: 990, Gaps: 10}}, 1)
	if lost.Lost != 10 || lost.ZeroLoss {
		t.Errorf("loss verdict = %+v", lost)
	}
	if none := (DeliveryCheck{}).Verdict(sums, nil, 1); none.Measured || none.ZeroLoss {
		t.Errorf("unmeasured verdict = %+v", none)
	}
}

func TestProduceArgsSplitRateBetweenProducers(t *testing.T) {
	l := scenario.BuiltIn.Case.Load
	l.Producers, l.Streams = 2, 10
	l.Steps = []pacing.Step{{Rate: 30000, Duration: 5 * time.Minute}, {Rate: 0, Duration: time.Minute}}
	args := strings.Join(LoadgenArgs{}.Produce("b:1", []string{"stress-a", "stress-b"}, l, 1, 5*time.Second), " ")
	for _, want := range []string{"--steps 15000:5m0s,0:1m0s", "--stream-prefix p1", "--streams 5", "--topic stress-a,stress-b"} {
		if !strings.Contains(args, want) {
			t.Errorf("args %q lack %q", args, want)
		}
	}
	many := strings.Join(LoadgenArgs{}.Produce("b:1", make([]string, 40), l, 0, time.Second), " ")
	if !strings.Contains(many, "--streams 40") {
		t.Errorf("streams must cover every topic: %s", many)
	}
}

func TestTimelineBuildsStepsForKnee(t *testing.T) {
	t0 := time.Unix(1_800_000_000, 0)
	tl := NewTimeline(t0)
	for i, rate := range []float64{1000, 2000} {
		start := t0.Add(time.Duration(i) * time.Minute)
		tl.OnLine("p0", stream.Line{Kind: stream.KindStep, Step: &stream.StepReport{Index: i, TargetRate: rate, StartNs: start.UnixNano()}})
		for s := 0; s < 60; s += 5 {
			at := start.Add(time.Duration(s) * time.Second)
			tl.OnSnapshot(sampling.Snapshot{At: at, GroupLag: int64(float64(i) * 100 * float64(s))})
			h := latency.New()
			h.Record(20 * time.Millisecond)
			tl.OnLine("m", stream.Line{Kind: stream.KindWindow, AtNs: at.UnixNano(), Measure: &stream.MeasureWindow{Histogram: h.Sparse()}})
		}
	}
	sums := []stream.ProduceSummary{{Steps: []stream.StepReport{
		{Index: 0, AchievedRate: 1000, EndNs: t0.Add(time.Minute).UnixNano()},
		{Index: 1, AchievedRate: 2000, EndNs: t0.Add(2 * time.Minute).UnixNano()},
	}}}
	crit := knee.DefaultCriteria
	crit.Warmup = 0
	r := knee.Detector{Criteria: crit}.Analyze(tl.Steps(sums))
	if r.MaxSustainable != 1000 || r.KneeRate != 2000 || r.KneeReason != knee.VerdictLagGrowing {
		t.Errorf("knee = %+v", r)
	}
	if r.Steps[0].Latency.Count != 12 {
		t.Errorf("windows assigned to step 0 = %d", r.Steps[0].Latency.Count)
	}
}
