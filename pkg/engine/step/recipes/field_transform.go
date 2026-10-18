package recipes

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/step"
)

// ValueMapper converts one field value; kinds that do not apply to a value return it unchanged.
type ValueMapper interface {
	Map(sc *step.StepContext, v any) (any, error)
}

// FieldTransformBuilder compiles recipes that read `field`, convert it and write `to` (default `field`).
type FieldTransformBuilder struct{}

// Build parses field and the optional to member and wraps the mapper.
func (FieldTransformBuilder) Build(c *step.StepCompileCtx, raw config.Step, m ValueMapper, ok bool) (step.Step, bool) {
	source, okF := c.Path(raw.Field, config.TokenField)
	target := source
	okT := true
	if raw.To != nil {
		target, okT = c.Path(raw.To, config.TokenTo)
	}
	return &fieldTransform{Base: step.NewBase(c, raw), source: source, target: target, mapper: m}, ok && okF && okT
}

// fieldTransform applies a ValueMapper; missing is a no-op unless strict, null stays null.
type fieldTransform struct {
	step.Base
	source, target step.FieldRef
	mapper         ValueMapper
}

func (s *fieldTransform) Apply(sc *step.StepContext, in engine.Event) ([]engine.Event, error) {
	v, ok := s.source.Get(&in)
	if !ok {
		return step.One(in), s.Missing(s.source)
	}
	if v == nil {
		return step.One(in), nil
	}
	out, err := s.mapper.Map(sc, v)
	if err != nil {
		return nil, &step.StepError{Reason: step.ReasonConvertFailed, Field: s.source.String(), Cause: err}
	}
	if err := s.target.Write(&in, out); err != nil {
		return nil, err
	}
	return step.One(in), nil
}
