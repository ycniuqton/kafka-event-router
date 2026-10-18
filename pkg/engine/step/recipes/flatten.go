package recipes

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/step"
)

// FlattenDefaultSeparator joins nested keys when separator is absent.
const FlattenDefaultSeparator = "."

// Flatten turns a nested object into one level with joined keys; arrays stay values.
type Flatten struct{}

func (Flatten) Kind() config.StepKind { return config.StepKindFlatten }
func (Flatten) Group() step.Group     { return step.GroupStruct }

func (Flatten) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	sep := FlattenDefaultSeparator
	if raw.Separator != nil {
		sep = *raw.Separator
	}
	return FieldTransformBuilder{}.Build(c, raw, flattener{sep: sep}, true)
}

type flattener struct {
	sep string
}

func (f flattener) Map(_ *step.StepContext, v any) (any, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return v, nil
	}
	out := map[string]any{}
	f.into(out, "", m)
	return out, nil
}

func (f flattener) into(out map[string]any, prefix string, m map[string]any) {
	for k, v := range m {
		key := prefix + k
		child, ok := v.(map[string]any)
		if ok && len(child) > 0 {
			f.into(out, key+f.sep, child)
			continue
		}
		out[key] = v
	}
}
