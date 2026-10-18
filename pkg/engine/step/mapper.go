package step

import (
	"sort"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/path"
)

// MapStartDefault is the start mode when a map step omits it.
const MapStartDefault = config.MapStartCopyInput

// mapStarts builds the initial output value per start mode.
var mapStarts = map[config.MapStart]func(in any) any{
	config.MapStartCopyInput: func(in any) any { return in },
	config.MapStartEmpty:     func(any) any { return map[string]any{} },
}

// MapperCompiler compiles the Visual Mapper `map` step: a target tree under value whose
// leaves are value sources (field, constant, formula, metadata) or removals.
type MapperCompiler struct{}

func (MapperCompiler) Kind() config.StepKind { return config.StepKindMap }
func (MapperCompiler) Group() Group          { return GroupFields }

func (MapperCompiler) Compile(c *StepCompileCtx, raw config.Step) (Step, bool) {
	if raw.Target == nil {
		c.Fail(config.IssueSchemaInvalid, config.TokenTarget, "map requires target")
		return nil, false
	}
	mode := MapStartDefault
	if raw.Start != nil {
		mode = *raw.Start
	}
	start, ok := mapStarts[mode]
	if !ok {
		c.Fail(config.IssueSchemaInvalid, config.TokenStart, "unknown map start %q", mode)
		return nil, false
	}
	m := &mapper{Base: NewBase(c, raw), start: start}
	tree := MapTree{c: c, m: m, ok: true}
	tree.walk(c.Pointer.Token(config.TokenTarget), path.Parser{}.Build(path.RootValue), *raw.Target)
	return m, tree.ok
}

// MapTree flattens the target tree into ordered assignments and removals at compile time.
type MapTree struct {
	c  *StepCompileCtx
	m  *mapper
	ok bool
}

func (t *MapTree) walk(ptr config.Pointer, at path.FieldPath, node config.MapField) {
	if node.Source != nil {
		src, ok := t.c.Values.Compile(t.c, ptr.Token(config.TokenSource), node.Source)
		t.ok = t.ok && ok
		t.m.assigns = append(t.m.assigns, mapAssign{target: FieldRef{FieldPath: at}, source: src})
	}
	if node.Remove != nil && *node.Remove {
		t.m.removals = append(t.m.removals, FieldRef{FieldPath: at})
	}
	names := make([]string, 0, len(node.Fields))
	for name := range node.Fields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t.walk(ptr.Token(config.TokenFields).Key(name), at.Child(name), node.Fields[name])
	}
}

type mapAssign struct {
	target FieldRef
	source ValueResolver
}

// mapper resolves every leaf against the input first, then builds the output value.
type mapper struct {
	Base
	start    func(in any) any
	assigns  []mapAssign
	removals []FieldRef
}

// mapValue is one resolved leaf.
type mapValue struct {
	value   any
	present bool
}

func (s *mapper) Apply(sc *StepContext, in engine.Event) ([]engine.Event, error) {
	resolved, err := s.resolve(sc, &in)
	if err != nil {
		return nil, err
	}
	in.Value = s.start(in.Value)
	for _, r := range s.removals {
		r.Delete(&in)
	}
	for i, a := range s.assigns {
		if !resolved[i].present {
			continue
		}
		if err := a.target.Write(&in, resolved[i].value); err != nil {
			return nil, err
		}
	}
	return One(in), nil
}

// resolve reads every leaf source from the unmodified input event.
func (s *mapper) resolve(sc *StepContext, in *engine.Event) ([]mapValue, error) {
	out := make([]mapValue, len(s.assigns))
	for i, a := range s.assigns {
		v, ok, err := a.source.Resolve(sc, in)
		if err != nil {
			return nil, err
		}
		if !ok {
			if err := s.Missing(a.target); err != nil {
				return nil, err
			}
		}
		out[i] = mapValue{value: v, present: ok}
	}
	return out, nil
}
