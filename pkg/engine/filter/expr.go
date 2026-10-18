package filter

import (
	"errors"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/script"
)

// ExprCompiler compiles `expr` nodes into sandbox predicate modules.
type ExprCompiler struct{}

func (ExprCompiler) Type() config.NodeType { return config.NodeTypeExpr }

func (ExprCompiler) Compile(c *NodeCompileCtx, rel config.Pointer, raw config.FilterNode) (Node, bool) {
	if raw.Code == nil {
		c.Scope.Fail(config.IssueSchemaInvalid, c.Abs(rel), "expr node requires code")
		return nil, false
	}
	ptr := c.Abs(rel).Token(config.TokenCode)
	mod, ok := c.Scope.Script(script.Source{Code: *raw.Code, Entry: script.EntryPredicate, Origin: string(ptr)}, ptr)
	if !ok {
		return nil, false
	}
	return &exprNode{rel: rel, spec: raw, module: mod}, true
}

// ErrExprFailed wraps sandbox failures of an expr node.
var ErrExprFailed = errors.New("expr evaluation failed")

type exprNode struct {
	rel    config.Pointer
	spec   config.FilterNode
	module script.Module
}

func (n *exprNode) Pointer() config.Pointer { return n.rel }

func (n *exprNode) Spec() config.FilterNode { return n.spec }

// Eval calls the predicate; non-boolean results use JS truthiness and a warning.
func (n *exprNode) Eval(ec *EvalContext) (bool, error) {
	res, err := n.module.Call(ec.Ctx, script.Invocation{Event: ec.Snapshot(), Now: ec.Now})
	if err != nil {
		return false, &FilterError{Pointer: n.rel, Cause: err}
	}
	if b, isBool := res.Value.(bool); isBool {
		ec.record(n, b, nil)
		return b, nil
	}
	ok := Truthiness{}.Of(res.Value)
	ec.record(n, ok, &CoercionWarning{Code: WarnNonBooleanExpr})
	return ok, nil
}

// Truthiness applies JavaScript truthiness to JSON-shaped values.
type Truthiness struct{}

// Of returns false for null, false, 0, NaN and "", true otherwise.
func (Truthiness) Of(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	}
	if f, ok := (Coercer{}).Number(v); ok {
		return f != 0 && f == f
	}
	return true
}
