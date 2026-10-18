// Package scenario is the YAML model of stress scenarios (test/stress/scenarios/*.yaml) and of the targets they run
// against (test/stress/targets.yaml).
package scenario

import (
	"time"

	"github.com/eventrouter/router/test/stress/kit/knee"
	"github.com/eventrouter/router/test/stress/kit/pacing"
)

// Topology says how routes of a case share topics.
type Topology string

const (
	TopologyPerRoute    Topology = "per_route"
	TopologySharedInput Topology = "shared_input"
)

// JSMode selects the JavaScript step added to each route.
type JSMode string

const (
	JSNone       JSMode = "none"
	JSIdentity   JSMode = "identity"
	JSThrowOnBad JSMode = "throw_on_bad"
)

// Scenario is one YAML file.
type Scenario struct {
	Name           string                 `yaml:"name"`
	Description    string                 `yaml:"description"`
	Target         string                 `yaml:"target"`
	Criteria       knee.Criteria          `yaml:"criteria"`
	SampleInterval time.Duration          `yaml:"sampleInterval"`
	Incremental    bool                   `yaml:"incremental"`
	StopOnKnee     bool                   `yaml:"stopOnKnee"`
	TargetExtras   map[string]TargetExtra `yaml:"targetExtras"`
	Cases          []Case                 `yaml:"-"`
}

// TargetExtra adds scenario-specific samplers to a target (e.g. the extra workers of a scale-out scenario).
type TargetExtra struct {
	Metrics   []MetricsSpec `yaml:"metrics"`
	Processes []ProcessSpec `yaml:"processes"`
}

// HooksSpec are shell commands run synchronously before provisioning and after the case (always).
type HooksSpec struct {
	Before []string `yaml:"before"`
	After  []string `yaml:"after"`
}

// Case is one measured configuration (after defaults are merged in).
type Case struct {
	Name       string       `yaml:"name"`
	Partitions int32        `yaml:"partitions"`
	Routes     RouteSpec    `yaml:"routes"`
	Load       LoadSpec     `yaml:"load"`
	Measure    MeasureSpec  `yaml:"measure"`
	Drain      DrainSpec    `yaml:"drain"`
	Actions    []ActionSpec `yaml:"actions"`
	Hooks      HooksSpec    `yaml:"hooks"`
	Notes      string       `yaml:"notes"`
}

// RouteSpec describes the routes of a case.
type RouteSpec struct {
	Count       int            `yaml:"count"`
	Topology    Topology       `yaml:"topology"`
	Replicas    int            `yaml:"replicas"`
	MaxInFlight int            `yaml:"maxInFlight"`
	Delivery    string         `yaml:"delivery"`
	OnError     string         `yaml:"onError"`
	StartFrom   string         `yaml:"startFrom"`
	Filter      FilterSpec     `yaml:"filter"`
	Steps       StepsSpec      `yaml:"steps"`
	JS          JSMode         `yaml:"js"`
	DeployBatch int            `yaml:"deployConcurrency"`
	Extra       map[string]any `yaml:"extra"`
}

// FilterSpec generates a filter of N always-true conditions (AND), or uses Raw as is.
type FilterSpec struct {
	Conditions int            `yaml:"conditions"`
	Raw        map[string]any `yaml:"raw"`
}

// StepsSpec generates N recipe steps of one kind, or uses Raw as is.
type StepsSpec struct {
	Count int              `yaml:"count"`
	Kind  string           `yaml:"kind"`
	Raw   []map[string]any `yaml:"raw"`
}

// LoadSpec is the offered load of a case.
type LoadSpec struct {
	Steps     []pacing.Step `yaml:"steps"`
	Ramp      *RampSpec     `yaml:"ramp"`
	Size      int           `yaml:"size"`
	Lanes     int           `yaml:"lanes"`
	Streams   int           `yaml:"streams"`
	Keys      string        `yaml:"keys"`
	Acks      string        `yaml:"acks"`
	Linger    time.Duration `yaml:"linger"`
	BatchSize int           `yaml:"batchBytes"`
	Codec     string        `yaml:"compression"`
	BadRatio  float64       `yaml:"badRatio"`
	Producers int           `yaml:"producers"`
	Template  string        `yaml:"template"`
}

// RampSpec is a linear ramp of steps.
type RampSpec struct {
	From     float64       `yaml:"from"`
	To       float64       `yaml:"to"`
	Step     float64       `yaml:"step"`
	Duration time.Duration `yaml:"duration"`
}

// MeasureSpec configures the measurer of a case.
type MeasureSpec struct {
	Disabled      bool   `yaml:"disabled"`
	ReadCommitted bool   `yaml:"readCommitted"`
	SeqSource     string `yaml:"seqSource"`
	IncludeDLQ    bool   `yaml:"includeDlq"`
	TrackBy       string `yaml:"trackBy"`
}

// DrainSpec bounds the wait for lag to reach zero after the load ends.
type DrainSpec struct {
	Timeout time.Duration `yaml:"timeout"`
}

// ActionSpec runs a shell command at an offset from the start of the load (fault injection, scale-out).
type ActionSpec struct {
	At   time.Duration `yaml:"at"`
	Name string        `yaml:"name"`
	Run  string        `yaml:"run"`
}

// WithExtras returns t with the scenario's extra samplers for that target appended.
func (s Scenario) WithExtras(t Target) Target {
	extra := s.TargetExtras[t.Name]
	t.Metrics = append(append([]MetricsSpec{}, t.Metrics...), extra.Metrics...)
	t.Processes = append(append([]ProcessSpec{}, t.Processes...), extra.Processes...)
	return t
}

// Plan returns the load plan of the case.
func (l LoadSpec) Plan() pacing.Plan {
	if l.Ramp != nil {
		return pacing.Planner{}.Ramp(l.Ramp.From, l.Ramp.To, l.Ramp.Step, l.Ramp.Duration)
	}
	return pacing.Plan{Steps: l.Steps}
}
