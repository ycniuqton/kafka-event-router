package step

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/formula"
)

// NestedIDSeparator joins a parent step id and a nested step id in traces and errors.
const NestedIDSeparator = "/"

// Chain compiles the enabled steps at base into a chain; ids get idPrefix when nested
// and spliced steps (library) contribute their own entries.
func (c *StepCompileCtx) Chain(base config.Pointer, idPrefix string, steps config.Steps) *StepChain {
	entries := []ChainEntry{}
	for j, raw := range steps {
		if !raw.IsEnabled() {
			continue
		}
		child := *c
		child.Pointer = base.Index(j)
		child.ID = idPrefix + IDFor(raw, j)
		s, ok := c.Steps.Compile(&child, raw)
		if ok {
			entries = append(entries, ChainEntries{}.Of(s, child.Pointer)...)
		}
	}
	return NewStepChain(entries, c.Scope.Limits.MaxOutputsPerEvent)
}

// NestedPrefix returns the id prefix of steps nested under this step and member.
func (c *StepCompileCtx) NestedPrefix(member config.PointerToken) string {
	return c.ID + NestedIDSeparator + string(member) + NestedIDSeparator
}

// Formula compiles a formula member, recording FormulaInvalid at ptr on failure.
func (c *StepCompileCtx) Formula(src string, ptr config.Pointer) (*formula.Program, bool) {
	p, err := c.Formulas.Compile(src)
	if err != nil {
		c.Scope.Fail(config.IssueFormulaInvalid, ptr, "%v", err)
		return nil, false
	}
	fields, ok := p.Fields()
	c.Scope.Access.Formula(src, fields, ok)
	return p, true
}

// FormulaContext builds the evaluation context of a formula for one event.
func (sc *StepContext) FormulaContext(ev *engine.Event) *formula.EvalContext {
	return &formula.EvalContext{Ctx: sc.Ctx, Event: ev, Now: sc.Now, Helpers: sc.Helpers}
}
