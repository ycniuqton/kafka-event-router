package scenario

import (
	"errors"
	"fmt"
	"os"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/eventrouter/router/test/stress/kit/knee"
)

// BuiltIn holds the defaults applied before a file's `defaults:` block and each case.
var BuiltIn = struct {
	Scenario Scenario
	Case     Case
}{
	Scenario: Scenario{Target: "local", Criteria: knee.DefaultCriteria, SampleInterval: 5 * time.Second},
	Case: Case{
		Partitions: 12,
		Routes: RouteSpec{
			Count: 1, Topology: TopologyPerRoute, Replicas: 1, Delivery: "at_least_once", OnError: "dlq", StartFrom: "earliest",
			JS: JSNone, DeployBatch: 8, Steps: StepsSpec{Kind: "set"},
		},
		Load:    LoadSpec{Size: 1024, Lanes: 4, Streams: 256, Keys: "roundrobin", Acks: "all", Linger: 5 * time.Millisecond, Codec: "none", Producers: 1},
		Measure: MeasureSpec{SeqSource: "header", TrackBy: "stream"},
		Drain:   DrainSpec{Timeout: 2 * time.Minute},
	},
}

// file is the on-disk shape: scenario fields, a defaults block and the cases (decoded on top of the defaults).
type file struct {
	Scenario `yaml:",inline"`
	Defaults yaml.Node   `yaml:"defaults"`
	Cases    []yaml.Node `yaml:"cases"`
}

// Loader reads and validates scenario files.
type Loader struct{}

// Load reads one scenario file.
func (l Loader) Load(path string) (Scenario, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Scenario{}, err
	}
	return l.Parse(raw)
}

// Parse decodes a scenario document: built-in defaults ← defaults block ← case, then validates it.
func (l Loader) Parse(raw []byte) (Scenario, error) {
	f := file{Scenario: BuiltIn.Scenario}
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return Scenario{}, err
	}
	s := f.Scenario
	for i := range f.Cases {
		c, err := l.mergeCase(&f.Defaults, &f.Cases[i])
		if err != nil {
			return Scenario{}, fmt.Errorf("case %d: %w", i, err)
		}
		s.Cases = append(s.Cases, c)
	}
	return s, Validator{}.Check(s)
}

func (Loader) mergeCase(defaults, node *yaml.Node) (Case, error) {
	c := BuiltIn.Case
	if !defaults.IsZero() {
		if err := defaults.Decode(&c); err != nil {
			return Case{}, fmt.Errorf("defaults: %w", err)
		}
	}
	if err := node.Decode(&c); err != nil {
		return Case{}, err
	}
	return c, nil
}

// Validator checks a merged scenario.
type Validator struct{}

var (
	knownTopologies = map[Topology]bool{TopologyPerRoute: true, TopologySharedInput: true}
	knownJS         = map[JSMode]bool{JSNone: true, JSIdentity: true, JSThrowOnBad: true}
)

// Check returns every problem found, joined.
func (v Validator) Check(s Scenario) error {
	var errs []error
	if s.Name == "" {
		errs = append(errs, errors.New("name is required"))
	}
	if len(s.Cases) == 0 {
		errs = append(errs, errors.New("at least one case is required"))
	}
	seen := map[string]bool{}
	for _, c := range s.Cases {
		if seen[c.Name] {
			errs = append(errs, fmt.Errorf("case name %q repeated", c.Name))
		}
		seen[c.Name] = true
		errs = append(errs, v.checkCase(c)...)
	}
	return errors.Join(errs...)
}

func (Validator) checkCase(c Case) []error {
	var errs []error
	add := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("case %q: "+format, append([]any{c.Name}, args...)...))
	}
	if c.Name == "" {
		add("name is required")
	}
	if c.Routes.Count < 1 || c.Partitions < 1 {
		add("routes.count and partitions must be >= 1")
	}
	if !knownTopologies[c.Routes.Topology] || !knownJS[c.Routes.JS] {
		add("unknown topology %q or js %q", c.Routes.Topology, c.Routes.JS)
	}
	plan := c.Load.Plan()
	if len(plan.Steps) == 0 {
		add("load needs steps or ramp")
	}
	for _, st := range plan.Steps {
		if st.Duration <= 0 || st.Rate < 0 {
			add("load step %+v needs rate >= 0 and duration > 0", st)
		}
	}
	if c.Load.Producers < 1 || c.Load.Lanes < 1 {
		add("load.producers and load.lanes must be >= 1")
	}
	return errs
}
