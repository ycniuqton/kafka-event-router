package recipes

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/step"
)

// DefaultIfMissing writes a constant when the field is missing or null.
type DefaultIfMissing struct{}

func (DefaultIfMissing) Kind() config.StepKind { return config.StepKindDefaultIfMissing }
func (DefaultIfMissing) Group() step.Group     { return step.GroupValues }

func (DefaultIfMissing) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	target, ok := c.Path(raw.Field, config.TokenField)
	return &defaultIfMissing{Base: step.NewBase(c, raw), target: target, value: step.NumberNormalizer{}.Of(raw.Value)}, ok
}

type defaultIfMissing struct {
	step.Base
	target step.FieldRef
	value  any
}

func (s *defaultIfMissing) Apply(_ *step.StepContext, in engine.Event) ([]engine.Event, error) {
	if v, ok := s.target.Get(&in); ok && v != nil {
		return step.One(in), nil
	}
	if err := s.target.Write(&in, engine.ValueCloner{}.Clone(s.value)); err != nil {
		return nil, err
	}
	return step.One(in), nil
}
