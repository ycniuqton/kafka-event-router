package infer

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/eventrouter/router/pkg/config"
)

// StepEmitter turns matches into recipe steps: copies first (a source feeding several targets), then
// rename (same parent) / move, then constants (set), then one remove of every input node no target keeps.
type StepEmitter struct{}

// Emission is what the emitter produced.
type Emission struct {
	Steps    []config.Step
	Unmapped []string
}

// Emit builds the steps for matches over the input tree and its source pool.
func (e StepEmitter) Emit(matches []Match, input Tree, pool *SourcePool) Emission {
	plan := &relocationPlan{pool: pool, moved: map[int]bool{}, remaining: map[int]int{}}
	var copies, moves, sets []config.Step
	for _, m := range matches {
		if m.Source == nil {
			sets = append(sets, e.set(m.Target))
			continue
		}
		step, kind := plan.step(m)
		switch kind {
		case relocationCopy:
			copies = append(copies, step)
		case relocationMove:
			moves = append(moves, step)
		}
	}
	steps := append(append(copies, moves...), sets...)
	removal := RemovalPlanner{targets: TargetPaths{}.Of(matches), moved: plan.movedPaths()}.Plan(input, pool)
	if len(removal.Paths) > 0 {
		steps = append(steps, config.Step{Kind: config.StepKindRemove, Fields: removal.Paths})
	}
	return Emission{Steps: steps, Unmapped: removal.Dropped}
}

// set writes the target's literal value.
func (StepEmitter) set(target Leaf) config.Step {
	field := target.Path()
	return config.Step{Kind: config.StepKindSet, Field: &field, Source: &config.ValueSource{From: config.ValueSourceKindConstant, Value: target.Value}}
}

// relocation names what a matched source leaf turns into.
type relocation int

const (
	relocationNone relocation = iota
	relocationCopy
	relocationMove
)

// relocationPlan decides per match between copy and rename/move: a source's last use moves it,
// earlier uses copy it; a source that also stays in place is only copied.
type relocationPlan struct {
	pool      *SourcePool
	moved     map[int]bool
	remaining map[int]int
}

// step returns the step of one match (relocationNone for a field already in place).
func (p *relocationPlan) step(m Match) (config.Step, relocation) {
	i := p.pool.Index(m.Source)
	from, to := m.Source.Path(), m.Target.Path()
	if _, seen := p.remaining[i]; !seen {
		p.remaining[i] = p.pool.Uses(i)
	}
	p.remaining[i]--
	if from == to {
		p.moved[i] = false
		return config.Step{}, relocationNone
	}
	if p.remaining[i] > 0 || p.keptInPlace(i) {
		return config.Step{Kind: config.StepKindCopy, From: &from, To: &to}, relocationCopy
	}
	p.moved[i] = true
	kind := config.StepKindMove
	if m.Source.Parent() == m.Target.Parent() {
		kind = config.StepKindRename
	}
	return config.Step{Kind: kind, From: &from, To: &to}, relocationMove
}

// keptInPlace reports whether an earlier match of the same source kept it at its own path.
func (p *relocationPlan) keptInPlace(i int) bool {
	moved, seen := p.moved[i]
	return seen && !moved
}

// movedPaths lists the input paths that no longer exist after the moves.
func (p *relocationPlan) movedPaths() map[string]bool {
	out := map[string]bool{}
	for i, moved := range p.moved {
		if moved {
			out[p.pool.Leaves[i].Path()] = true
		}
	}
	return out
}

// TargetPaths collects every target path and its ancestors (nodes the output keeps).
type TargetPaths struct{}

// Of returns the set.
func (TargetPaths) Of(matches []Match) map[string]bool {
	out := map[string]bool{}
	for _, m := range matches {
		for n := 1; n <= len(m.Target.Segments); n++ {
			out[Leaf{Segments: m.Target.Segments[:n]}.Path()] = true
		}
	}
	return out
}

// Removal lists the fields to remove and the input leaves whose data is dropped.
type Removal struct {
	Paths   []string
	Dropped []string
}

// RemovalPlanner removes every maximal input node outside the target paths that still holds data.
type RemovalPlanner struct {
	targets map[string]bool
	moved   map[string]bool
}

// Plan walks the input tree.
func (r RemovalPlanner) Plan(input Tree, pool *SourcePool) Removal {
	out := Removal{Paths: []string{}, Dropped: []string{}}
	r.walk(input.Root, nil, &out)
	for i, l := range pool.Leaves {
		if !pool.Used(i) && !r.targets[l.Path()] {
			out.Dropped = append(out.Dropped, l.Path())
		}
	}
	sort.Strings(out.Dropped)
	return out
}

func (r RemovalPlanner) walk(obj map[string]any, prefix []string, out *Removal) {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		segs := append(append([]string{}, prefix...), k)
		path := Leaf{Segments: segs}.Path()
		child, isObj := obj[k].(map[string]any)
		switch {
		case !(PathRules{}).Addressable(segs) || r.moved[path]:
		case r.targets[path] && isObj:
			r.walk(child, segs, out)
		case !r.targets[path] && !r.under(path):
			out.Paths = append(out.Paths, path)
		}
	}
}

// under reports whether path is inside a target leaf (already overwritten).
func (r RemovalPlanner) under(path string) bool {
	for t := range r.targets {
		if strings.HasPrefix(path, t+string(Dot)) {
			return true
		}
	}
	return false
}

// StepJSON renders steps as raw JSON.
type StepJSON struct{}

// Of returns the steps.
func (StepJSON) Of(steps []config.Step) ([]json.RawMessage, error) {
	out := make([]json.RawMessage, 0, len(steps))
	for _, s := range steps {
		raw, err := json.Marshal(s)
		if err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, nil
}
