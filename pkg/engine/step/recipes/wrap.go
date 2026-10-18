package recipes

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/path"
	"github.com/eventrouter/router/pkg/engine/step"
)

// wrappableRoots are the event members wrap and unwrap may restructure.
var wrappableRoots = map[path.PathRoot]bool{path.RootValue: true, path.RootKey: true}

// Wrap nests the whole root member of `field` (usually value) at that path,
// e.g. field `value.payload` turns {"a":1} into {"payload":{"a":1}}.
type Wrap struct{}

func (Wrap) Kind() config.StepKind { return config.StepKindWrap }
func (Wrap) Group() step.Group     { return step.GroupStruct }

func (Wrap) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	at, ok := c.Path(raw.Field, config.TokenField)
	if ok && (at.IsRoot() || !wrappableRoots[at.Root]) {
		c.Fail(config.IssuePathInvalid, config.TokenField, "wrap needs a key or value member path such as value.payload")
		ok = false
	}
	return &wrap{Base: step.NewBase(c, raw), at: at, root: step.FieldRef{FieldPath: at.RootOnly()}}, ok
}

type wrap struct {
	step.Base
	at, root step.FieldRef
}

func (s *wrap) Apply(_ *step.StepContext, in engine.Event) ([]engine.Event, error) {
	whole, _ := s.root.Get(&in)
	if err := s.root.Write(&in, map[string]any{}); err != nil {
		return nil, err
	}
	if err := s.at.Write(&in, whole); err != nil {
		return nil, err
	}
	return step.One(in), nil
}

// Unwrap replaces the root member of `field` with the value at that path,
// e.g. field `value.payload` turns {"payload":{"a":1}} into {"a":1}.
type Unwrap struct{}

func (Unwrap) Kind() config.StepKind { return config.StepKindUnwrap }
func (Unwrap) Group() step.Group     { return step.GroupStruct }

func (Unwrap) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	at, ok := c.Path(raw.Field, config.TokenField)
	if ok && (at.IsRoot() || !wrappableRoots[at.Root]) {
		c.Fail(config.IssuePathInvalid, config.TokenField, "unwrap needs a key or value member path such as value.payload")
		ok = false
	}
	return &unwrap{Base: step.NewBase(c, raw), at: at, root: step.FieldRef{FieldPath: at.RootOnly()}}, ok
}

type unwrap struct {
	step.Base
	at, root step.FieldRef
}

func (s *unwrap) Apply(_ *step.StepContext, in engine.Event) ([]engine.Event, error) {
	inner, ok := s.at.Get(&in)
	if !ok {
		return step.One(in), s.Missing(s.at)
	}
	if err := s.root.Write(&in, inner); err != nil {
		return nil, err
	}
	return step.One(in), nil
}
