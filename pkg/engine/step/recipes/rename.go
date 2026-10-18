package recipes

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/step"
)

// Rename moves a field to a new name (from/to are full paths).
type Rename struct{}

func (Rename) Kind() config.StepKind { return config.StepKindRename }
func (Rename) Group() step.Group     { return step.GroupFields }

func (Rename) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	return relocateCompiler{kind: config.StepKindRename}.Compile(c, raw)
}
