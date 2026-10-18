package recipes

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/step"
)

// Hash replaces a field with its salted SHA-256 hex digest (per-team salt).
type Hash struct{}

func (Hash) Kind() config.StepKind { return config.StepKindHash }
func (Hash) Group() step.Group     { return step.GroupPrivacy }

func (Hash) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	target, ok := c.Path(raw.Field, config.TokenField)
	return &hash{Base: step.NewBase(c, raw), target: target}, ok
}

type hash struct {
	step.Base
	target step.FieldRef
}

func (s *hash) Apply(sc *step.StepContext, in engine.Event) ([]engine.Event, error) {
	v, ok := s.target.Get(&in)
	if !ok || v == nil {
		return step.One(in), s.Missing(s.target)
	}
	digest, err := sc.Helpers.Hash.Hash(sc.Ctx, v)
	if err != nil {
		return nil, &step.StepError{Reason: step.ReasonHelperFailed, Field: s.target.String(), Cause: err}
	}
	if err := s.target.Write(&in, digest); err != nil {
		return nil, err
	}
	return step.One(in), nil
}
