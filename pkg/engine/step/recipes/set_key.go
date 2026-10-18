package recipes

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/step"
)

// SetKey replaces the record key from a value source.
type SetKey struct{}

func (SetKey) Kind() config.StepKind { return config.StepKindSetKey }
func (SetKey) Group() step.Group     { return step.GroupValues }

func (SetKey) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	src, ok := c.Values.Compile(c, c.Pointer.Token(config.TokenSource), raw.Source)
	return &setKey{Base: step.NewBase(c, raw), source: src}, ok
}

type setKey struct {
	step.Base
	source step.ValueResolver
}

func (s *setKey) Apply(sc *step.StepContext, in engine.Event) ([]engine.Event, error) {
	v, ok, err := s.source.Resolve(sc, &in)
	if err != nil {
		return nil, err
	}
	if !ok {
		if s.Strict {
			return nil, &step.StepError{Reason: step.ReasonFieldMissing, Field: string(config.TokenKey), Cause: step.ErrMissing}
		}
		return step.One(in), nil
	}
	in.Key = v
	return step.One(in), nil
}
