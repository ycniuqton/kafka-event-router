package recipes

import (
	"fmt"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/step"
)

// SplitArray emits one event per element of the array at `field`, writing the element to
// `to` (default `field`); an empty array drops the event.
type SplitArray struct{}

func (SplitArray) Kind() config.StepKind { return config.StepKindSplitArray }
func (SplitArray) Group() step.Group     { return step.GroupStruct }

func (SplitArray) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	source, okF := c.Path(raw.Field, config.TokenField)
	target, okT := source, true
	if raw.To != nil {
		target, okT = c.Path(raw.To, config.TokenTo)
	}
	return &splitArray{Base: step.NewBase(c, raw), source: source, target: target}, okF && okT
}

type splitArray struct {
	step.Base
	source, target step.FieldRef
}

func (s *splitArray) Apply(_ *step.StepContext, in engine.Event) ([]engine.Event, error) {
	v, ok := s.source.Get(&in)
	if !ok {
		return step.One(in), s.Missing(s.source)
	}
	items, ok := v.([]any)
	if !ok {
		return nil, &step.StepError{Reason: step.ReasonTypeMismatch, Field: s.source.String(), Cause: fmt.Errorf("%T is not an array", v)}
	}
	out := make([]engine.Event, 0, len(items))
	for _, item := range items {
		ev := in.Clone()
		if err := s.target.Write(&ev, engine.ValueCloner{}.Clone(item)); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, nil
}
