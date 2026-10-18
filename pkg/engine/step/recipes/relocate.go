package recipes

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/step"
)

// relocateCompiler compiles from/to recipes (rename, copy, move).
type relocateCompiler struct {
	kind       config.StepKind
	keepSource bool
}

func (r relocateCompiler) Kind() config.StepKind { return r.kind }
func (relocateCompiler) Group() step.Group       { return step.GroupFields }

func (r relocateCompiler) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	from, okFrom := c.Path(raw.From, config.TokenFrom)
	to, okTo := c.Path(raw.To, config.TokenTo)
	if !okFrom || !okTo {
		return nil, false
	}
	return &relocate{Base: step.NewBase(c, raw), from: from, to: to, keepSource: r.keepSource}, true
}

// relocate moves or copies a value between paths; a missing source is a no-op unless strict.
type relocate struct {
	step.Base
	from, to   step.FieldRef
	keepSource bool
}

func (s *relocate) Apply(_ *step.StepContext, in engine.Event) ([]engine.Event, error) {
	v, ok := s.from.Get(&in)
	if !ok {
		return step.One(in), s.Missing(s.from)
	}
	if s.keepSource {
		v = engine.ValueCloner{}.Clone(v)
	} else {
		s.from.Delete(&in)
	}
	if err := s.to.Write(&in, v); err != nil {
		return nil, err
	}
	return step.One(in), nil
}
