package recipes

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/formula"
	"github.com/eventrouter/router/pkg/engine/step"
)

// Round rounds a number (or numeric text) half away from zero.
type Round struct{}

func (Round) Kind() config.StepKind { return config.StepKindRound }
func (Round) Group() step.Group     { return step.GroupTypes }

func (Round) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	decimals := 0
	if raw.Decimals != nil {
		decimals = *raw.Decimals
	}
	return FieldTransformBuilder{}.Build(c, raw, rounder{decimals: decimals}, true)
}

type rounder struct {
	decimals int
}

func (r rounder) Map(_ *step.StepContext, v any) (any, error) {
	f, err := formula.Numbers{}.Of(v)
	if err != nil {
		return nil, err
	}
	return formula.Rounder{}.Round(f, r.decimals), nil
}
