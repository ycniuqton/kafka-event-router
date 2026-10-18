package recipes

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/step"
)

// Mask hides a field using the shared ctx masker (full, last4, email).
type Mask struct{}

func (Mask) Kind() config.StepKind { return config.StepKindMask }
func (Mask) Group() step.Group     { return step.GroupPrivacy }

func (Mask) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	target, ok := c.Path(raw.Field, config.TokenField)
	if raw.Style == nil {
		c.Fail(config.IssueSchemaInvalid, config.TokenStyle, "mask requires style")
		return nil, false
	}
	return &mask{Base: step.NewBase(c, raw), target: target, style: *raw.Style}, ok
}

type mask struct {
	step.Base
	target step.FieldRef
	style  config.MaskStyle
}

func (s *mask) Apply(sc *step.StepContext, in engine.Event) ([]engine.Event, error) {
	v, ok := s.target.Get(&in)
	if !ok {
		return step.One(in), s.Missing(s.target)
	}
	masked, err := sc.Helpers.Mask.Mask(v, s.style)
	if err != nil {
		return nil, &step.StepError{Reason: step.ReasonHelperFailed, Field: s.target.String(), Cause: err}
	}
	if err := s.target.Write(&in, masked); err != nil {
		return nil, err
	}
	return step.One(in), nil
}
