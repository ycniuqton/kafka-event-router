package recipes

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/filter"
	"github.com/eventrouter/router/pkg/engine/step"
	"github.com/eventrouter/router/pkg/engine/trace"
)

// IfElse runs thenSteps when the condition (a filter AST) is true, else elseSteps.
type IfElse struct{}

func (IfElse) Kind() config.StepKind { return config.StepKindIfElse }
func (IfElse) Group() step.Group     { return step.GroupFlow }

func (IfElse) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	if raw.Condition == nil || raw.ThenSteps == nil {
		c.Fail(config.IssueSchemaInvalid, config.TokenCondition, "if_else requires condition and thenSteps")
		return nil, false
	}
	nc := &filter.NodeCompileCtx{Scope: c.Scope, Base: c.Pointer.Token(config.TokenCondition), Nodes: c.Nodes, Operators: c.Operators}
	node, ok := nc.Compile(config.PointerRoot, *raw.Condition)
	s := &ifElse{Base: step.NewBase(c, raw), condition: node}
	s.then = c.Chain(c.Pointer.Token(config.TokenThenSteps), c.NestedPrefix(config.TokenThenSteps), *raw.ThenSteps)
	s.otherwise = step.NewStepChain(nil, c.Scope.Limits.MaxOutputsPerEvent)
	if raw.ElseSteps != nil {
		s.otherwise = c.Chain(c.Pointer.Token(config.TokenElseSteps), c.NestedPrefix(config.TokenElseSteps), *raw.ElseSteps)
	}
	return s, ok
}

type ifElse struct {
	step.Base
	condition       filter.Node
	then, otherwise *step.StepChain
}

func (s *ifElse) Apply(sc *step.StepContext, in engine.Event) ([]engine.Event, error) {
	ec := &filter.EvalContext{Ctx: sc.Ctx, Event: &in, Now: sc.Now, Branch: sc.Branch, Tracer: trace.NoopTracer{}}
	ok, err := s.condition.Eval(ec)
	if err != nil {
		return nil, &step.StepError{Reason: step.ReasonScriptFailed, Cause: err}
	}
	chain := s.otherwise
	if ok {
		chain = s.then
	}
	return chain.Run(sc, trace.NoopTracer{}, in)
}
