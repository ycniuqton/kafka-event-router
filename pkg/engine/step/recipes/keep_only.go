package recipes

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/path"
	"github.com/eventrouter/router/pkg/engine/step"
)

// KeepOnly rebuilds the value with only the listed value paths.
type KeepOnly struct{}

func (KeepOnly) Kind() config.StepKind { return config.StepKindKeepOnly }
func (KeepOnly) Group() step.Group     { return step.GroupFields }

func (KeepOnly) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	fields, ok := FieldList{}.Parse(c, raw.Fields)
	for i, f := range fields {
		if ok && !(KeepOnlyPaths{}).Accepts(f) {
			c.Scope.Fail(config.IssuePathInvalid, c.Pointer.Token(config.TokenFields).Index(i), "keep_only accepts value.* member paths without indexes")
			ok = false
		}
	}
	return &keepOnly{Base: step.NewBase(c, raw), fields: fields}, ok
}

type keepOnly struct {
	step.Base
	fields []step.FieldRef
}

func (s *keepOnly) Apply(_ *step.StepContext, in engine.Event) ([]engine.Event, error) {
	kept := engine.Event{Value: map[string]any{}}
	for _, f := range s.fields {
		v, ok := f.Get(&in)
		if !ok {
			if err := s.Missing(f); err != nil {
				return nil, err
			}
			continue
		}
		if err := f.Write(&kept, v); err != nil {
			return nil, err
		}
	}
	in.Value = kept.Value
	return step.One(in), nil
}

// KeepOnlyPaths restricts keep_only to object member paths under value.
type KeepOnlyPaths struct{}

// Accepts reports whether f is a value member path without array indexes.
func (KeepOnlyPaths) Accepts(f step.FieldRef) bool {
	if f.Root != path.RootValue || f.IsRoot() {
		return false
	}
	for _, s := range f.Segments {
		if s.IsIndex {
			return false
		}
	}
	return true
}
