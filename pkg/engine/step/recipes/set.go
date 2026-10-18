package recipes

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/step"
)

// Set writes a value from a field, constant, metadata or formula source.
type Set struct{}

func (Set) Kind() config.StepKind { return config.StepKindSet }
func (Set) Group() step.Group     { return step.GroupValues }

func (Set) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	target, okT := c.Path(raw.Field, config.TokenField)
	src, okS := c.Values.Compile(c, c.Pointer.Token(config.TokenSource), raw.Source)
	return &set{Base: step.NewBase(c, raw), target: target, source: src}, okT && okS
}

type set struct {
	step.Base
	target step.FieldRef
	source step.ValueResolver
}

func (s *set) Apply(sc *step.StepContext, in engine.Event) ([]engine.Event, error) {
	v, ok, err := s.source.Resolve(sc, &in)
	if err != nil {
		return nil, err
	}
	if !ok {
		return step.One(in), s.Missing(s.target)
	}
	if err := s.target.Write(&in, v); err != nil {
		return nil, err
	}
	return step.One(in), nil
}
