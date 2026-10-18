// Package knee decides, per load step, whether the system sustained the offered rate (consumer lag flat and p99
// end-to-end latency under the limit) and finds the knee: the first step that was not sustained.
package knee

import (
	"time"

	"github.com/eventrouter/router/test/stress/kit/latency"
)

// Verdict is the outcome of one step.
type Verdict string

const (
	VerdictSustained     Verdict = "sustained"
	VerdictLagGrowing    Verdict = "lag_growing"
	VerdictLatency       Verdict = "p99_over_limit"
	VerdictLoadgenLimit  Verdict = "loadgen_below_target"
	VerdictInsufficient  Verdict = "insufficient_samples"
	VerdictNotApplicable Verdict = "no_measurement"
)

// Criteria are the sustainability thresholds.
type Criteria struct {
	P99Limit        time.Duration `json:"p99Limit" yaml:"p99Limit"`
	LagSlopeAbs     float64       `json:"lagSlopeAbs" yaml:"lagSlopeAbs"`
	LagSlopeRel     float64       `json:"lagSlopeRel" yaml:"lagSlopeRel"`
	Warmup          time.Duration `json:"warmup" yaml:"warmup"`
	MinSamples      int           `json:"minSamples" yaml:"minSamples"`
	MinAchievedFrac float64       `json:"minAchievedFrac" yaml:"minAchievedFrac"`
}

// DefaultCriteria: p99 < 1 s, lag slope ≤ max(50 ev/s, 1% of the offered rate), first 30 s of a step ignored.
var DefaultCriteria = Criteria{P99Limit: time.Second, LagSlopeAbs: 50, LagSlopeRel: 0.01, Warmup: 30 * time.Second, MinSamples: 3, MinAchievedFrac: 0.95}

// Sample is one 5 s observation inside a step.
type Sample struct {
	At  time.Time `json:"at"`
	Lag float64   `json:"lag"`
}

// Step is everything observed during one load step.
type Step struct {
	Index        int                `json:"index"`
	TargetRate   float64            `json:"targetRate"`
	AchievedRate float64            `json:"achievedRate"`
	Start        time.Time          `json:"start"`
	End          time.Time          `json:"end"`
	Samples      []Sample           `json:"-"`
	Latency      *latency.Histogram `json:"-"`
}

// StepResult is the evaluated step.
type StepResult struct {
	Index         int             `json:"index"`
	TargetRate    float64         `json:"targetRate"`
	AchievedRate  float64         `json:"achievedRate"`
	LagSlope      float64         `json:"lagSlopePerSec"`
	LagSlopeLimit float64         `json:"lagSlopeLimit"`
	LagEnd        float64         `json:"lagEnd"`
	Latency       latency.Summary `json:"latency"`
	Verdict       Verdict         `json:"verdict"`
}

// Result is the knee analysis of a ramp.
type Result struct {
	Steps          []StepResult `json:"steps"`
	MaxSustainable float64      `json:"maxSustainableRate"`
	KneeRate       float64      `json:"kneeRate,omitempty"`
	KneeReason     Verdict      `json:"kneeReason,omitempty"`
}

// Detector evaluates steps against criteria.
type Detector struct {
	Criteria Criteria
}

// Analyze evaluates every step; the knee is the first step that is not sustained.
func (d Detector) Analyze(steps []Step) Result {
	var r Result
	for _, s := range steps {
		sr := d.Evaluate(s)
		r.Steps = append(r.Steps, sr)
		if r.KneeReason != "" {
			continue
		}
		if sr.Verdict == VerdictSustained {
			r.MaxSustainable = sr.TargetRate
			continue
		}
		r.KneeRate, r.KneeReason = sr.TargetRate, sr.Verdict
	}
	return r
}

// Evaluate judges one step.
func (d Detector) Evaluate(s Step) StepResult {
	out := StepResult{Index: s.Index, TargetRate: s.TargetRate, AchievedRate: s.AchievedRate}
	if s.Latency != nil {
		out.Latency = s.Latency.Summary()
	}
	steady := d.steady(s)
	out.LagSlopeLimit = max(d.Criteria.LagSlopeAbs, d.Criteria.LagSlopeRel*s.TargetRate)
	if len(steady) > 0 {
		out.LagEnd = steady[len(steady)-1].Lag
	}
	if len(steady) < d.Criteria.MinSamples {
		out.Verdict = VerdictInsufficient
		return out
	}
	out.LagSlope = Regression{}.Slope(steady)
	out.Verdict = d.verdict(s, out)
	return out
}

func (d Detector) verdict(s Step, r StepResult) Verdict {
	switch {
	case r.LagSlope > r.LagSlopeLimit:
		return VerdictLagGrowing
	case s.Latency == nil || s.Latency.Count() == 0:
		return VerdictNotApplicable
	case s.Latency.Quantile(0.99) >= d.Criteria.P99Limit:
		return VerdictLatency
	case s.TargetRate > 0 && s.AchievedRate < d.Criteria.MinAchievedFrac*s.TargetRate:
		return VerdictLoadgenLimit
	default:
		return VerdictSustained
	}
}

func (d Detector) steady(s Step) []Sample {
	from := s.Start.Add(d.Criteria.Warmup)
	var out []Sample
	for _, smp := range s.Samples {
		if !smp.At.Before(from) && (s.End.IsZero() || !smp.At.After(s.End)) {
			out = append(out, smp)
		}
	}
	return out
}

// Regression fits lag(t) = a + b·t by least squares.
type Regression struct{}

// Slope returns b in lag units per second (0 for fewer than two distinct instants).
func (Regression) Slope(samples []Sample) float64 {
	if len(samples) < 2 {
		return 0
	}
	t0 := samples[0].At
	var n, sx, sy, sxx, sxy float64
	for _, s := range samples {
		x := s.At.Sub(t0).Seconds()
		n++
		sx += x
		sy += s.Lag
		sxx += x * x
		sxy += x * s.Lag
	}
	den := n*sxx - sx*sx
	if den == 0 {
		return 0
	}
	return (n*sxy - sx*sy) / den
}
