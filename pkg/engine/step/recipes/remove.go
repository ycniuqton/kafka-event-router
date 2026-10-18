package recipes

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/step"
)

// Remove deletes fields; missing fields are skipped unless strict.
type Remove struct{}

func (Remove) Kind() config.StepKind { return config.StepKindRemove }
func (Remove) Group() step.Group     { return step.GroupFields }

func (Remove) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	fields, ok := FieldList{}.Parse(c, raw.Fields)
	return &remove{Base: step.NewBase(c, raw), fields: fields}, ok
}

type remove struct {
	step.Base
	fields []step.FieldRef
}

func (s *remove) Apply(_ *step.StepContext, in engine.Event) ([]engine.Event, error) {
	for _, f := range s.fields {
		if !f.Delete(&in) {
			if err := s.Missing(f); err != nil {
				return nil, err
			}
		}
	}
	return step.One(in), nil
}
