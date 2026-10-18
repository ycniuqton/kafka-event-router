package formula

import (
	"context"
	"time"

	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/path"
)

// EvalContext is the per-event state a formula reads: the event, the frozen clock and helpers.
type EvalContext struct {
	Ctx     context.Context
	Event   *engine.Event
	Now     time.Time
	Helpers ctxhelpers.Set
}

// Expr is one compiled AST node.
type Expr interface {
	Eval(ec *EvalContext) (any, error)
}

// Program is a compiled formula.
type Program struct {
	root Expr
	src  string
}

// Source returns the formula text.
func (p *Program) Source() string { return p.src }

// Eval evaluates the formula against the event; missing fields read as null.
func (p *Program) Eval(ec *EvalContext) (any, error) { return p.root.Eval(ec) }

// literal is a constant (number, string, boolean or null).
type literal struct {
	value any
}

func (n literal) Eval(*EvalContext) (any, error) { return n.value, nil }

// fieldRef reads an event field; missing reads as null.
type fieldRef struct {
	path path.FieldPath
}

func (n fieldRef) Eval(ec *EvalContext) (any, error) {
	v, _ := n.path.Get(ec.Event)
	return v, nil
}

// call invokes a catalogue function with lazily evaluated arguments.
type call struct {
	fn   Function
	args []Expr
}

func (n call) Eval(ec *EvalContext) (any, error) {
	v, err := n.fn.Call(NewArgs(ec, n.args))
	if err != nil {
		return nil, &EvalError{Func: n.fn.Name(), Cause: err}
	}
	return v, nil
}

// binary applies an infix operator.
type binary struct {
	op          BinaryOperator
	left, right Expr
}

func (n binary) Eval(ec *EvalContext) (any, error) { return n.op.Apply(ec, n.left, n.right) }

// unary applies a prefix operator.
type unary struct {
	op      UnaryOperator
	operand Expr
}

func (n unary) Eval(ec *EvalContext) (any, error) {
	v, err := n.operand.Eval(ec)
	if err != nil {
		return nil, err
	}
	return n.op.Apply(v)
}

// Args gives functions lazy, memoised access to their argument values.
type Args struct {
	ec     *EvalContext
	exprs  []Expr
	values []any
	done   []bool
}

// NewArgs wraps argument expressions.
func NewArgs(ec *EvalContext, exprs []Expr) *Args {
	return &Args{ec: ec, exprs: exprs, values: make([]any, len(exprs)), done: make([]bool, len(exprs))}
}

// ValueArgs wraps already evaluated values (direct calls from tests and tools).
func ValueArgs(ec *EvalContext, values ...any) *Args {
	exprs := make([]Expr, len(values))
	for i, v := range values {
		exprs[i] = literal{value: v}
	}
	return NewArgs(ec, exprs)
}

// Len returns the argument count.
func (a *Args) Len() int { return len(a.exprs) }

// At evaluates argument i once.
func (a *Args) At(i int) (any, error) {
	if i >= len(a.exprs) {
		return nil, nil
	}
	if a.done[i] {
		return a.values[i], nil
	}
	v, err := a.exprs[i].Eval(a.ec)
	if err != nil {
		return nil, err
	}
	a.values[i], a.done[i] = v, true
	return v, nil
}

// All evaluates every argument in order.
func (a *Args) All() ([]any, error) {
	out := make([]any, a.Len())
	for i := range a.exprs {
		v, err := a.At(i)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

// Context returns the evaluation context of the call.
func (a *Args) Context() *EvalContext { return a.ec }
