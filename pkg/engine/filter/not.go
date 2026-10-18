package filter

import "github.com/eventrouter/router/pkg/config"

// NotCompiler compiles `not` nodes.
type NotCompiler struct{}

func (NotCompiler) Type() config.NodeType { return config.NodeTypeNot }

func (NotCompiler) Compile(c *NodeCompileCtx, rel config.Pointer, raw config.FilterNode) (Node, bool) {
	if raw.Child == nil {
		c.Scope.Fail(config.IssueSchemaInvalid, c.Abs(rel), "not node requires child")
		return nil, false
	}
	child, ok := c.Compile(rel.Token(config.TokenChild), *raw.Child)
	return &notNode{rel: rel, spec: raw, child: child}, ok
}

type notNode struct {
	rel   config.Pointer
	spec  config.FilterNode
	child Node
}

func (n *notNode) Pointer() config.Pointer { return n.rel }

func (n *notNode) Spec() config.FilterNode {
	s := n.spec
	child := n.child.Spec()
	s.Child = &child
	return s
}

func (n *notNode) Eval(ec *EvalContext) (bool, error) {
	ok, err := n.child.Eval(ec)
	if err != nil {
		return false, err
	}
	ec.record(n, !ok, nil)
	return !ok, nil
}
