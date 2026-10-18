package recipes

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/step"
)

// Copy duplicates a field's value (deep copy) to another path.
type Copy struct{}

func (Copy) Kind() config.StepKind { return config.StepKindCopy }
func (Copy) Group() step.Group     { return step.GroupFields }

func (Copy) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	return relocateCompiler{kind: config.StepKindCopy, keepSource: true}.Compile(c, raw)
}
