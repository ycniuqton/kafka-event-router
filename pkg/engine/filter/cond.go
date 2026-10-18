package filter

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/path"
)

// CondCompiler compiles `cond` nodes: field accessor + prepared operator.
type CondCompiler struct{}

func (CondCompiler) Type() config.NodeType { return config.NodeTypeCond }

func (CondCompiler) Compile(c *NodeCompileCtx, rel config.Pointer, raw config.FilterNode) (Node, bool) {
	if raw.Field == nil || raw.Op == nil {
		c.Scope.Fail(config.IssueSchemaInvalid, c.Abs(rel), "cond node requires field and op")
		return nil, false
	}
	field, ok := c.Scope.Path(*raw.Field, c.Abs(rel).Token(config.TokenField))
	if !ok {
		return nil, false
	}
	op, err := c.Operators.Get(*raw.Op)
	if err != nil {
		c.Scope.Fail(config.IssueSchemaInvalid, c.Abs(rel).Token(config.TokenOp), "%v", err)
		return nil, false
	}
	prepared, err := op.Prepare(OperandFrom(raw))
	if err != nil {
		code, ptr := OperandErrorInfo{}.Of(err, c.Abs(rel))
		c.Scope.Fail(code, ptr, "%v", err)
		return nil, false
	}
	return &condNode{rel: rel, spec: raw, field: field, op: op, prepared: prepared}, true
}

// OperandFrom extracts the operand members of a cond node with engine number types.
func OperandFrom(raw config.FilterNode) Operand {
	np := codec.NumberPolicy{}
	o := Operand{Value: np.Normalized(raw.Value), From: np.Normalized(raw.From), To: np.Normalized(raw.To), Amount: raw.Amount, Unit: raw.Unit}
	for _, v := range raw.Values {
		o.Values = append(o.Values, np.Normalized(v))
	}
	return o
}

type condNode struct {
	rel      config.Pointer
	spec     config.FilterNode
	field    path.FieldPath
	op       Operator
	prepared Prepared
}

func (n *condNode) Pointer() config.Pointer { return n.rel }

func (n *condNode) Spec() config.FilterNode { return n.spec }

// Eval applies the missing-field rule, then the operator with the fixed coercion table.
func (n *condNode) Eval(ec *EvalContext) (bool, error) {
	left, present := n.field.Get(ec.Event)
	if !present && !n.op.HandlesMissing() {
		ec.record(n, false, nil)
		return false, nil
	}
	ok, w := n.prepared.Match(MatchInput{Left: left, Present: present, Now: ec.Now, Coercer: ec.Coercer})
	ec.record(n, ok, w)
	return ok, nil
}
