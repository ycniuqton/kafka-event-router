package recipes

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/step"
)

// FieldList parses the `fields` member of a step.
type FieldList struct{}

// Parse returns all parsed paths, recording an issue per invalid entry.
func (FieldList) Parse(c *step.StepCompileCtx, raw []string) ([]step.FieldRef, bool) {
	out := make([]step.FieldRef, 0, len(raw))
	ok := true
	for i, f := range raw {
		p, pok := c.Scope.Path(f, c.Pointer.Token(config.TokenFields).Index(i))
		ok = ok && pok
		out = append(out, step.FieldRef{FieldPath: p})
	}
	return out, ok
}
