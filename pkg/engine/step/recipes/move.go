package recipes

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/step"
)

// Move relocates a value to any path, including across key/value/headers.
type Move struct{}

func (Move) Kind() config.StepKind { return config.StepKindMove }
func (Move) Group() step.Group     { return step.GroupFields }

func (Move) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	return relocateCompiler{kind: config.StepKindMove}.Compile(c, raw)
}
