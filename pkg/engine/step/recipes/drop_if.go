package recipes

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/filter"
	"github.com/eventrouter/router/pkg/engine/step"
	"github.com/eventrouter/router/pkg/engine/trace"
)

// DropIf drops the event when its condition (a filter AST) is true.
type DropIf struct{}

func (DropIf) Kind() config.StepKind { return config.StepKindDropIf }
func (DropIf) Group() step.Group     { return step.GroupFlow }

func (DropIf) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	if raw.Condition == nil {
		c.Fail(config.IssueSchemaInvalid, config.TokenCondition, "drop_if requires condition")
		return nil, false
	}
	nc := &filter.NodeCompileCtx{Scope: c.Scope, Base: c.Pointer.Token(config.TokenCondition), Nodes: c.Nodes, Operators: c.Operators}
	node, ok := nc.Compile(config.PointerRoot, *raw.Condition)
	return &dropIf{Base: step.NewBase(c, raw), condition: node}, ok
}

type dropIf struct {
	step.Base
	condition filter.Node
}

func (s *dropIf) Apply(sc *step.StepContext, in engine.Event) ([]engine.Event, error) {
	ec := &filter.EvalContext{Ctx: sc.Ctx, Event: &in, Now: sc.Now, Branch: sc.Branch, Tracer: trace.NoopTracer{}}
	drop, err := s.condition.Eval(ec)
	if err != nil {
		return nil, &step.StepError{Reason: step.ReasonScriptFailed, Cause: err}
	}
	if drop {
		return []engine.Event{}, nil
	}
	return step.One(in), nil
}
