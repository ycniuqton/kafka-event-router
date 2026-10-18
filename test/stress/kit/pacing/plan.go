// Package pacing describes load plans (fixed rate or ramp steps) and paces a producer lane to its share of a rate.
package pacing

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Syntax holds the separators of plan specs: "rate:duration,rate:duration" and "from:to:step:duration".
var Syntax = struct {
	Steps string
	Pair  string
}{Steps: ",", Pair: ":"}

// Step is one constant-rate segment; Rate 0 means "as fast as possible".
type Step struct {
	Rate     float64       `json:"rate" yaml:"rate"`
	Duration time.Duration `json:"duration" yaml:"duration"`
}

// Plan is the ordered list of steps a producer runs.
type Plan struct {
	Steps []Step `json:"steps"`
}

// Planner builds plans from CLI/YAML specs.
type Planner struct{}

// Fixed returns a single-step plan.
func (Planner) Fixed(rate float64, d time.Duration) Plan {
	return Plan{Steps: []Step{{Rate: rate, Duration: d}}}
}

// ParseSteps parses "1000:30s,2000:30s".
func (Planner) ParseSteps(spec string) (Plan, error) {
	var p Plan
	for _, part := range strings.Split(spec, Syntax.Steps) {
		fields := strings.Split(strings.TrimSpace(part), Syntax.Pair)
		if len(fields) != 2 {
			return Plan{}, fmt.Errorf("step %q: want rate%sduration", part, Syntax.Pair)
		}
		rate, err := strconv.ParseFloat(fields[0], 64)
		if err != nil || rate < 0 {
			return Plan{}, fmt.Errorf("step %q: bad rate", part)
		}
		d, err := time.ParseDuration(fields[1])
		if err != nil || d <= 0 {
			return Plan{}, fmt.Errorf("step %q: bad duration", part)
		}
		p.Steps = append(p.Steps, Step{Rate: rate, Duration: d})
	}
	return p, nil
}

// ParseRamp parses "from:to:step:duration", e.g. "10000:100000:10000:5m" (inclusive of to).
func (pl Planner) ParseRamp(spec string) (Plan, error) {
	f := strings.Split(spec, Syntax.Pair)
	if len(f) != 4 {
		return Plan{}, fmt.Errorf("ramp %q: want from:to:step:duration", spec)
	}
	nums := make([]float64, 3)
	for i := range nums {
		v, err := strconv.ParseFloat(f[i], 64)
		if err != nil || v <= 0 {
			return Plan{}, fmt.Errorf("ramp %q: bad number %q", spec, f[i])
		}
		nums[i] = v
	}
	d, err := time.ParseDuration(f[3])
	if err != nil || d <= 0 {
		return Plan{}, fmt.Errorf("ramp %q: bad duration", spec)
	}
	return pl.Ramp(nums[0], nums[1], nums[2], d), nil
}

// Ramp returns steps from, from+step, ... up to and including to.
func (Planner) Ramp(from, to, step float64, d time.Duration) Plan {
	var p Plan
	for r := from; r <= to+step/1e6; r += step {
		p.Steps = append(p.Steps, Step{Rate: r, Duration: d})
	}
	return p
}

// Total is the summed duration of all steps.
func (p Plan) Total() time.Duration {
	var d time.Duration
	for _, s := range p.Steps {
		d += s.Duration
	}
	return d
}
