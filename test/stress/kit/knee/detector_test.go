package knee

import (
	"math"
	"testing"
	"time"

	"github.com/eventrouter/router/test/stress/kit/latency"
)

var t0 = time.Unix(1_800_000_000, 0)

func stepWith(index int, rate float64, lagAt func(sec float64) float64, lat time.Duration) Step {
	s := Step{Index: index, TargetRate: rate, AchievedRate: rate, Start: t0, End: t0.Add(5 * time.Minute), Latency: latency.New()}
	for sec := 0.0; sec <= 300; sec += 5 {
		s.Samples = append(s.Samples, Sample{At: t0.Add(time.Duration(sec) * time.Second), Lag: lagAt(sec)})
	}
	for i := 0; i < 1000; i++ {
		s.Latency.Record(lat)
	}
	return s
}

func flat(float64) float64 { return 120 }

func TestSlopeOfLinearLag(t *testing.T) {
	var samples []Sample
	for i := 0; i < 10; i++ {
		samples = append(samples, Sample{At: t0.Add(time.Duration(i*5) * time.Second), Lag: 1000 + 250*float64(i*5)})
	}
	if got := (Regression{}).Slope(samples); math.Abs(got-250) > 1e-9 {
		t.Errorf("slope = %v, want 250", got)
	}
	if (Regression{}).Slope(samples[:1]) != 0 {
		t.Error("single sample slope must be 0")
	}
}

func TestKneeIsFirstUnsustainedStep(t *testing.T) {
	d := Detector{Criteria: DefaultCriteria}
	steps := []Step{
		stepWith(0, 10000, flat, 20*time.Millisecond),
		stepWith(1, 20000, func(s float64) float64 { return 120 + 30*s }, 40*time.Millisecond),
		stepWith(2, 30000, func(s float64) float64 { return 5000 * s }, 3*time.Second),
	}
	r := d.Analyze(steps)
	if r.MaxSustainable != 20000 || r.KneeRate != 30000 || r.KneeReason != VerdictLagGrowing {
		t.Errorf("result = %+v", r)
	}
	if r.Steps[1].Verdict != VerdictSustained || r.Steps[1].LagSlopeLimit != 200 {
		t.Errorf("slope 30/s under limit 200/s must be sustained: %+v", r.Steps[1])
	}
}

func TestLatencyOverLimitIsKnee(t *testing.T) {
	r := Detector{Criteria: DefaultCriteria}.Analyze([]Step{stepWith(0, 5000, flat, 1500*time.Millisecond)})
	if r.KneeReason != VerdictLatency || r.MaxSustainable != 0 {
		t.Errorf("result = %+v", r)
	}
}

func TestLoadgenShortfallIsReported(t *testing.T) {
	s := stepWith(0, 100000, flat, 10*time.Millisecond)
	s.AchievedRate = 60000
	if v := (Detector{Criteria: DefaultCriteria}).Evaluate(s).Verdict; v != VerdictLoadgenLimit {
		t.Errorf("verdict = %s", v)
	}
}

func TestWarmupSamplesAreIgnored(t *testing.T) {
	s := stepWith(0, 10000, func(sec float64) float64 {
		if sec < 30 {
			return 100000 - 3000*sec
		}
		return 50
	}, 10*time.Millisecond)
	if v := (Detector{Criteria: DefaultCriteria}).Evaluate(s); v.Verdict != VerdictSustained || v.LagSlope != 0 {
		t.Errorf("warm-up drain must not count: %+v", v)
	}
}

func TestTooFewSamples(t *testing.T) {
	s := stepWith(0, 1000, flat, time.Millisecond)
	s.Samples = s.Samples[len(s.Samples)-2:]
	if v := (Detector{Criteria: DefaultCriteria}).Evaluate(s).Verdict; v != VerdictInsufficient {
		t.Errorf("verdict = %s", v)
	}
}
